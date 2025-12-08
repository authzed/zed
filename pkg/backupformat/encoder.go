package backupformat

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/hamba/avro/v2/ocf"
	"github.com/mattn/go-isatty"
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

type Encoder interface {
	Append(r *v1.Relationship, cursor string) error
	MarkComplete()
}

var (
	_ Encoder = (*OcfEncoder)(nil)
	_ Encoder = (*OcfFileEncoder)(nil)
	_ Encoder = (*ProgressRenderingEncoder)(nil)
)

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

type OcfFileEncoder struct {
	file      *os.File
	lockFile  *os.File
	completed bool
	*OcfEncoder
}

func NewOrExistingFileEncoder(filename, schema string, revision *v1.ZedToken) (e *OcfFileEncoder, cursor string, err error) {
	if _, err := os.Stat(filename); filename != "-" && err == nil {
		return NewExistingFileEncoder(filename)
	}
	enc, err := NewFileEncoder(filename, schema, revision)
	return enc, "", err
}

func NewExistingFileEncoder(filename string) (e *OcfFileEncoder, cursor string, err error) {
	if _, err := os.Stat(filename); filename == "-" && err != nil {
		return nil, "", fmt.Errorf("unable to open existing backup file: %w", err)
	}

	f, err := os.OpenFile(filename, os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return nil, "", fmt.Errorf("unable to open existing backup file: %w", err)
	}

	encoder, err := NewEncoderForExisting(f)
	if err != nil {
		return nil, "", err
	}

	lockFileName := filename + ".lock"
	cursorBytes, err := os.ReadFile(lockFileName)
	if os.IsNotExist(err) {
		return nil, "", fmt.Errorf("completed backup file %s already exists", filename)
	} else if err != nil {
		return nil, "", err
	}

	// If backup existed and there is a progress marker, the latter should not
	// be truncated to make sure the cursor stays around in case of a failure
	// before we even start ingesting from bulk export.
	lf, err := os.OpenFile(lockFileName, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open lockfile: %w", err)
	}

	return &OcfFileEncoder{
		file:       f,
		lockFile:   lf,
		OcfEncoder: encoder,
	}, string(cursorBytes), nil
}

func NewFileEncoder(filename, schema string, revision *v1.ZedToken) (*OcfFileEncoder, error) {
	if filename == "-" {
		return encoderForFile(os.Stdout, nil, schema, revision)
	}

	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("unable to create backup file: %w", err)
	}

	lf, err := os.OpenFile(filename+".lock", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("unable to create lock file: %w", err)
	}

	return encoderForFile(f, lf, schema, revision)
}

func encoderForFile(file, lockFile *os.File, schema string, revision *v1.ZedToken) (*OcfFileEncoder, error) {
	encoder, err := NewEncoder(file, schema, revision)
	if err != nil {
		return nil, err
	}

	return &OcfFileEncoder{
		file:       file,
		lockFile:   lockFile,
		OcfEncoder: encoder,
	}, nil
}

func (fe *OcfFileEncoder) Append(r *v1.Relationship, cursor string) error {
	if err := fe.OcfEncoder.Append(r, cursor); err != nil {
		return fmt.Errorf("error storing relationship: %w", err)
	}

	// TODO(jzelinskie): Replace this with a new file+rename to make it atomic
	// similar to tailscale.com/atomicfile.

	if err := fe.lockFile.Truncate(0); err != nil {
		return fmt.Errorf("unable to truncate backup progress file: %w", err)
	}

	if _, err := fe.lockFile.Seek(0, 0); err != nil {
		return fmt.Errorf("unable to seek backup progress file: %w", err)
	}

	if _, err := fe.lockFile.WriteString(cursor); err != nil {
		return fmt.Errorf("unable to write result cursor to backup progress file: %w", err)
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
		closeFile(fe.lockFile),
		removeCompleted(fe.lockFile.Name()),
	)
}

func (fe *OcfFileEncoder) MarshalZerologObject(e *zerolog.Event) {
	e.EmbedObject(fe.OcfEncoder).
		Str("file", fe.file.Name()).
		Str("lockFile", fe.lockFile.Name())
}

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
