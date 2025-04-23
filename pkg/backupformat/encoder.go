package backupformat

import (
	"errors"
	"fmt"
	"io"

	"github.com/hamba/avro/v2/ocf"
	"google.golang.org/protobuf/proto"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

func NewEncoderForExisting(w io.Writer) (*Encoder, error) {
	avroSchema, err := avroSchemaV1()
	if err != nil {
		return nil, fmt.Errorf("unable to create avro schema: %w", err)
	}

	enc, err := ocf.NewEncoder(avroSchema, w, ocf.WithCodec(ocf.Snappy))
	if err != nil {
		return nil, fmt.Errorf("unable to create encoder: %w", err)
	}

	return &Encoder{enc}, nil
}

func NewEncoder(w io.Writer, schema string, token *v1.ZedToken) (*Encoder, error) {
	avroSchema, err := avroSchemaV1()
	if err != nil {
		return nil, fmt.Errorf("unable to create avro schema: %w", err)
	}

	if token == nil {
		return nil, errors.New("missing expected token")
	}

	md := map[string][]byte{
		metadataKeyZT: []byte(token.Token),
	}

	enc, err := ocf.NewEncoder(avroSchema, w, ocf.WithCodec(ocf.Snappy), ocf.WithMetadata(md))
	if err != nil {
		return nil, fmt.Errorf("unable to create encoder: %w", err)
	}

	if err := enc.Encode(SchemaV1{
		SchemaText: schema,
	}); err != nil {
		return nil, fmt.Errorf("unable to encode SpiceDB schema object: %w", err)
	}

	return &Encoder{enc}, nil
}

type Encoder struct {
	enc *ocf.Encoder
}

func (e *Encoder) Append(rel *v1.Relationship) error {
	var toEncode RelationshipV1

	toEncode.ObjectType = rel.Resource.ObjectType
	toEncode.ObjectID = rel.Resource.ObjectId
	toEncode.Relation = rel.Relation
	toEncode.SubjectObjectType = rel.Subject.Object.ObjectType
	toEncode.SubjectObjectID = rel.Subject.Object.ObjectId
	toEncode.SubjectRelation = rel.Subject.OptionalRelation
	if rel.OptionalCaveat != nil {
		contextBytes, err := proto.Marshal(rel.OptionalCaveat.Context)
		if err != nil {
			return fmt.Errorf("error marshaling caveat context: %w", err)
		}

		toEncode.CaveatName = rel.OptionalCaveat.CaveatName
		toEncode.CaveatContext = contextBytes
	}

	if err := e.enc.Encode(toEncode); err != nil {
		return fmt.Errorf("unable to encode relationship: %w", err)
	}

	return nil
}

func (e *Encoder) Close() error {
	if err := e.enc.Flush(); err != nil {
		return fmt.Errorf("unable to flush encoder: %w", err)
	}
	return nil
}
