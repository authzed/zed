package backupformat

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/hamba/avro/v2"
)

func init() {
	avro.DefaultConfig.Register(spiceDBBackupNamespace+"."+schemaV1SchemaName, SchemaV1{})
	avro.DefaultConfig.Register(spiceDBBackupNamespace+"."+relationshipV1SchemaName, RelationshipV1{})
}

type RelationshipV1 struct {
	ObjectType        string    `avro:"object_type"`
	ObjectID          string    `avro:"object_id"`
	Relation          string    `avro:"relation"`
	SubjectObjectType string    `avro:"subject_object_type"`
	SubjectObjectID   string    `avro:"subject_object_id"`
	SubjectRelation   string    `avro:"subject_relation"`
	CaveatName        string    `avro:"caveat_name"`
	CaveatContext     []byte    `avro:"caveat_context"`
	Expiration        time.Time `avro:"expiration"`
}

type SchemaV1 struct {
	SchemaText string `avro:"schema_text"`
}

const (
	spiceDBBackupNamespace = "com.authzed.spicedb.backup"

	relationshipV1SchemaName = "relationship_v1"
	schemaV1SchemaName       = "schema_v1"

	metadataKeyZT = "com.authzed.spicedb.zedtoken.v1"
)

func avroSchemaV1() (string, error) {
	relationshipSchema, err := recordSchemaFromAvroStruct(
		relationshipV1SchemaName,
		spiceDBBackupNamespace,
		RelationshipV1{},
	)
	if err != nil {
		return "", fmt.Errorf("unable to create schema: %w", err)
	}

	schemaSchema, err := recordSchemaFromAvroStruct(
		schemaV1SchemaName,
		spiceDBBackupNamespace,
		SchemaV1{},
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

func recordSchemaFromAvroStruct(name, namespace string, avroStruct any) (*avro.RecordSchema, error) {
	v := reflect.TypeOf(avroStruct)
	schemaFields := make([]*avro.Field, 0, v.NumField())
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		fieldName := f.Tag.Get("avro")
		if fieldName == "" {
			return nil, fmt.Errorf("field `%s` missing avro struct tag", f.Name)
		}
		fieldGoType := f.Type

		var fieldSchema avro.Schema
		if fieldGoType == reflect.TypeOf(time.Time{}) {
			// timestamp-millis is a logical type that extends long. see https://github.com/hamba/avro/tree/v2.30.0?tab=readme-ov-file#types-conversions
			logicalSchema := avro.NewPrimitiveLogicalSchema(avro.TimestampMillis)
			fieldSchema = avro.NewPrimitiveSchema(avro.Long, logicalSchema)
		} else {
			var fieldType avro.Type
			switch fieldGoType.Kind() {
			case reflect.String:
				fieldType = avro.String
			case reflect.Slice:
				if fieldGoType.Elem().Kind() != reflect.Uint8 {
					return nil, errors.New("unable to build schema for slice, only byte slices are supported")
				}
				fieldType = avro.Bytes
			default:
				return nil, fmt.Errorf("unsupported struct kind: %s", fieldGoType)
			}
			fieldSchema = avro.NewPrimitiveSchema(fieldType, nil)
		}

		schemaField, err := avro.NewField(fieldName, fieldSchema)
		if err != nil {
			return nil, fmt.Errorf("unable to create avro schema field: %w", err)
		}

		schemaFields = append(schemaFields, schemaField)
	}

	return avro.NewRecordSchema(name, namespace, schemaFields)
}
