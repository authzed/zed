package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ccoveille/go-safecast/v2"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/caveats/types"
	"github.com/authzed/spicedb/pkg/diff"
	caveatdiff "github.com/authzed/spicedb/pkg/diff/caveats"
	nsdiff "github.com/authzed/spicedb/pkg/diff/namespace"
	corev1 "github.com/authzed/spicedb/pkg/proto/core/v1"
	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/authzed/spicedb/pkg/schemadsl/generator"
	"github.com/authzed/spicedb/pkg/schemadsl/input"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/commands"
	"github.com/authzed/zed/internal/console"
)

type termChecker interface {
	IsTerminal(fd int) bool
}

type realTermChecker struct{}

func (rtc *realTermChecker) IsTerminal(fd int) bool {
	return term.IsTerminal(fd)
}

func registerAdditionalSchemaCmds(schemaCmd *cobra.Command) {
	schemaWriteCmd := &cobra.Command{
		Use:               "write <file?>",
		Args:              commands.ValidationWrapper(cobra.MaximumNArgs(1)),
		Short:             "Write a schema file (.zed or stdin) to the current permissions system",
		ValidArgsFunction: commands.FileExtensionCompletions("zed"),
		Example: `
	Write from a file:
		zed schema write schema.zed
	Write from stdin:
		cat schema.zed | zed schema write
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client.NewClient(cmd)
			if err != nil {
				return err
			}
			return schemaWriteCmdImpl(cmd, args, client, &realTermChecker{})
		},
	}

	schemaCopyCmd := &cobra.Command{
		Use:               "copy <src context> <dest context>",
		Short:             "Copy a schema from one context into another",
		Args:              commands.ValidationWrapper(cobra.ExactArgs(2)),
		ValidArgsFunction: ContextGet,
		RunE:              schemaCopyCmdFunc,
	}

	schemaDiffCmd := &cobra.Command{
		Use:   "diff <file> [file]",
		Short: "Diff a schema file against the current schema or another file",
		Long: `Compare schema files to find differences.

With one argument, the provided schema file is diffed against the schema
stored in the current context's permissions system via the API.

With two arguments, the two local schema files are diffed against each other.`,
		Args: commands.ValidationWrapper(cobra.RangeArgs(1, 2)),
		RunE: schemaDiffCmdFunc,
	}

	schemaCompileCmd := &cobra.Command{
		Use:   "compile <file>",
		Args:  commands.ValidationWrapper(cobra.ExactArgs(1)),
		Short: "Compile a schema that uses import syntax into one that can be written to SpiceDB",
		Example: `
	Write to stdout:
		zed schema compile root.zed
	Write to redirected stdout:
		zed schema compile schema.zed 1> compiled.zed
	Write to a file:
		zed schema compile root.zed --out compiled.zed
	`,
		ValidArgsFunction: commands.FileExtensionCompletions("zed"),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := schemaCompileOuter(cmd, args)
			return err
		},
	}

	schemaCmd.AddCommand(schemaCopyCmd)
	schemaCopyCmd.Flags().Bool("json", false, "output as JSON")
	schemaCopyCmd.Flags().String("schema-definition-prefix", "", "prefix to add to the schema's definition(s) before writing")

	schemaCmd.AddCommand(schemaWriteCmd)
	schemaWriteCmd.Flags().Bool("json", false, "output as JSON")
	schemaWriteCmd.Flags().String("schema-definition-prefix", "", "prefix to add to the schema's definition(s) before writing")

	schemaCmd.AddCommand(schemaDiffCmd)
	schemaDiffCmd.Flags().Bool("json", false, "output as JSON")

	schemaCmd.AddCommand(schemaCompileCmd)
	schemaCompileCmd.Flags().String("out", "", "output filepath; omitting writes to stdout")
}

func schemaDiffCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		c, err := client.NewClient(cmd)
		if err != nil {
			return err
		}
		return schemaDiffAPICmdFunc(cmd, args[0], c)
	}

	beforeReader, err := os.Open(args[0])
	if err != nil {
		return fmt.Errorf("failed to open before schema file: %w", err)
	}

	afterReader, err := os.Open(args[1])
	if err != nil {
		return fmt.Errorf("failed to open after schema file: %w", err)
	}

	schemaDiff, err := computeLocalDiff(beforeReader, afterReader, args[0], args[1])
	if err != nil {
		return err
	}

	resp := localDiffToResponse(schemaDiff)

	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := commands.PrettyProto(resp)
		if err != nil {
			return fmt.Errorf("failed to convert diff to JSON: %w", err)
		}
		console.Println(string(prettyProto))
		return nil
	}

	return printDiffSchemaResponse(resp, os.Stdout)
}

func schemaDiffAPICmdFunc(cmd *cobra.Command, schemaFile string, c v1.SchemaServiceClient) error {
	schemaBytes, err := os.ReadFile(schemaFile)
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}

	request := &v1.DiffSchemaRequest{
		ComparisonSchema: string(schemaBytes),
	}

	resp, err := c.DiffSchema(cmd.Context(), request)
	if err != nil {
		return err
	}

	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := commands.PrettyProto(resp)
		if err != nil {
			return fmt.Errorf("failed to convert diff to JSON: %w", err)
		}
		console.Println(string(prettyProto))
		return nil
	}

	return printDiffSchemaResponse(resp, os.Stdout)
}

type diffDelta struct {
	deltaType string
	name      string
}

// printDiffSchemaResponse formats a DiffSchemaResponse for human-readable output.
// Output is grouped by definition/caveat to match the original local diff format.
func printDiffSchemaResponse(resp *v1.DiffSchemaResponse, writer io.Writer) error {
	var addedDefs, removedDefs, addedCaveats, removedCaveats []string
	changedDefsOrder := make([]string, 0)
	changedDefs := make(map[string][]diffDelta)
	changedCaveatsOrder := make([]string, 0)
	changedCaveats := make(map[string][]diffDelta)

	addDefDelta := func(defName string, delta diffDelta) {
		if _, ok := changedDefs[defName]; !ok {
			changedDefsOrder = append(changedDefsOrder, defName)
		}
		changedDefs[defName] = append(changedDefs[defName], delta)
	}
	addCaveatDelta := func(caveatName string, delta diffDelta) {
		if _, ok := changedCaveats[caveatName]; !ok {
			changedCaveatsOrder = append(changedCaveatsOrder, caveatName)
		}
		changedCaveats[caveatName] = append(changedCaveats[caveatName], delta)
	}

	for _, d := range resp.GetDiffs() {
		switch t := d.GetDiff().(type) {
		case *v1.ReflectionSchemaDiff_DefinitionAdded:
			addedDefs = append(addedDefs, t.DefinitionAdded.GetName())
		case *v1.ReflectionSchemaDiff_DefinitionRemoved:
			removedDefs = append(removedDefs, t.DefinitionRemoved.GetName())
		case *v1.ReflectionSchemaDiff_DefinitionDocCommentChanged:
			addDefDelta(t.DefinitionDocCommentChanged.GetName(), diffDelta{string(nsdiff.NamespaceCommentsChanged), ""})
		case *v1.ReflectionSchemaDiff_RelationAdded:
			addDefDelta(t.RelationAdded.GetParentDefinitionName(), diffDelta{string(nsdiff.AddedRelation), t.RelationAdded.GetName()})
		case *v1.ReflectionSchemaDiff_RelationRemoved:
			addDefDelta(t.RelationRemoved.GetParentDefinitionName(), diffDelta{string(nsdiff.RemovedRelation), t.RelationRemoved.GetName()})
		case *v1.ReflectionSchemaDiff_RelationDocCommentChanged:
			addDefDelta(t.RelationDocCommentChanged.GetParentDefinitionName(), diffDelta{string(nsdiff.ChangedRelationComment), t.RelationDocCommentChanged.GetName()})
		case *v1.ReflectionSchemaDiff_RelationSubjectTypeAdded:
			addDefDelta(t.RelationSubjectTypeAdded.GetRelation().GetParentDefinitionName(), diffDelta{string(nsdiff.RelationAllowedTypeAdded), t.RelationSubjectTypeAdded.GetRelation().GetName()})
		case *v1.ReflectionSchemaDiff_RelationSubjectTypeRemoved:
			addDefDelta(t.RelationSubjectTypeRemoved.GetRelation().GetParentDefinitionName(), diffDelta{string(nsdiff.RelationAllowedTypeRemoved), t.RelationSubjectTypeRemoved.GetRelation().GetName()})
		case *v1.ReflectionSchemaDiff_PermissionAdded:
			addDefDelta(t.PermissionAdded.GetParentDefinitionName(), diffDelta{string(nsdiff.AddedPermission), t.PermissionAdded.GetName()})
		case *v1.ReflectionSchemaDiff_PermissionRemoved:
			addDefDelta(t.PermissionRemoved.GetParentDefinitionName(), diffDelta{string(nsdiff.RemovedPermission), t.PermissionRemoved.GetName()})
		case *v1.ReflectionSchemaDiff_PermissionDocCommentChanged:
			addDefDelta(t.PermissionDocCommentChanged.GetParentDefinitionName(), diffDelta{string(nsdiff.ChangedPermissionComment), t.PermissionDocCommentChanged.GetName()})
		case *v1.ReflectionSchemaDiff_PermissionExprChanged:
			addDefDelta(t.PermissionExprChanged.GetParentDefinitionName(), diffDelta{string(nsdiff.ChangedPermissionImpl), t.PermissionExprChanged.GetName()})
		case *v1.ReflectionSchemaDiff_CaveatAdded:
			addedCaveats = append(addedCaveats, t.CaveatAdded.GetName())
		case *v1.ReflectionSchemaDiff_CaveatRemoved:
			removedCaveats = append(removedCaveats, t.CaveatRemoved.GetName())
		case *v1.ReflectionSchemaDiff_CaveatDocCommentChanged:
			addCaveatDelta(t.CaveatDocCommentChanged.GetName(), diffDelta{string(caveatdiff.CaveatCommentsChanged), ""})
		case *v1.ReflectionSchemaDiff_CaveatExprChanged:
			addCaveatDelta(t.CaveatExprChanged.GetName(), diffDelta{string(caveatdiff.CaveatExpressionChanged), ""})
		case *v1.ReflectionSchemaDiff_CaveatParameterAdded:
			addCaveatDelta(t.CaveatParameterAdded.GetParentCaveatName(), diffDelta{string(caveatdiff.AddedParameter), t.CaveatParameterAdded.GetName()})
		case *v1.ReflectionSchemaDiff_CaveatParameterRemoved:
			addCaveatDelta(t.CaveatParameterRemoved.GetParentCaveatName(), diffDelta{string(caveatdiff.RemovedParameter), t.CaveatParameterRemoved.GetName()})
		case *v1.ReflectionSchemaDiff_CaveatParameterTypeChanged:
			addCaveatDelta(t.CaveatParameterTypeChanged.GetParameter().GetParentCaveatName(), diffDelta{string(caveatdiff.ParameterTypeChanged), t.CaveatParameterTypeChanged.GetParameter().GetName()})
		}
	}

	for _, ns := range addedDefs {
		fmt.Fprintf(writer, "Added definition: %s\n", ns)
	}
	for _, ns := range removedDefs {
		fmt.Fprintf(writer, "Removed definition: %s\n", ns)
	}
	for _, nsName := range changedDefsOrder {
		fmt.Fprintf(writer, "Changed definition: %s\n", nsName)
		for _, delta := range changedDefs[nsName] {
			fmt.Fprintf(writer, "\t %s: %s\n", delta.deltaType, delta.name)
		}
	}
	for _, caveat := range addedCaveats {
		fmt.Fprintf(writer, "Added caveat: %s\n", caveat)
	}
	for _, caveat := range removedCaveats {
		fmt.Fprintf(writer, "Removed caveat: %s\n", caveat)
	}
	for _, caveatName := range changedCaveatsOrder {
		fmt.Fprintf(writer, "Changed caveat: %s\n", caveatName)
		for _, delta := range changedCaveats[caveatName] {
			fmt.Fprintf(writer, "\t %s: %s\n", delta.deltaType, delta.name)
		}
	}

	return nil
}

func computeLocalDiff(beforeReader, afterReader io.Reader, beforeSource, afterSource string) (*diff.SchemaDiff, error) {
	beforeBytes, err := io.ReadAll(beforeReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read before schema: %w", err)
	}

	afterBytes, err := io.ReadAll(afterReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read after schema: %w", err)
	}

	before, err := compiler.Compile(
		compiler.InputSchema{Source: input.Source(beforeSource), SchemaString: string(beforeBytes)},
		compiler.AllowUnprefixedObjectType(),
	)
	if err != nil {
		return nil, err
	}

	after, err := compiler.Compile(
		compiler.InputSchema{Source: input.Source(afterSource), SchemaString: string(afterBytes)},
		compiler.AllowUnprefixedObjectType(),
	)
	if err != nil {
		return nil, err
	}

	dbefore := diff.NewDiffableSchemaFromCompiledSchema(before)
	dafter := diff.NewDiffableSchemaFromCompiledSchema(after)

	return diff.DiffSchemas(dbefore, dafter, types.Default.TypeSet)
}


// localDiffToResponse converts a local diff.SchemaDiff to a DiffSchemaResponse proto,
// producing the same JSON structure as the API for consistent --json output.
func localDiffToResponse(sd *diff.SchemaDiff) *v1.DiffSchemaResponse {
	var diffs []*v1.ReflectionSchemaDiff

	for _, ns := range sd.AddedNamespaces {
		diffs = append(diffs, &v1.ReflectionSchemaDiff{
			Diff: &v1.ReflectionSchemaDiff_DefinitionAdded{
				DefinitionAdded: &v1.ReflectionDefinition{Name: ns},
			},
		})
	}

	for _, ns := range sd.RemovedNamespaces {
		diffs = append(diffs, &v1.ReflectionSchemaDiff{
			Diff: &v1.ReflectionSchemaDiff_DefinitionRemoved{
				DefinitionRemoved: &v1.ReflectionDefinition{Name: ns},
			},
		})
	}

	for nsName, ns := range sd.ChangedNamespaces {
		for _, delta := range ns.Deltas() {
			diffs = append(diffs, namespaceDeltaToDiff(nsName, delta)...)
		}
	}

	for _, caveat := range sd.AddedCaveats {
		diffs = append(diffs, &v1.ReflectionSchemaDiff{
			Diff: &v1.ReflectionSchemaDiff_CaveatAdded{
				CaveatAdded: &v1.ReflectionCaveat{Name: caveat},
			},
		})
	}

	for _, caveat := range sd.RemovedCaveats {
		diffs = append(diffs, &v1.ReflectionSchemaDiff{
			Diff: &v1.ReflectionSchemaDiff_CaveatRemoved{
				CaveatRemoved: &v1.ReflectionCaveat{Name: caveat},
			},
		})
	}

	for caveatName, caveatDiff := range sd.ChangedCaveats {
		for _, delta := range caveatDiff.Deltas() {
			diffs = append(diffs, caveatDeltaToDiff(caveatName, delta)...)
		}
	}

	return &v1.DiffSchemaResponse{Diffs: diffs}
}

func namespaceDeltaToDiff(nsName string, delta nsdiff.Delta) []*v1.ReflectionSchemaDiff {
	switch delta.Type {
	case nsdiff.AddedRelation:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_RelationAdded{
				RelationAdded: &v1.ReflectionRelation{Name: delta.RelationName, ParentDefinitionName: nsName},
			},
		}}
	case nsdiff.RemovedRelation:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_RelationRemoved{
				RelationRemoved: &v1.ReflectionRelation{Name: delta.RelationName, ParentDefinitionName: nsName},
			},
		}}
	case nsdiff.AddedPermission:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_PermissionAdded{
				PermissionAdded: &v1.ReflectionPermission{Name: delta.RelationName, ParentDefinitionName: nsName},
			},
		}}
	case nsdiff.RemovedPermission:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_PermissionRemoved{
				PermissionRemoved: &v1.ReflectionPermission{Name: delta.RelationName, ParentDefinitionName: nsName},
			},
		}}
	case nsdiff.ChangedPermissionImpl:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_PermissionExprChanged{
				PermissionExprChanged: &v1.ReflectionPermission{Name: delta.RelationName, ParentDefinitionName: nsName},
			},
		}}
	case nsdiff.ChangedPermissionComment:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_PermissionDocCommentChanged{
				PermissionDocCommentChanged: &v1.ReflectionPermission{Name: delta.RelationName, ParentDefinitionName: nsName},
			},
		}}
	case nsdiff.ChangedRelationComment:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_RelationDocCommentChanged{
				RelationDocCommentChanged: &v1.ReflectionRelation{Name: delta.RelationName, ParentDefinitionName: nsName},
			},
		}}
	case nsdiff.NamespaceCommentsChanged:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_DefinitionDocCommentChanged{
				DefinitionDocCommentChanged: &v1.ReflectionDefinition{Name: nsName},
			},
		}}
	case nsdiff.RelationAllowedTypeAdded:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_RelationSubjectTypeAdded{
				RelationSubjectTypeAdded: &v1.ReflectionRelationSubjectTypeChange{
					Relation:           &v1.ReflectionRelation{Name: delta.RelationName, ParentDefinitionName: nsName},
					ChangedSubjectType: allowedRelationToTypeRef(delta.AllowedType),
				},
			},
		}}
	case nsdiff.RelationAllowedTypeRemoved:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_RelationSubjectTypeRemoved{
				RelationSubjectTypeRemoved: &v1.ReflectionRelationSubjectTypeChange{
					Relation:           &v1.ReflectionRelation{Name: delta.RelationName, ParentDefinitionName: nsName},
					ChangedSubjectType: allowedRelationToTypeRef(delta.AllowedType),
				},
			},
		}}
	default:
		return nil
	}
}

func allowedRelationToTypeRef(ar *corev1.AllowedRelation) *v1.ReflectionTypeReference {
	if ar == nil {
		return nil
	}
	ref := &v1.ReflectionTypeReference{
		SubjectDefinitionName: ar.GetNamespace(),
	}
	if ar.GetRequiredCaveat() != nil {
		ref.OptionalCaveatName = ar.GetRequiredCaveat().GetCaveatName()
	}
	switch t := ar.GetRelationOrWildcard().(type) {
	case *corev1.AllowedRelation_Relation:
		ref.Typeref = &v1.ReflectionTypeReference_OptionalRelationName{OptionalRelationName: t.Relation}
	case *corev1.AllowedRelation_PublicWildcard_:
		ref.Typeref = &v1.ReflectionTypeReference_IsPublicWildcard{IsPublicWildcard: true}
	default:
		ref.Typeref = &v1.ReflectionTypeReference_IsTerminalSubject{IsTerminalSubject: true}
	}
	return ref
}

func caveatDeltaToDiff(caveatName string, delta caveatdiff.Delta) []*v1.ReflectionSchemaDiff {
	switch delta.Type {
	case caveatdiff.CaveatCommentsChanged:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_CaveatDocCommentChanged{
				CaveatDocCommentChanged: &v1.ReflectionCaveat{Name: caveatName},
			},
		}}
	case caveatdiff.CaveatExpressionChanged:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_CaveatExprChanged{
				CaveatExprChanged: &v1.ReflectionCaveat{Name: caveatName},
			},
		}}
	case caveatdiff.AddedParameter:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_CaveatParameterAdded{
				CaveatParameterAdded: &v1.ReflectionCaveatParameter{Name: delta.ParameterName, ParentCaveatName: caveatName},
			},
		}}
	case caveatdiff.RemovedParameter:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_CaveatParameterRemoved{
				CaveatParameterRemoved: &v1.ReflectionCaveatParameter{Name: delta.ParameterName, ParentCaveatName: caveatName},
			},
		}}
	case caveatdiff.ParameterTypeChanged:
		return []*v1.ReflectionSchemaDiff{{
			Diff: &v1.ReflectionSchemaDiff_CaveatParameterTypeChanged{
				CaveatParameterTypeChanged: &v1.ReflectionCaveatParameterTypeChange{
					Parameter:    &v1.ReflectionCaveatParameter{Name: delta.ParameterName, ParentCaveatName: caveatName, Type: caveatTypeRefToString(delta.CurrentType)},
					PreviousType: caveatTypeRefToString(delta.PreviousType),
				},
			},
		}}
	default:
		return nil
	}
}

func caveatTypeRefToString(ref *corev1.CaveatTypeReference) string {
	if ref == nil {
		return ""
	}
	if len(ref.GetChildTypes()) == 0 {
		return ref.GetTypeName()
	}
	children := make([]string, 0, len(ref.GetChildTypes()))
	for _, child := range ref.GetChildTypes() {
		children = append(children, caveatTypeRefToString(child))
	}
	return ref.GetTypeName() + "<" + strings.Join(children, ", ") + ">"
}

func schemaCopyCmdFunc(cmd *cobra.Command, args []string) error {
	_, secretStore := client.DefaultStorage()
	srcClient, err := client.NewClientForContext(cmd, args[0], secretStore)
	if err != nil {
		return err
	}

	destClient, err := client.NewClientForContext(cmd, args[1], secretStore)
	if err != nil {
		return err
	}

	prefix := cobrautil.MustGetString(cmd, "schema-definition-prefix")
	outputJSON := cobrautil.MustGetBool(cmd, "json")

	resp, err := schemaCopyInner(cmd.Context(), srcClient, destClient, prefix)
	if err != nil {
		return err
	}

	if outputJSON {
		prettyProto, err := commands.PrettyProto(resp)
		if err != nil {
			return fmt.Errorf("failed to convert schema to JSON: %w", err)
		}

		console.Println(string(prettyProto))
	}

	return nil
}

func schemaCopyInner(ctx context.Context, srcClient, destClient v1.SchemaServiceClient, definitionPrefix string) (*v1.WriteSchemaResponse, error) {
	readRequest := &v1.ReadSchemaRequest{}
	log.Trace().Interface("request", readRequest).Msg("requesting schema read")

	readResp, err := srcClient.ReadSchema(ctx, readRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema: %w", err)
	}
	log.Trace().Interface("response", readResp).Msg("read schema")

	prefix, err := determinePrefixForSchema(ctx, definitionPrefix, nil, &readResp.SchemaText)
	if err != nil {
		return nil, err
	}

	schemaText, err := rewriteSchema(ctx, readResp.SchemaText, prefix)
	if err != nil {
		return nil, err
	}

	writeRequest := &v1.WriteSchemaRequest{Schema: schemaText}
	log.Trace().Interface("request", writeRequest).Msg("writing schema")

	resp, err := destClient.WriteSchema(ctx, writeRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to write schema: %w", err)
	}
	log.Trace().Interface("response", resp).Msg("wrote schema")

	return resp, nil
}

func schemaWriteCmdImpl(cmd *cobra.Command, args []string, client v1.SchemaServiceClient, terminalChecker termChecker) error {
	stdInFd, err := safecast.Convert[int](os.Stdin.Fd())
	if err != nil {
		return err
	}

	if len(args) == 0 && terminalChecker.IsTerminal(stdInFd) {
		return errors.New("must provide file path or contents via stdin")
	}

	var schemaBytes []byte
	switch len(args) {
	case 1:
		schemaBytes, err = os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("failed to read schema file: %w", err)
		}
		log.Trace().Str("schema", string(schemaBytes)).Str("file", args[0]).Msg("read schema from file")
	case 0:
		schemaBytes, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read schema file: %w", err)
		}
		log.Trace().Str("schema", string(schemaBytes)).Msg("read schema from stdin")
	default:
		panic("schemaWriteCmdFunc called with incorrect number of arguments")
	}

	if len(schemaBytes) == 0 {
		return errors.New("attempted to write empty schema")
	}

	prefix, err := determinePrefixForSchema(cmd.Context(), cobrautil.MustGetString(cmd, "schema-definition-prefix"), client, nil)
	if err != nil {
		return err
	}

	schemaText, err := rewriteSchema(cmd.Context(), string(schemaBytes), prefix)
	if err != nil {
		return err
	}

	request := &v1.WriteSchemaRequest{Schema: schemaText}
	log.Trace().Interface("request", request).Msg("writing schema")

	resp, err := client.WriteSchema(cmd.Context(), request)
	if err != nil {
		return fmt.Errorf("failed to write schema: %w", err)
	}
	log.Trace().Interface("response", resp).Msg("wrote schema")

	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := commands.PrettyProto(resp)
		if err != nil {
			return fmt.Errorf("failed to convert schema to JSON: %w", err)
		}

		console.Println(string(prettyProto))
	}

	return nil
}

// rewriteSchema rewrites the given existing schema to include the specified prefix on all definitions and caveats.
func rewriteSchema(ctx context.Context, existingSchemaText string, definitionPrefix string) (string, error) {
	if definitionPrefix == "" {
		return existingSchemaText, nil
	}

	compiled, err := compiler.Compile(
		compiler.InputSchema{Source: input.Source("schema"), SchemaString: existingSchemaText},
		compiler.ObjectTypePrefix(definitionPrefix),
		compiler.SkipValidation(),
	)
	if err != nil {
		return "", err
	}

	generated, _, err := generator.GenerateSchema(ctx, compiled.OrderedDefinitions)
	return generated, err
}

// determinePrefixForSchema determines the prefix to be applied to a schema that will be written.
//
// If specifiedPrefix is non-empty, it is returned immediately.
// If existingSchema is non-nil, it is parsed for the prefix.
// Otherwise, the client is used to retrieve the existing schema (if any), and the prefix is retrieved from there.
func determinePrefixForSchema(ctx context.Context, specifiedPrefix string, client v1.SchemaServiceClient, existingSchema *string) (string, error) {
	if specifiedPrefix != "" {
		return specifiedPrefix, nil
	}

	var schemaText string
	if existingSchema != nil {
		schemaText = *existingSchema
	} else {
		readSchemaText, err := commands.ReadSchema(ctx, client)
		if err != nil {
			return "", nil
		}
		schemaText = readSchemaText
	}

	// If there is no schema found, return the empty string.
	if schemaText == "" {
		return "", nil
	}

	// Otherwise, compile the schema and grab the prefixes of the namespaces defined.
	found, err := compiler.Compile(
		compiler.InputSchema{Source: input.Source("schema"), SchemaString: schemaText},
		compiler.AllowUnprefixedObjectType(),
		compiler.SkipValidation(),
	)
	if err != nil {
		return "", err
	}

	foundPrefixes := make([]string, 0, len(found.OrderedDefinitions))
	for _, def := range found.OrderedDefinitions {
		if strings.Contains(def.GetName(), "/") {
			parts := strings.Split(def.GetName(), "/")
			foundPrefixes = append(foundPrefixes, parts[0])
		} else {
			foundPrefixes = append(foundPrefixes, "")
		}
	}

	prefixes := stringz.Dedup(foundPrefixes)
	if len(prefixes) == 1 {
		prefix := prefixes[0]
		log.Debug().Str("prefix", prefix).Msg("found schema definition prefix")
		return prefix, nil
	}

	return "", nil
}

func schemaCompileOuter(cmd *cobra.Command, args []string) (bool, error) {
	outputFilepath := cobrautil.MustGetString(cmd, "out")

	var outputFile *os.File
	var toStdout bool
	switch outputFilepath {
	case "":
		toStdout = true
		outputFile = os.Stdout
	default:
		toStdout = false
		var err error
		outputFile, err = os.OpenFile(outputFilepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return toStdout, fmt.Errorf("failed to create output file: %w", err)
		}
		defer func() {
			if err := outputFile.Close(); err != nil {
				log.Warn().Err(err).Msg("failed to close output file")
			}
		}()
	}

	return toStdout, schemaCompileInner(cmd.Context(), args, outputFile)
}

// Compiles an input schema written in the new composable schema syntax
// and produces it as a fully-realized schema
func schemaCompileInner(ctx context.Context, args []string, writer io.Writer) error {
	inputFilepath := args[0]
	inputSourceFolder := filepath.Dir(inputFilepath)
	schemaBytes, err := os.ReadFile(inputFilepath)
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}
	log.Trace().Str("schema", string(schemaBytes)).Str("file", args[0]).Msg("read schema from file")

	if len(schemaBytes) == 0 {
		return errors.New("attempted to compile empty schema")
	}

	compiled, err := compiler.Compile(compiler.InputSchema{
		Source:       input.Source(inputFilepath),
		SchemaString: string(schemaBytes),
	}, compiler.AllowUnprefixedObjectType(),
		compiler.SourceFolder(inputSourceFolder))
	if err != nil {
		return err
	}

	// Generate the schema, which compiles over import and partial syntax
	generated, _, err := generator.GenerateSchema(ctx, compiled.OrderedDefinitions)
	if err != nil {
		return fmt.Errorf("could not generate resulting schema: %w", err)
	}

	// Add a newline at the end for hygiene's sake
	terminated := generated + "\n"

	_, err = fmt.Fprint(writer, terminated)
	if err != nil {
		return fmt.Errorf("failed to write schema: %w", err)
	}

	return nil
}
