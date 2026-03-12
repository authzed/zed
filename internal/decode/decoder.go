package decode

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"github.com/authzed/spicedb/pkg/spiceerrors"
	"github.com/authzed/spicedb/pkg/validationfile"
	"github.com/authzed/spicedb/pkg/validationfile/blocks"
)

// yamlKeyPatterns match YAML top-level keys that indicate a validation file format.
// These patterns look for the key at the start of a line (column 0), followed by a colon.
// This avoids false positives like "relation schema: parent" in a schema file being
// mistaken for the "schema:" YAML key.
var (
	yamlSchemaKeyPattern        = regexp.MustCompile(`(?m)^\s*schema\s*:`)
	yamlSchemaFileKeyPattern    = regexp.MustCompile(`(?m)^\s*schemaFile\s*:`)
	yamlRelationshipsKeyPattern = regexp.MustCompile(`(?m)^\s*relationships\s*:`)

	playgroundPattern = regexp.MustCompile("^.*/s/.*/schema|relationships|assertions|expected.*$")
)

const (
	FileTypeUnknown = iota
	FileTypeYaml
	FileTypeZed
)

// Decoder holds fetched file contents along with a filesystem for resolving
// relative paths (e.g. schemaFile references). For remote URLs the filesystem
// is nil because relative file references are not supported.
type Decoder struct {
	Contents []byte
	// FS is rooted at the directory of the fetched file. It is non-nil only
	// for local (file://) URLs.
	FS fs.FS
}

// DecoderFromURL interprets the URL, fetches the content, and returns a
// Decoder. For local files the Decoder's FS is rooted at the file's directory
// so that relative schemaFile paths can be resolved.
func DecoderFromURL(u *url.URL) (*Decoder, error) {
	switch s := u.Scheme; s {
	case "", "file":
		return decoderFromFile(u)
	case "http", "https":
		return decoderFromHTTP(u)
	default:
		return nil, fmt.Errorf("%s scheme not supported", s)
	}
}

func decoderFromFile(u *url.URL) (*Decoder, error) {
	filePath := u.Path
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(filePath)
	return &Decoder{
		Contents: data,
		FS:       os.DirFS(dir),
	}, nil
}

func decoderFromHTTP(u *url.URL) (*Decoder, error) {
	rewriteURL(u)
	data, err := fetchHTTPDirectly(u)
	if err != nil {
		return nil, err
	}
	return &Decoder{
		Contents: data,
		FS:       nil,
	}, nil
}

func rewriteURL(u *url.URL) {
	// match playground urls
	if playgroundPattern.MatchString(u.Path) {
		u.Path = u.Path[:strings.LastIndex(u.Path, "/")]
		u.Path += "/download"
		return
	}

	switch u.Hostname() {
	case "gist.github.com":
		u.Host = "gist.githubusercontent.com"
		u.Path = path.Join(u.Path, "/raw")
	case "pastebin.com":
		if ok, _ := path.Match("/raw/*", u.Path); ok {
			return
		}
		u.Path = path.Join("/raw/", u.Path)
	}
}

func fetchHTTPDirectly(u *url.URL) ([]byte, error) {
	log.Debug().Stringer("url", u).Send()
	r, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	return data, err
}

var ErrInvalidYamlTryZed = errors.New("invalid yaml")

// UnmarshalAsYAMLOrSchema tries to unmarshal as YAML first, falling back to
// treating the contents as a raw schema.
func (d *Decoder) UnmarshalAsYAMLOrSchema() (*validationfile.ValidationFile, error) {
	vFile, err := d.UnmarshalYAMLValidationFile()
	if err == nil {
		return vFile, nil
	}
	if !errors.Is(err, ErrInvalidYamlTryZed) {
		return nil, err
	}

	return d.UnmarshalSchemaValidationFile(), nil
}

// UnmarshalYAMLValidationFile unmarshals YAML validation file contents. If the
// YAML contains a schemaFile reference, the Decoder's FS is used to resolve it.
func (d *Decoder) UnmarshalYAMLValidationFile() (*validationfile.ValidationFile, error) {
	inputString := string(d.Contents)

	// Only attempt YAML unmarshaling if the input looks like a YAML validation file.
	if !hasYAMLSchemaKey(inputString) && !hasYAMLSchemaFileKey(inputString) && !yamlRelationshipsKeyPattern.MatchString(inputString) {
		return nil, fmt.Errorf("%w: input does not appear to be a YAML validation file", ErrInvalidYamlTryZed)
	}

	var validationFile validationfile.ValidationFile
	err := yaml.Unmarshal(d.Contents, &validationFile)
	if err != nil {
		return nil, err
	}

	// If schemaFile is specified, resolve it using the Decoder's filesystem.
	if validationFile.SchemaFile != "" {
		// Clean the path for use with fs.FS (which doesn't accept ./ prefix).
		schemaPath := filepath.Clean(validationFile.SchemaFile)

		if !filepath.IsLocal(schemaPath) {
			return nil, fmt.Errorf("schema filepath %q must be local to where the command was invoked", schemaPath)
		}

		if d.FS == nil {
			return nil, fmt.Errorf("cannot resolve schemaFile %q: no local filesystem context (remote URL?)", schemaPath)
		}

		file, err := d.FS.Open(schemaPath)
		if err != nil {
			return nil, err
		}
		schemaBytes, err := io.ReadAll(file)
		if err != nil {
			return nil, err
		}
		validationFile.SchemaFile = ""
		validationFile.Schema = blocks.SchemaWithPosition{
			SourcePosition: spiceerrors.SourcePosition{LineNumber: 1, ColumnPosition: 1},
			Schema:         string(schemaBytes),
		}
	}

	return &validationFile, nil
}

// UnmarshalSchemaValidationFile wraps raw schema bytes into a ValidationFile.
func (d *Decoder) UnmarshalSchemaValidationFile() *validationfile.ValidationFile {
	return &validationfile.ValidationFile{
		Schema: blocks.SchemaWithPosition{
			SourcePosition: spiceerrors.SourcePosition{LineNumber: 1, ColumnPosition: 1},
			Schema:         string(d.Contents),
		},
	}
}

// hasYAMLSchemaKey returns true if the input contains a "schema:" YAML key at the start of a line.
func hasYAMLSchemaKey(input string) bool {
	return yamlSchemaKeyPattern.MatchString(input)
}

// hasYAMLSchemaFileKey returns true if the input contains a "schemaFile:" YAML key at the start of a line.
func hasYAMLSchemaFileKey(input string) bool {
	return yamlSchemaFileKeyPattern.MatchString(input)
}
