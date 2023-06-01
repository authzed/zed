package backupformat

import (
	"errors"
	"fmt"
	"io"
	"reflect"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/hamba/avro/v2/ocf"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func NewDecoder(r io.Reader) (*Decoder, error) {
	dec, err := ocf.NewDecoder(r)
	if err != nil {
		return nil, fmt.Errorf("unable to create ocf decoder: %w", err)
	}

	var schemaText string
	if dec.HasNext() {
		var decodedSchema any
		if err := dec.Decode(&decodedSchema); err != nil {
			return nil, fmt.Errorf("unable to decode schema object: %w", err)
		}

		schema, ok := decodedSchema.(SchemaV1)
		if !ok {
			schemaType := reflect.TypeOf(decodedSchema)
			return nil, fmt.Errorf("received schema object of wrong type: %s", schemaType.Name())
		}
		schemaText = schema.SchemaText
	} else {
		return nil, errors.New("avro stream contains no schema object")
	}

	return &Decoder{
		dec,
		schemaText,
	}, nil
}

type Decoder struct {
	dec    *ocf.Decoder
	schema string
}

func (d *Decoder) Schema() string {
	return d.schema
}

func (d *Decoder) Close() error {
	return nil
}

func (d *Decoder) Next() (*v1.Relationship, error) {
	if !d.dec.HasNext() {
		return nil, nil
	}

	var nextRelIFace any
	if err := d.dec.Decode(&nextRelIFace); err != nil {
		return nil, fmt.Errorf("unable to decode relationship from avro stream: %w", err)
	}

	flat := nextRelIFace.(RelationshipV1)

	rel := &v1.Relationship{
		Resource: &v1.ObjectReference{
			ObjectType: flat.ObjectType,
			ObjectId:   flat.ObjectID,
		},
		Relation: flat.Relation,
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{
				ObjectType: flat.SubjectObjectType,
				ObjectId:   flat.SubjectObjectID,
			},
			OptionalRelation: flat.SubjectRelation,
		},
	}

	if flat.CaveatName != "" {
		var deserializedCtxt structpb.Struct

		if err := proto.Unmarshal(flat.CaveatContext, &deserializedCtxt); err != nil {
			return nil, fmt.Errorf("unable to deserialize caveat context: %w", err)
		}

		rel.OptionalCaveat = &v1.ContextualizedCaveat{
			CaveatName: flat.CaveatName,
			Context:    &deserializedCtxt,
		}
	}

	return rel, nil
}
