package backupformat

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	corev1 "github.com/authzed/spicedb/pkg/proto/core/v1"
	schemapkg "github.com/authzed/spicedb/pkg/schema"
	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/authzed/spicedb/pkg/schemadsl/generator"
)

func RegisterRewriterFlags(cmd *cobra.Command) {
	cmd.Flags().StringToString("prefix-replacements", nil, "potentially modify the schema to replace desired prefixes")
	cmd.Flags().String("prefix-filter", "", "include only schema definitions and relationships with a given prefix")
	cmd.Flags().Bool("rewrite-legacy", false, "potentially modify the schema to exclude legacy/broken syntax")
	_ = cmd.Flags().MarkHidden("rewrite-legacy")
}

func RewriterFromFlags(cmd *cobra.Command) Rewriter {
	if cmd == nil {
		return &NoopRewriter{}
	}

	rws := []Rewriter{&NoopRewriter{}}
	if set, err := cmd.Flags().GetBool("rewrite-legacy"); err == nil && set {
		rws = append(rws, &LegacyRewriter{})
	}

	if prefix, err := cmd.Flags().GetString("prefix-filter"); err == nil && prefix != "" {
		rws = append(rws, &PrefixFilterer{Prefix: prefix})
	}

	if replacements, err := cmd.Flags().GetStringToString("prefix-replacements"); err == nil && len(replacements) > 0 {
		rws = append(rws, &PrefixReplacer{replacements: replacements})
	}
	return &ChainRewriter{rws}
}

// Rewriter is used to transform a backup while encoding or decoding.
type Rewriter interface {
	// RewriteSchema transforms a schema.
	RewriteSchema(schema string) (string, error)

	// RewriteRelationship transforms a relationship or returns nil to signal
	// its removal.
	RewriteRelationship(r *v1.Relationship) (*v1.Relationship, error)
}

var (
	_ Rewriter = (*NoopRewriter)(nil)
	_ Rewriter = (*ChainRewriter)(nil)
	_ Rewriter = (*LegacyRewriter)(nil)
	_ Rewriter = (*PrefixFilterer)(nil)
	_ Rewriter = (*PrefixReplacer)(nil)
)

// NoopRewriter is a rewriter that returns the schema and relationships unchanged.
type NoopRewriter struct{}

func (n *NoopRewriter) RewriteSchema(schema string) (string, error) { return schema, nil }
func (n *NoopRewriter) RewriteRelationship(r *v1.Relationship) (*v1.Relationship, error) {
	return r.CloneVT(), nil
}

// ChainRewriter is a rewriter that composes other rewriters and
// runs them in the order they were declared.
type ChainRewriter struct {
	rewriters []Rewriter
}

func (cr *ChainRewriter) RewriteSchema(schema string) (string, error) {
	var err error
	for _, rw := range cr.rewriters {
		schema, err = rw.RewriteSchema(schema)
		if err != nil {
			return "", err
		}
	}
	return schema, nil
}

func (cr *ChainRewriter) RewriteRelationship(r *v1.Relationship) (*v1.Relationship, error) {
	var err error
	for _, rw := range cr.rewriters {
		r, err = rw.RewriteRelationship(r)
		if err != nil {
			return nil, err
		} else if r == nil {
			break
		}
	}
	return r, nil
}

func (cr *ChainRewriter) MarshalZerologObject(e *zerolog.Event) {
	for _, rw := range cr.rewriters {
		if obj, ok := rw.(zerolog.LogObjectMarshaler); ok {
			e.EmbedObject(obj)
		}
	}
}

type LegacyRewriter struct{}

func (lr *LegacyRewriter) RewriteRelationship(r *v1.Relationship) (*v1.Relationship, error) {
	return r.CloneVT(), nil
}

var (
	missingAllowedTypes = regexp.MustCompile(`(\s*)(relation)(.+)(/\* missing allowed types \*/)(.*)`)
	shortRelations      = regexp.MustCompile(`(\s*)relation [a-z][a-z0-9_]:(.+)`)
)

// TODO: what are these changes and why are they necessary?
func (lr *LegacyRewriter) RewriteSchema(schema string) (string, error) {
	schema = string(missingAllowedTypes.ReplaceAll([]byte(schema), []byte("\n/* deleted missing allowed type error */")))
	schema = string(shortRelations.ReplaceAll([]byte(schema), []byte("\n/* deleted short relation name */")))
	return schema, nil
}

// PrefixFilterer implements Rewriter by filtering any definitions and
// relationships that do not have the included prefix.
type PrefixFilterer struct {
	Prefix  string
	skipped uint64
	kept    uint64
}

func (sf *PrefixFilterer) MarshalZerologObject(e *zerolog.Event) {
	e.Str("prefix", sf.Prefix).Uint64("skippedRels", sf.skipped).Uint64("keptRels", sf.kept)
}

func (sf *PrefixFilterer) RewriteSchema(schema string) (string, error) {
	if schema == "" || sf.Prefix == "" {
		return schema, nil
	}

	compiledSchema, err := compileSchema(schema)
	if err != nil {
		return "", fmt.Errorf("error reading schema: %w", err)
	}

	var defs []compiler.SchemaDefinition
	for _, def := range compiledSchema.ObjectDefinitions {
		if strings.HasPrefix(def.Name, sf.Prefix+"/") {
			defs = append(defs, def.CloneVT())
		}
	}

	for _, def := range compiledSchema.CaveatDefinitions {
		if strings.HasPrefix(def.Name, sf.Prefix+"/") {
			defs = append(defs, def.CloneVT())
		}
	}

	return validateAndCompileDefs(defs)
}

func (sf *PrefixFilterer) RewriteRelationship(r *v1.Relationship) (*v1.Relationship, error) {
	if strings.HasPrefix(r.Resource.ObjectType, sf.Prefix) &&
		strings.HasPrefix(r.Subject.Object.ObjectType, sf.Prefix) {
		sf.kept++
		return r.CloneVT(), nil
	}
	sf.skipped++
	return nil, nil
}

// PrefixReplacer takes a given map of (oldPrefix, newPrefix)
// pairs and replaces those prefixes in definitions and relationships.
type PrefixReplacer struct {
	replacements map[string]string
	replacedRels uint64
}

func (pr *PrefixReplacer) replaceName(name string) string {
	prefix, prefixlessName, prefixExists := strings.Cut(name, "/")
	if newPrefix, ok := pr.replacements[prefix]; prefixExists && ok {
		if newPrefix == "" {
			return prefixlessName
		}
		return newPrefix + "/" + prefixlessName
	}
	return name
}

func (pr *PrefixReplacer) RewriteSchema(schema string) (string, error) {
	if schema == "" {
		return schema, nil
	}

	compiledSchema, err := compileSchema(schema)
	if err != nil {
		return "", fmt.Errorf("error reading schema: %w", err)
	}

	defs := make([]compiler.SchemaDefinition, 0, len(compiledSchema.OrderedDefinitions))
	for _, def := range compiledSchema.ObjectDefinitions {
		newDef := def.CloneVT()
		newDef.Name = pr.replaceName(newDef.Name)

		var rels []*corev1.Relation
		for _, rel := range def.Relation {
			newRel := rel.CloneVT()

			if newRel.TypeInformation != nil {
				var allowedTypes []*corev1.AllowedRelation
				for _, allowedType := range newRel.TypeInformation.AllowedDirectRelations {
					newType := allowedType.CloneVT()
					newType.Namespace = pr.replaceName(newType.Namespace)
					allowedTypes = append(allowedTypes, newType)
				}
				newRel.TypeInformation.AllowedDirectRelations = allowedTypes
			}

			rels = append(rels, newRel)
		}
		newDef.Relation = rels

		defs = append(defs, newDef)
	}

	for _, def := range compiledSchema.CaveatDefinitions {
		newDef := def.CloneVT()
		newDef.Name = pr.replaceName(newDef.Name)
		defs = append(defs, newDef)
	}

	return validateAndCompileDefs(defs)
}

func (pr *PrefixReplacer) RewriteRelationship(r *v1.Relationship) (*v1.Relationship, error) {
	newRel := r.CloneVT()
	prefix, name, prefixExists := strings.Cut(r.Resource.ObjectType, "/")
	if replacement, foundReplacement := pr.replacements[prefix]; prefixExists && foundReplacement {
		newRel.Resource.ObjectType = replacement + "/" + name
	}

	prefix, name, prefixExists = strings.Cut(r.Subject.Object.ObjectType, "/")
	if replacement, foundReplacement := pr.replacements[prefix]; prefixExists && foundReplacement {
		newRel.Subject.Object.ObjectType = replacement + "/" + name
	}

	if newRel.Resource.ObjectType != r.Resource.ObjectType ||
		newRel.Subject.Object.ObjectType != r.Subject.Object.ObjectType {
		pr.replacedRels++
	}

	return newRel, nil
}

func compileSchema(schema string) (*compiler.CompiledSchema, error) {
	return compiler.Compile(
		compiler.InputSchema{Source: "schema", SchemaString: schema},
		compiler.AllowUnprefixedObjectType(),
		compiler.SkipValidation(),
	)
}

func validateAndCompileDefs(defs []compiler.SchemaDefinition) (string, error) {
	if len(defs) == 0 {
		return "", errors.New("filtered all definitions from schema")
	}

	schema, _, err := generator.GenerateSchema(defs)
	if err != nil {
		return "", fmt.Errorf("error generating processed schema: %w", err)
	}

	compiledSchema, err := compiler.Compile(
		compiler.InputSchema{Source: "generated-schema", SchemaString: schema},
		compiler.AllowUnprefixedObjectType(),
	)
	if err != nil {
		return "", fmt.Errorf("failed to compile schema: %w", err)
	}

	for _, rawDef := range compiledSchema.ObjectDefinitions {
		ts := schemapkg.NewTypeSystem(schemapkg.ResolverForCompiledSchema(*compiledSchema))
		def, err := schemapkg.NewDefinition(ts, rawDef)
		if err != nil {
			return "", fmt.Errorf("failed to create schema definition: %w", err)
		}
		if _, err := def.Validate(context.Background()); err != nil {
			return "", fmt.Errorf("failed to validate schema definition: %w", err)
		}
	}

	return schema, nil
}
