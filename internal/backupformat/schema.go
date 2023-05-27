package backupformat

import (
	"fmt"

	"github.com/hamba/avro/v2"
)

func init() {
	avro.DefaultConfig.Register(spiceDBBackupNamespace+"."+schemaV1SchemaName, SchemaV1{})
	avro.DefaultConfig.Register(spiceDBBackupNamespace+"."+relationshipV1SchemaName, RelationshipV1{})
}

type RelationshipV1 struct {
	ObjectType        string `avro:"object_type"`
	ObjectID          string `avro:"object_id"`
	Relation          string `avro:"relation"`
	SubjectObjectType string `avro:"subject_object_type"`
	SubjectObjectID   string `avro:"subject_object_id"`
	SubjectRelation   string `avro:"subject_relation"`
	CaveatName        string `avro:"caveat_name"`
	CaveatContext     []byte `avro:"caveat_context"`
}

type SchemaV1 struct {
	SchemaText string `avro:"schema_text"`
}

const (
	spiceDBBackupNamespace = "com.authzed.spicedb.backup"

	relationshipV1SchemaName = "relationship_v1"
	schemaV1SchemaName       = "schema_v1"
)

func avroSchemaV1() (string, error) {
	stringFieldNames := []string{
		"object_type",
		"object_id",
		"relation",
		"subject_object_type",
		"subject_object_id",
		"subject_relation",
		"caveat_name",
	}
	relationshipFields := make([]*avro.Field, 0, len(stringFieldNames)+1)
	for _, fieldName := range stringFieldNames {
		field, err := avro.NewField(fieldName, avro.NewPrimitiveSchema(avro.String, nil))
		if err != nil {
			return "", fmt.Errorf("unable to create avro schema field: %w", err)
		}
		relationshipFields = append(relationshipFields, field)
	}
	caveatContextField, err := avro.NewField("caveat_context", avro.NewPrimitiveSchema(avro.Bytes, nil))
	if err != nil {
		return "", fmt.Errorf("unable to create avro schema field: %w", err)
	}
	relationshipFields = append(relationshipFields, caveatContextField)

	relationshipSchema, err := avro.NewRecordSchema(
		relationshipV1SchemaName,
		spiceDBBackupNamespace,
		relationshipFields,
	)
	if err != nil {
		return "", fmt.Errorf("unable to create schema: %w", err)
	}

	schemaField, err := avro.NewField("schema_text", avro.NewPrimitiveSchema(avro.String, nil))
	if err != nil {
		return "", fmt.Errorf("unable to create avro schema field: %w", err)
	}

	schemaSchema, err := avro.NewRecordSchema(
		schemaV1SchemaName,
		spiceDBBackupNamespace,
		[]*avro.Field{schemaField},
	)
	if err != nil {
		return "", fmt.Errorf("unable to create avro SpiceDB schema schema: %w", err)
	}

	unionSchema, err := avro.NewUnionSchema([]avro.Schema{relationshipSchema, schemaSchema})
	if err != nil {
		return "", fmt.Errorf("unable to create avro union schema: %w", err)
	}

	serialized, err := unionSchema.MarshalJSON()
	return string(serialized), err
}
