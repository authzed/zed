package backupformat

import (
	"errors"
	"fmt"
	"io"

	"github.com/hamba/avro/v2"
	"github.com/hamba/avro/v2/ocf"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

func init() {
	// This defaults to a 1MiB limit, but large schemas can exceed this size.
	avro.DefaultConfig = avro.Config{
		MaxByteSliceSize: 1024 * 1024 * 100, // 100 MiB
	}.Freeze()
}

func NewDecoder(r io.Reader) (*OcfDecoder, error) {
	dec, err := ocf.NewDecoder(r)
	if err != nil {
		return nil, fmt.Errorf("unable to create ocf decoder: %w", err)
	}

	md := dec.Metadata()
	var zedToken *v1.ZedToken

	if token, ok := md[metadataKeyZT]; ok {
		zedToken = &v1.ZedToken{
			Token: string(token),
		}
	}

	var schemaText string
	if dec.HasNext() {
		var decodedSchema any
		if err := dec.Decode(&decodedSchema); err != nil {
			return nil, fmt.Errorf("unable to decode schema object: %w", err)
		}

		schema, ok := decodedSchema.(SchemaV1)
		if !ok {
			return nil, fmt.Errorf("received schema object of wrong type: %T", decodedSchema)
		}
		schemaText = schema.SchemaText
	} else {
		return nil, errors.New("avro stream contains no schema object")
	}

	return &OcfDecoder{
		dec,
		schemaText,
		zedToken,
	}, nil
}

type Decoder interface {
	Schema() (string, error)
	ZedToken() (*v1.ZedToken, error)
	Next() (*v1.Relationship, error)
}

var (
	_ Decoder = (*OcfDecoder)(nil)
	_ Decoder = (*RewriteDecoder)(nil)
)

type RewriteDecoder struct {
	Rewriter
	Decoder
}

func (d *RewriteDecoder) Schema() (string, error) {
	schema, err := d.Decoder.Schema()
	if err != nil {
		return "", err
	}
	return d.RewriteSchema(schema)
}

func (d *RewriteDecoder) Next() (*v1.Relationship, error) {
	r, err := d.Decoder.Next()
	if err != nil {
		return nil, err
	}

	for {
		r, err = d.RewriteRelationship(r)
		if err != nil {
			return nil, err
		} else if r == nil {
			// The rewriter filtered this relationship, decode another.
			r, err = d.Decoder.Next()
			if err != nil {
				return nil, err
			}
			continue
		}
		return r, nil
	}
}

type OcfDecoder struct {
	dec      *ocf.Decoder
	schema   string
	zedToken *v1.ZedToken
}

func (d *OcfDecoder) Schema() (string, error)         { return d.schema, nil }
func (d *OcfDecoder) ZedToken() (*v1.ZedToken, error) { return d.zedToken, nil }
func (d *OcfDecoder) Close() error                    { return nil }

func (d *OcfDecoder) Next() (*v1.Relationship, error) {
	if !d.dec.HasNext() {
		return nil, io.EOF
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

	if !flat.Expiration.IsZero() {
		rel.OptionalExpiresAt = timestamppb.New(flat.Expiration)
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
