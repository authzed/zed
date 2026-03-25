package decode

import (
	"errors"
	"fmt"
	"io"
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

type FileType int

const (
	FileTypeUnknown FileType = iota
	FileTypeYaml
	FileTypeZed
)

type SourceType int

const (
	SourceTypeUnknown SourceType = iota
	SourceTypeFile
	SourceTypeHTTP
)

func SourceTypeFromURL(u *url.URL) (SourceType, error) {
	switch s := u.Scheme; s {
	case "", "file":
		return SourceTypeFile, nil
	case "http", "https":
		return SourceTypeHTTP, nil
	default:
		return SourceTypeUnknown, fmt.Errorf("%s scheme not supported", s)
	}
}

// FetchFromURL interprets the URL, fetches the content, and returns
// the bytes.
func FetchFromURL(u *url.URL) ([]byte, error) {
	sourceType, err := SourceTypeFromURL(u)
	if err != nil {
		return nil, err
	}

	switch sourceType {
	case SourceTypeFile:
		return FetchFromFile(u)
	case SourceTypeHTTP:
		return FetchFromHTTP(u)
	default:
		// NOTE: this should not be hit, because `SourceTypeFromURL`
		// should cover this case.
		return nil, errors.New("unknown source type")
	}
}

func FetchFromFile(u *url.URL) ([]byte, error) {
	filePath := filepath.Clean(u.Path)

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	return io.ReadAll(file)
}

func FetchFromHTTP(u *url.URL) ([]byte, error) {
	rewriteURL(u)
	return fetchHTTPDirectly(u)
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
func UnmarshalAsYAMLOrSchema(contents []byte) (*validationfile.ValidationFile, error) {
	vFile, err := UnmarshalYAMLValidationFile(contents)
	if err == nil {
		return vFile, nil
	}
	if !errors.Is(err, ErrInvalidYamlTryZed) {
		return nil, err
	}

	return UnmarshalSchemaValidationFile(contents), nil
}

// UnmarshalYAMLValidationFile unmarshals YAML validation file contents into a ValidationFile
// struct.
func UnmarshalYAMLValidationFile(contents []byte) (*validationfile.ValidationFile, error) {
	inputString := string(contents)

	// Only attempt YAML unmarshaling if the input looks like a YAML validation file.
	if !hasYAMLSchemaKey(inputString) && !hasYAMLSchemaFileKey(inputString) && !yamlRelationshipsKeyPattern.MatchString(inputString) {
		return nil, fmt.Errorf("%w: input does not appear to be a YAML validation file", ErrInvalidYamlTryZed)
	}

	var validationFile validationfile.ValidationFile
	err := yaml.Unmarshal(contents, &validationFile)
	if err != nil {
		return nil, err
	}

	return &validationFile, nil
}

// ValidationFileFromFilename takes a filename and a desired/expected FileType and
// returns the contents fetched from the filename and the associated validationFile.
func ValidationFileFromFilename(filename string, fileType FileType) (vfile *validationfile.ValidationFile, contents []byte, err error) {
	u, err := url.Parse(filename)
	if err != nil {
		return nil, nil, err
	}

	sourceType, err := SourceTypeFromURL(u)
	if err != nil {
		return nil, nil, err
	}

	contents, err = FetchFromURL(u)
	if err != nil {
		return nil, nil, err
	}

	var parsed *validationfile.ValidationFile
	switch fileType {
	case FileTypeYaml:
		parsed, err = UnmarshalYAMLValidationFile(contents)
	case FileTypeZed:
		parsed = UnmarshalSchemaValidationFile(contents)
	default:
		parsed, err = UnmarshalAsYAMLOrSchema(contents)
	}
	// This block handles the error regardless of which case statement is hit
	if err != nil {
		return nil, nil, err
	}

	schemaPresent := parsed.Schema.Schema != ""
	schemaFilePresent := parsed.SchemaFile != ""

	// Ensure that either schema or schemaFile is present
	if !schemaPresent && !schemaFilePresent {
		return nil, nil, errors.New("either schema or schemaFile must be present")
	}

	if schemaPresent && schemaFilePresent {
		return nil, nil, errors.New("schema and schemaFile keys are both defined; please choose one")
	}

	// We will refuse to read in a `schemaFile` key when the file is fetched from a remote resource
	// We don't do this for HTTP-fetched ValidationFiles because we don't want them
	// referencing files in the local filesystem.
	if sourceType == SourceTypeHTTP && schemaFilePresent {
		return nil, nil, errors.New("cannot use schemaFile key when fetched from a remote resource")
	}

	// Attach the SchemaFile if we're dealing with a local file
	if sourceType == SourceTypeFile {
		err = ResolveSchemaFileIfPresent(filename, parsed)
		if err != nil {
			return nil, nil, err
		}
	}

	return parsed, contents, nil
}

// ResolveSchemaFileIfPresent takes a ValidationFile and if the SchemaFile key is present,
// uses it to populate the `Schema` key.
func ResolveSchemaFileIfPresent(filename string, validationFile *validationfile.ValidationFile) error {
	if validationFile.SchemaFile != "" {
		schemaPath := filepath.Clean(filepath.Join(filepath.Dir(filename), validationFile.SchemaFile))

		file, err := os.Open(schemaPath)
		if err != nil {
			return err
		}
		schemaBytes, err := io.ReadAll(file)
		if err != nil {
			return err
		}
		validationFile.Schema = blocks.SchemaWithPosition{
			SourcePosition: spiceerrors.SourcePosition{LineNumber: 1, ColumnPosition: 1},
			Schema:         string(schemaBytes),
		}
	}
	return nil
}

// UnmarshalSchemaValidationFile wraps raw schema bytes into a ValidationFile.
func UnmarshalSchemaValidationFile(contents []byte) *validationfile.ValidationFile {
	return &validationfile.ValidationFile{
		Schema: blocks.SchemaWithPosition{
			// If the file is just a schema file, we set the LineNumber offset to 0
			// for the purposes of displaying errors.
			SourcePosition: spiceerrors.SourcePosition{LineNumber: 0, ColumnPosition: 1},
			Schema:         string(contents),
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
