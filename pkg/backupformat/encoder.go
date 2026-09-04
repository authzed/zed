package backupformat

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
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

// Encoder represents the operations required to iteratively encode a backup
// of SpiceDB relationship data.
type Encoder interface {
	WriteSchema(schema, revision string) error

	// Append encodes an additional Relationship using the provided cursor to
	// keep track of progress.
	Append(r *v1.Relationship, cursor string) error

	// MarkComplete signals that the final relationship has been written and
	// that the process is complete.
	MarkComplete()
}

var (
	_ Encoder = (*MockEncoder)(nil)
	_ Encoder = (*RewriteEncoder)(nil)
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

func (m *MockEncoder) WriteSchema(_, _ string) error { return nil }
func (m *MockEncoder) MarkComplete()                 { m.Complete = true }

func WithRewriter(rw Rewriter, e Encoder) *RewriteEncoder {
	return &RewriteEncoder{Rewriter: rw, Encoder: e}
}

// RewriteEncoder implements `Encoder` by rewriting any relationships before
// passing it on to the provided Encoder.
type RewriteEncoder struct {
	Rewriter
	Encoder
}

func (e *RewriteEncoder) Append(r *v1.Relationship, cursor string) error {
	rel, err := e.RewriteRelationship(r)
	if err != nil {
		return err
	} else if rel == nil {
		return nil
	}
	return e.Encoder.Append(rel, cursor)
}

func (e *RewriteEncoder) MarshalZerologObject(event *zerolog.Event) {
	if obj, ok := e.Rewriter.(zerolog.LogObjectMarshaler); ok {
		event.EmbedObject(obj)
	}

	if obj, ok := e.Encoder.(zerolog.LogObjectMarshaler); ok {
		event.EmbedObject(obj)
	}
}

func (e *RewriteEncoder) Close() error {
	if closer, ok := e.Encoder.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// OcfEncoder implements `Encoder` by formatting data in the AVRO OCF format.
type OcfEncoder struct {
	w   io.Writer
	enc *ocf.Encoder
}

func NewOcfEncoder(w io.Writer) *OcfEncoder {
	return &OcfEncoder{w: w}
}

func (e *OcfEncoder) encoder(revision string) (*ocf.Encoder, error) {
	if e.enc != nil {
		return e.enc, nil
	}

	avroSchema, err := avroSchemaV1()
	if err != nil {
		return nil, fmt.Errorf("unable to create avro schema: %w", err)
	}

	opts := []ocf.EncoderFunc{ocf.WithCodec(ocf.Snappy)}
	if revision != "" {
		opts = append(opts, ocf.WithMetadata(map[string][]byte{metadataKeyZT: []byte(revision)}))
	}

	e.enc, err = ocf.NewEncoder(avroSchema, e.w, opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to create encoder: %w", err)
	}

	return e.enc, nil
}

func (e *OcfEncoder) WriteSchema(schema, revision string) error {
	enc, err := e.encoder(revision)
	if err != nil {
		return err
	}

	if err := enc.Encode(SchemaV1{SchemaText: schema}); err != nil {
		return fmt.Errorf("unable to encode SpiceDB schema object: %w", err)
	}

	return nil
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

	if r.OptionalExpiresAt != nil && !r.OptionalExpiresAt.AsTime().IsZero() {
		toEncode.Expiration = r.OptionalExpiresAt.AsTime()
	}

	encoder, err := e.encoder("")
	if err != nil {
		return err
	}

	if err := encoder.Encode(toEncode); err != nil {
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
	// file is the destination the encoder writes to. For regular backups this
	// is a file on disk; when streaming it is os.Stdout.
	file *os.File
	// fileIsStream is true when the underlying file is a stream (e.g. os.Stdout)
	// for which lockfile-based progress tracking and Sync/Close are not
	// applicable.
	fileIsStream bool
	// lastSyncedCursor is the most recent cursor value written to the lockfile.
	// It is used to avoid redundant lockfile writes when the cursor has not
	// advanced since the previous Append call.
	lastSyncedCursor string
	// completed indicates that the backup finished successfully. When true,
	// Close removes the lockfile because no resume is needed.
	completed bool
	// OcfEncoder is the embedded AVRO OCF encoder that performs the actual
	// serialization of relationships into the file.
	ocfEncoder *OcfEncoder
}

func (fe *OcfFileEncoder) WriteSchema(schema, revision string) error {
	return fe.ocfEncoder.WriteSchema(schema, revision)
}

// ErrBackupAlreadyCompleted indicates a backup file is on disk and is marked
// complete (via the sidecar completion sentinel). Callers should refuse to
// overwrite it.
var ErrBackupAlreadyCompleted = errors.New("backup file already exists and is marked complete")

// ErrBackupUnresumable indicates a backup file exists but neither a resume
// cursor nor a completion sentinel is present. Either the previous run died
// before recording any progress, or the file was produced by an older zed
// version that did not write a completion marker. The file may be complete
// or partial; the encoder cannot tell. Delete the file to start fresh.
var ErrBackupUnresumable = errors.New("backup file has no resume cursor or completion marker; if it was produced by an older zed version it may already be complete — delete it to start over")

func (fe *OcfFileEncoder) lockFileName() string {
	return fe.file.Name() + ".lock"
}

func (fe *OcfFileEncoder) doneFileName() string {
	return fe.file.Name() + ".done"
}

func (fe *OcfFileEncoder) Cursor() (string, error) {
	if fe.fileIsStream {
		return "", errors.New("resume is not supported when streaming to stdout")
	}
	// A progress lockfile always indicates an in-progress backup that should
	// resume from its cursor — even if a stale completion sentinel from a
	// previous run is also present.
	cursorBytes, err := os.ReadFile(fe.lockFileName())
	if err == nil {
		return string(cursorBytes), nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if _, err := os.Stat(fe.doneFileName()); err == nil {
		return "", ErrBackupAlreadyCompleted
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return "", ErrBackupUnresumable
}

func NewFileEncoder(filename string) (e *OcfFileEncoder, existed bool, err error) {
	_, err = os.Stat(filename)
	backupExisted := filename != "-" && err == nil

	var f *os.File
	isStream := filename == "-"
	if isStream {
		f = os.Stdout
	} else {
		var err error
		f, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return nil, backupExisted, fmt.Errorf("unable to open backup file: %w", err)
		}
		// When starting a fresh backup, remove any stale sidecar files from a
		// previous backup at the same path. A stale .done would misreport the
		// new run as already completed if it crashed before the first Append();
		// a stale .lock would resume the new run from an old cursor pointing
		// into the prior export's snapshot, silently skipping relationships.
		if !backupExisted {
			for _, sidecar := range []string{f.Name() + ".lock", f.Name() + ".done"} {
				if rmErr := os.Remove(sidecar); rmErr != nil && !os.IsNotExist(rmErr) {
					_ = f.Close()
					return nil, backupExisted, fmt.Errorf("unable to clear stale sidecar %s: %w", sidecar, rmErr)
				}
			}
		}
	}

	return &OcfFileEncoder{file: f, fileIsStream: isStream, ocfEncoder: &OcfEncoder{w: f}}, backupExisted, nil
}

func (fe *OcfFileEncoder) Append(r *v1.Relationship, cursor string) error {
	if err := fe.ocfEncoder.Append(r, cursor); err != nil {
		return fmt.Errorf("error storing relationship: %w", err)
	}

	// Streaming destinations (e.g. stdout) can't be resumed, so skip writing the cursor lockfile.
	if fe.fileIsStream {
		return nil
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
	safeClose := func() error {
		if fe.file == nil {
			return nil
		}
		var flushErr error
		if fe.ocfEncoder != nil && fe.ocfEncoder.enc != nil {
			flushErr = fe.ocfEncoder.Close()
		}
		// Stdout is owned by the process; Sync would fail with
		// "inappropriate ioctl for device" and we must not close it.
		if fe.fileIsStream {
			return flushErr
		}
		return errors.Join(flushErr, fe.file.Sync(), fe.file.Close())
	}

	finalizeCompleted := func() error {
		if fe.fileIsStream {
			return nil
		}
		if !fe.completed {
			return nil
		}
		// Write the completion sentinel before removing the progress lockfile so
		// that a crash between the two leaves the backup as resumable rather
		// than as a misleading "completed" state.
		if err := atomic.WriteFile(fe.doneFileName(), bytes.NewBuffer(nil)); err != nil {
			return fmt.Errorf("failed to write completion sentinel: %w", err)
		}
		if err := os.Remove(fe.lockFileName()); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	return errors.Join(
		safeClose(),
		finalizeCompleted(),
	)
}

func (fe *OcfFileEncoder) MarshalZerologObject(e *zerolog.Event) {
	e.EmbedObject(fe.ocfEncoder).
		Str("file", fe.file.Name()).
		Str("lockFile", fe.lockFileName())
}

// ProgressRenderingEncoder implements `Encoder` by wrapping an existing Encoder
// and displaying its progress to the current tty.
type ProgressRenderingEncoder struct {
	relsProcessed uint64
	progressBar   *progressbar.ProgressBar
	startTime     time.Time
	ticker        <-chan time.Time
	Encoder
}

func WithProgress(e Encoder) *ProgressRenderingEncoder {
	return &ProgressRenderingEncoder{
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
		Uint64("processed", pre.relsProcessed).
		Uint64("throughput", perSec(pre.relsProcessed, time.Since(pre.startTime))).
		Stringer("elapsed", time.Since(pre.startTime).Round(time.Second))
}

func (pre *ProgressRenderingEncoder) Append(r *v1.Relationship, cursor string) error {
	pre.relsProcessed++
	if err := pre.Encoder.Append(r, cursor); err != nil {
		return err
	}

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

func perSec(i uint64, d time.Duration) uint64 {
	secs := uint64(d.Seconds())
	if secs == 0 {
		return i
	}
	return i / secs
}
