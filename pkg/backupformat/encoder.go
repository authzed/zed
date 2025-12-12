package backupformat

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/hamba/avro/v2/ocf"
	"github.com/mattn/go-isatty"
	"github.com/natefinch/atomic"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
	"google.golang.org/protobuf/proto"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"

	"github.com/authzed/zed/internal/console"
)

func NewEncoderForExisting(w io.Writer) (*OcfEncoder, error) {
	avroSchema, err := avroSchemaV1()
	if err != nil {
		return nil, fmt.Errorf("unable to create avro schema: %w", err)
	}

	enc, err := ocf.NewEncoder(avroSchema, w, ocf.WithCodec(ocf.Snappy))
	if err != nil {
		return nil, fmt.Errorf("unable to create encoder: %w", err)
	}

	return &OcfEncoder{enc}, nil
}

func NewEncoder(w io.Writer, schema string, token *v1.ZedToken) (*OcfEncoder, error) {
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

	return &OcfEncoder{enc}, nil
}

// Encoder represents the operations required to iteratively encode a backup
// of SpiceDB relationship data.
type Encoder interface {
	// Append encodes an additional Relationship using the provided cursor to
	// keep track of progress.
	Append(r *v1.Relationship, cursor string) error

	// MarkComplete signals that the final relationship has been written and
	// that the process is complete.
	MarkComplete()
}

var (
	_ Encoder = (*MockEncoder)(nil)
	_ Encoder = (*OcfEncoder)(nil)
	_ Encoder = (*OcfFileEncoder)(nil)
	_ Encoder = (*ProgressRenderingEncoder)(nil)
)

type MockEncoder struct {
	Relationships []*v1.Relationship
	Cursors       []string
	Complete      bool
}

func (m *MockEncoder) Append(r *v1.Relationship, cursor string) error {
	m.Relationships = append(m.Relationships, r)
	m.Cursors = append(m.Cursors, cursor)
	return nil
}

func (m *MockEncoder) MarkComplete() { m.Complete = true }

// OcfEncoder implements `Encoder` by formatting data in the AVRO OCF format.
type OcfEncoder struct {
	enc *ocf.Encoder
}

func (e *OcfEncoder) MarshalZerologObject(event *zerolog.Event) {
	event.Str("format", "avro ocf")
}

func (e *OcfEncoder) Append(r *v1.Relationship, _ string) error {
	var toEncode RelationshipV1

	toEncode.ObjectType = r.Resource.ObjectType
	toEncode.ObjectID = r.Resource.ObjectId
	toEncode.Relation = r.Relation
	toEncode.SubjectObjectType = r.Subject.Object.ObjectType
	toEncode.SubjectObjectID = r.Subject.Object.ObjectId
	toEncode.SubjectRelation = r.Subject.OptionalRelation
	if r.OptionalCaveat != nil {
		contextBytes, err := proto.Marshal(r.OptionalCaveat.Context)
		if err != nil {
			return fmt.Errorf("error marshaling caveat context: %w", err)
		}

		toEncode.CaveatName = r.OptionalCaveat.CaveatName
		toEncode.CaveatContext = contextBytes
	}

	if err := e.enc.Encode(toEncode); err != nil {
		return fmt.Errorf("unable to encode relationship: %w", err)
	}

	return nil
}

func (e *OcfEncoder) MarkComplete() {}
func (e *OcfEncoder) Close() error {
	if err := e.enc.Flush(); err != nil {
		return fmt.Errorf("unable to flush encoder: %w", err)
	}
	return nil
}

// OcfFileEncoder implements `Encoder` by formatting data in the AVRO OCF
// format, while also persisting it to a file and maintaining a lockfile that
// tracks the progress so that it can be resumed if stopped.
type OcfFileEncoder struct {
	file             *os.File
	lastSyncedCursor string
	completed        bool
	*OcfEncoder
}

func (fe *OcfFileEncoder) lockFileName() string {
	return fe.file.Name() + ".lock"
}

func NewOrExistingFileEncoder(filename, schema string, revision *v1.ZedToken) (e *OcfFileEncoder, cursor string, err error) {
	if _, err := os.Stat(filename); filename != "-" && err == nil {
		return NewExistingFileEncoder(filename)
	}
	enc, err := NewFileEncoder(filename, schema, revision)
	return enc, "", err
}

func NewExistingFileEncoder(filename string) (e *OcfFileEncoder, cursor string, err error) {
	var f *os.File
	if filename == "" {
		f = os.Stdout
	} else {
		if _, err := os.Stat(filename); err != nil {
			return nil, "", fmt.Errorf("unable to open existing backup file: %w", err)
		}

		f, err = os.OpenFile(filename, os.O_RDWR|os.O_APPEND, 0o644)
		if err != nil {
			return nil, "", fmt.Errorf("unable to open existing backup file: %w", err)
		}
	}

	encoder, err := NewEncoderForExisting(f)
	if err != nil {
		return nil, "", err
	}

	fileEncoder := &OcfFileEncoder{file: f, OcfEncoder: encoder}
	cursorBytes, err := os.ReadFile(fileEncoder.lockFileName())
	if os.IsNotExist(err) {
		return nil, "", fmt.Errorf("completed backup file %s already exists", filename)
	} else if err != nil {
		return nil, "", err
	}

	return fileEncoder, string(cursorBytes), nil
}

func NewFileEncoder(filename, schema string, revision *v1.ZedToken) (*OcfFileEncoder, error) {
	if filename == "-" {
		return encoderForFile(os.Stdout, schema, revision)
	}

	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("unable to create backup file: %w", err)
	}

	return encoderForFile(f, schema, revision)
}

func encoderForFile(file *os.File, schema string, revision *v1.ZedToken) (*OcfFileEncoder, error) {
	encoder, err := NewEncoder(file, schema, revision)
	if err != nil {
		return nil, err
	}

	return &OcfFileEncoder{
		file:       file,
		OcfEncoder: encoder,
	}, nil
}

func (fe *OcfFileEncoder) Append(r *v1.Relationship, cursor string) error {
	if err := fe.OcfEncoder.Append(r, cursor); err != nil {
		return fmt.Errorf("error storing relationship: %w", err)
	}

	if cursor != fe.lastSyncedCursor { // Only write to disk when necessary
		if err := atomic.WriteFile(fe.lockFileName(), bytes.NewBufferString(cursor)); err != nil {
			return fmt.Errorf("failed to store cursor in lockfile: %w", err)
		}
		fe.lastSyncedCursor = cursor
	}

	return nil
}

func (fe *OcfFileEncoder) MarkComplete() { fe.completed = true }

func (fe *OcfFileEncoder) Close() error {
	// Don't throw any errors if the file is nil when flushing/closing.
	closeFile := func(f *os.File) error {
		if f != nil {
			return errors.Join(f.Sync(), f.Close())
		}
		return nil
	}

	removeCompleted := func(filename string) error {
		if fe.completed {
			return os.Remove(filename)
		}
		return nil
	}

	return errors.Join(
		fe.OcfEncoder.Close(),
		closeFile(fe.file),
		removeCompleted(fe.lockFileName()),
	)
}

func (fe *OcfFileEncoder) MarshalZerologObject(e *zerolog.Event) {
	e.EmbedObject(fe.OcfEncoder).
		Str("file", fe.file.Name()).
		Str("lockFile", fe.lockFileName())
}

// ProgressRenderingEncoder implements `Encoder` by wrapping an existing Encoder
// and displaying its progress to the current tty.
type ProgressRenderingEncoder struct {
	prefix          string
	relsProcessed   uint64
	relsFilteredOut uint64
	progressBar     *progressbar.ProgressBar
	startTime       time.Time
	ticker          <-chan time.Time
	Encoder
}

func WithProgress(prefix string, e Encoder) *ProgressRenderingEncoder {
	return &ProgressRenderingEncoder{
		prefix:    prefix,
		startTime: time.Now(),
		ticker:    time.Tick(5 * time.Second),
		Encoder:   e,
	}
}

func (pre *ProgressRenderingEncoder) bar() *progressbar.ProgressBar {
	if pre.progressBar == nil {
		pre.progressBar = console.CreateProgressBar("processing backup")
	}
	return pre.progressBar
}

func (pre *ProgressRenderingEncoder) Close() error {
	if err := pre.bar().Finish(); err != nil {
		return err
	}

	if closer, ok := pre.Encoder.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (pre *ProgressRenderingEncoder) MarshalZerologObject(e *zerolog.Event) {
	if obj, ok := pre.Encoder.(zerolog.LogObjectMarshaler); ok {
		e.EmbedObject(obj)
	}

	e.
		Uint64("filtered", pre.relsFilteredOut).
		Uint64("processed", pre.relsProcessed).
		Uint64("throughput", perSec(pre.relsProcessed, time.Since(pre.startTime))).
		Stringer("elapsed", time.Since(pre.startTime).Round(time.Second))
}

func (pre *ProgressRenderingEncoder) Append(r *v1.Relationship, cursor string) error {
	if hasRelPrefix(r, pre.prefix) {
		if err := pre.Encoder.Append(r, cursor); err != nil {
			return err
		}
	} else {
		pre.relsFilteredOut++
	}
	pre.relsProcessed++

	if err := pre.bar().Add(1); err != nil {
		return fmt.Errorf("error incrementing progress bar: %w", err)
	}
	if !isatty.IsTerminal(os.Stderr.Fd()) { // Fallback for non-interactive tty
		select {
		case <-pre.ticker:
			log.Info().EmbedObject(pre).Msg("backup progress")
		default:
		}
	}
	return nil
}

func hasRelPrefix(rel *v1.Relationship, prefix string) bool {
	// Skip any relationships without the prefix on both sides.
	return strings.HasPrefix(rel.Resource.ObjectType, prefix) &&
		strings.HasPrefix(rel.Subject.Object.ObjectType, prefix)
}

func perSec(i uint64, d time.Duration) uint64 {
	secs := uint64(d.Seconds())
	if secs == 0 {
		return i
	}
	return i / secs
}
