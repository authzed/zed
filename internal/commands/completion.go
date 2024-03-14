package commands

import (
	"errors"
	"strings"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/client"
)

type CompletionArgumentType int

const (
	ResourceType CompletionArgumentType = iota
	ResourceID
	Permission
	SubjectType
	SubjectID
	SubjectTypeWithOptionalRelation
)

func FileExtensionCompletions(extension ...string) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return extension, cobra.ShellCompDirectiveFilterFileExt
	}
}

func GetArgs(fields ...CompletionArgumentType) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Read the current schema, if any.
		schema, err := readSchema(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		// Find the specified resource type, if any.
		var resourceType string
	loop:
		for index, arg := range args {
			field := fields[index]
			switch field {
			case ResourceType:
				resourceType = arg
				break loop

			case ResourceID:
				pieces := strings.Split(arg, ":")
				if len(pieces) >= 1 {
					resourceType = pieces[0]
					break loop
				}
			}
		}

		// Handle : on resource and subject IDs.
		if strings.HasSuffix(toComplete, ":") && (fields[len(args)] == ResourceID || fields[len(args)] == SubjectID) {
			comps := []string{}
			comps = cobra.AppendActiveHelp(comps, "Please enter an object ID")
			return comps, cobra.ShellCompDirectiveNoFileComp
		}

		// Handle # on subject types. If the toComplete contains a valid subject,
		// then we should return the relation names. Note that we cannot do this
		// on the # character because shell autocompletion won't send it to us.
		if len(args) == len(fields)-1 && toComplete != "" && fields[len(args)] == SubjectTypeWithOptionalRelation {
			for _, objDef := range schema.ObjectDefinitions {
				subjectType := toComplete
				if objDef.Name == subjectType {
					relationNames := make([]string, 0)
					relationNames = append(relationNames, subjectType)
					for _, relation := range objDef.Relation {
						relationNames = append(relationNames, subjectType+"#"+relation.Name)
					}
					return relationNames, cobra.ShellCompDirectiveNoFileComp
				}
			}
		}

		if len(args) >= len(fields) {
			// If we have all the arguments, return no completions.
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Return the completions.
		currentFieldType := fields[len(args)]
		switch currentFieldType {
		case ResourceType:
			fallthrough

		case SubjectType:
			fallthrough

		case SubjectID:
			fallthrough

		case SubjectTypeWithOptionalRelation:
			fallthrough

		case ResourceID:
			resourceTypeNames := make([]string, 0, len(schema.ObjectDefinitions))
			for _, objDef := range schema.ObjectDefinitions {
				resourceTypeNames = append(resourceTypeNames, objDef.Name)
			}

			flags := cobra.ShellCompDirectiveNoFileComp
			if currentFieldType == ResourceID || currentFieldType == SubjectID || currentFieldType == SubjectTypeWithOptionalRelation {
				flags |= cobra.ShellCompDirectiveNoSpace
			}

			return resourceTypeNames, flags

		case Permission:
			if resourceType == "" {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}

			relationNames := make([]string, 0)
			for _, objDef := range schema.ObjectDefinitions {
				if objDef.Name == resourceType {
					for _, relation := range objDef.Relation {
						relationNames = append(relationNames, relation.Name)
					}
				}
			}
			return relationNames, cobra.ShellCompDirectiveNoFileComp
		}

		return nil, cobra.ShellCompDirectiveDefault
	}
}

func readSchema(cmd *cobra.Command) (*compiler.CompiledSchema, error) {
	// TODO: we should find a way to cache this
	client, err := client.NewClient(cmd)
	if err != nil {
		return nil, err
	}

	request := &v1.ReadSchemaRequest{}

	resp, err := client.ReadSchema(cmd.Context(), request)
	if err != nil {
		return nil, err
	}

	schemaText := resp.SchemaText
	if len(schemaText) == 0 {
		return nil, errors.New("no schema defined")
	}

	compiledSchema, err := compiler.Compile(
		compiler.InputSchema{Source: "schema", SchemaString: schemaText},
		compiler.AllowUnprefixedObjectType(),
		compiler.SkipValidation(),
	)
	if err != nil {
		return nil, err
	}

	return compiledSchema, nil
}
