package backupformat

import (
	"fmt"
	"io"
	"strconv"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/namespace"
	core "github.com/authzed/spicedb/pkg/proto/core/v1"
	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/authzed/spicedb/pkg/schemadsl/generator"
	"github.com/authzed/spicedb/pkg/schemadsl/input"
	"github.com/authzed/spicedb/pkg/spiceerrors"
)

// RedactionOptions are the options to use when redacting data.
type RedactionOptions struct {
	// RedactDefinitions will redact the definition names.
	RedactDefinitions bool

	// RedactRelations will redact the relation names.
	RedactRelations bool

	// RedactObjectIDs will redact the object IDs.
	RedactObjectIDs bool
}

// RedactionMap is the map of original names to their redacted names.
type RedactionMap struct {
	// Definitions is the map of original definition names to their redacted names.
	Definitions map[string]string

	// Caveats is the map of original caveat names to their redacted names.
	Caveats map[string]string

	// Relations is the map of original relation names to their redacted names.
	Relations map[string]string

	// ObjectIDs is the map of original object IDs to their redacted names.
	ObjectIDs map[string]string
}

// Invert returns the inverted redaction map, with the redacted names as the keys.
func (rm RedactionMap) Invert() RedactionMap {
	inverted := RedactionMap{
		Definitions: make(map[string]string),
		Caveats:     make(map[string]string),
		Relations:   make(map[string]string),
		ObjectIDs:   make(map[string]string),
	}

	for k, v := range rm.Definitions {
		inverted.Definitions[v] = k
	}

	for k, v := range rm.Caveats {
		inverted.Caveats[v] = k
	}

	for k, v := range rm.Relations {
		inverted.Relations[v] = k
	}

	for k, v := range rm.ObjectIDs {
		inverted.ObjectIDs[v] = k
	}

	return inverted
}

// NewRedactor creates a new redactor that will redact the data as it is written.
func NewRedactor(dec *Decoder, w io.Writer, opts RedactionOptions) (*Redactor, error) {
	// Rewrite the schema to redact as requested.
	redactedSchema, redactionMap, err := redactSchema(dec.Schema(), opts)
	if err != nil {
		return nil, err
	}

	// Create a new encoder with the redacted schema.
	token := dec.ZedToken()
	encoder, err := NewEncoder(w, redactedSchema, token)
	if err != nil {
		return nil, err
	}

	return &Redactor{dec, opts, encoder, redactionMap}, nil
}

type Redactor struct {
	dec          *Decoder
	opts         RedactionOptions
	enc          *Encoder
	redactionMap RedactionMap
}

// Next redacts the next record and writes it to the writer.
func (r *Redactor) Next() error {
	// Read the next record.
	rel, err := r.dec.Next()
	if err != nil {
		return err
	}

	if rel == nil {
		return io.EOF
	}

	// Redact the record.
	redactedRel, err := redactRelationship(rel, &r.redactionMap, r.opts)
	if err != nil {
		return err
	}

	// Write the redacted record.
	return r.enc.Append(redactedRel)
}

// RedactionMap returns the redaction map containing the original names and their redacted names.
func (r *Redactor) RedactionMap() RedactionMap {
	return r.redactionMap
}

func (r *Redactor) Close() error {
	if err := r.enc.Close(); err != nil {
		return err
	}

	return r.dec.Close()
}

func redactSchema(schema string, opts RedactionOptions) (string, RedactionMap, error) {
	// Parse the schema.
	compiled, err := compiler.Compile(compiler.InputSchema{
		Source: input.Source("schema"), SchemaString: schema,
	})
	if err != nil {
		return "", RedactionMap{}, err
	}

	// Create a new schema with the redacted fields.
	redactionMap := RedactionMap{
		Definitions: make(map[string]string),
		Caveats:     make(map[string]string),
		Relations:   make(map[string]string),
		ObjectIDs:   make(map[string]string),
	}

	redactionCount := 0

	// Redact namespace and caveat names.
	if opts.RedactDefinitions {
		for _, nsDef := range compiled.ObjectDefinitions {
			if opts.RedactDefinitions {
				redactionMap.Definitions[nsDef.Name] = "def" + strconv.Itoa(redactionCount)
				redactionCount++
				nsDef.Name = redactionMap.Definitions[nsDef.Name]
			}

			namespace.FilterUserDefinedMetadataInPlace(nsDef)
		}

		if len(compiled.CaveatDefinitions) > 0 {
			fmt.Println("WARNING: Caveat parameters and comments are not currently redacted.")
		}

		for _, caveatDef := range compiled.CaveatDefinitions {
			if opts.RedactDefinitions {
				redactionMap.Caveats[caveatDef.Name] = "cav" + strconv.Itoa(redactionCount)
				redactionCount++
				caveatDef.Name = redactionMap.Caveats[caveatDef.Name]
			}

			// TODO: Redact caveat parameters.
			// TODO: filter caveat metadata.
		}
	}

	// Redact relation names.
	if opts.RedactRelations {
		for _, nsDef := range compiled.ObjectDefinitions {
			for _, relDef := range nsDef.Relation {
				if existing, ok := redactionMap.Relations[relDef.Name]; ok {
					relDef.Name = existing
					continue
				}

				redactionMap.Relations[relDef.Name] = "rel" + strconv.Itoa(redactionCount)
				redactionCount++
				relDef.Name = redactionMap.Relations[relDef.Name]
			}
		}
	}

	// Redact type information.
	if opts.RedactDefinitions || opts.RedactRelations {
		for _, nsDef := range compiled.ObjectDefinitions {
			for _, relDef := range nsDef.Relation {
				if relDef.TypeInformation != nil {
					for _, allowedDirect := range relDef.TypeInformation.AllowedDirectRelations {
						if opts.RedactDefinitions {
							allowedDirect.Namespace = redactionMap.Definitions[allowedDirect.Namespace]

							if allowedDirect.RequiredCaveat != nil {
								allowedDirect.RequiredCaveat.CaveatName = redactionMap.Caveats[allowedDirect.RequiredCaveat.CaveatName]
							}
						}

						if opts.RedactRelations {
							switch t := allowedDirect.RelationOrWildcard.(type) {
							case *core.AllowedRelation_Relation:
								t.Relation = redactionMap.Relations[t.Relation]
							}
						}
					}
				}
			}
		}
	}

	// Redact within userset rewrites.
	if opts.RedactRelations {
		for _, nsDef := range compiled.ObjectDefinitions {
			for _, relDef := range nsDef.Relation {
				if relDef.UsersetRewrite != nil {
					err := redactUsersetRewrite(relDef.UsersetRewrite, &redactionMap)
					if err != nil {
						return "", RedactionMap{}, err
					}
				}
			}
		}
	}

	// Generate the schema string.
	generated, _, err := generator.GenerateSchema(compiled.OrderedDefinitions)
	return generated, redactionMap, err
}

func redactUsersetRewrite(usersetRewrite *core.UsersetRewrite, redactionMap *RedactionMap) error {
	switch t := usersetRewrite.RewriteOperation.(type) {
	case *core.UsersetRewrite_Union:
		return redactRewriteChildren(t.Union.Child, redactionMap)

	case *core.UsersetRewrite_Intersection:
		return redactRewriteChildren(t.Intersection.Child, redactionMap)

	case *core.UsersetRewrite_Exclusion:
		return redactRewriteChildren(t.Exclusion.Child, redactionMap)

	default:
		return spiceerrors.MustBugf("unknown userset rewrite type: %T", t)
	}
}

func redactRewriteChildren(children []*core.SetOperation_Child, redactionMap *RedactionMap) error {
	for _, child := range children {
		switch t := child.ChildType.(type) {
		case *core.SetOperation_Child_ComputedUserset:
			t.ComputedUserset.Relation = redactionMap.Relations[t.ComputedUserset.Relation]

		case *core.SetOperation_Child_UsersetRewrite:
			err := redactUsersetRewrite(t.UsersetRewrite, redactionMap)
			if err != nil {
				return err
			}

		case *core.SetOperation_Child_TupleToUserset:
			t.TupleToUserset.Tupleset.Relation = redactionMap.Relations[t.TupleToUserset.Tupleset.Relation]
			t.TupleToUserset.ComputedUserset.Relation = redactionMap.Relations[t.TupleToUserset.ComputedUserset.Relation]

		case *core.SetOperation_Child_XNil:
			// nothing to do

		case *core.SetOperation_Child_XThis:
			// nothing to do

		default:
			return spiceerrors.MustBugf("unknown child type: %T", t)
		}
	}

	return nil
}

func redactRelationship(rel *v1.Relationship, redactionMap *RedactionMap, opts RedactionOptions) (*v1.Relationship, error) {
	redactedRel := rel.CloneVT()

	// Redact the resource.
	if opts.RedactDefinitions {
		redactedRel.Resource.ObjectType = redactionMap.Definitions[redactedRel.Resource.ObjectType]
		redactedRel.Subject.Object.ObjectType = redactionMap.Definitions[redactedRel.Subject.Object.ObjectType]

		if rel.OptionalCaveat != nil {
			redactedRel.OptionalCaveat.CaveatName = redactionMap.Caveats[redactedRel.OptionalCaveat.CaveatName]
		}
	}

	// Redact the relation.
	if opts.RedactRelations {
		redactedRel.Relation = redactionMap.Relations[redactedRel.Relation]

		if rel.Subject.OptionalRelation != "" {
			redactedRel.Subject.OptionalRelation = redactionMap.Relations[redactedRel.Subject.OptionalRelation]
		}
	}

	// Redact the object IDs.
	if opts.RedactObjectIDs {
		if _, ok := redactionMap.ObjectIDs[redactedRel.Resource.ObjectId]; !ok {
			redactionMap.ObjectIDs[redactedRel.Resource.ObjectId] = "obj" + strconv.Itoa(len(redactionMap.ObjectIDs))
		}

		redactedRel.Resource.ObjectId = redactionMap.ObjectIDs[redactedRel.Resource.ObjectId]

		if _, ok := redactionMap.ObjectIDs[redactedRel.Subject.Object.ObjectId]; !ok {
			redactionMap.ObjectIDs[redactedRel.Subject.Object.ObjectId] = "obj" + strconv.Itoa(len(redactionMap.ObjectIDs))
		}

		redactedRel.Subject.Object.ObjectId = redactionMap.ObjectIDs[redactedRel.Subject.Object.ObjectId]
	}

	return redactedRel, nil
}
