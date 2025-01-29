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

	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/authzed/spicedb/pkg/schemadsl/input"
	"github.com/authzed/spicedb/pkg/spiceerrors"
	"github.com/authzed/spicedb/pkg/validationfile"
	"github.com/authzed/spicedb/pkg/validationfile/blocks"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

var playgroundPattern = regexp.MustCompile("^.*/s/.*/schema|relationships|assertions|expected.*$")

// SchemaRelationships holds the schema (as a string) and a list of
// relationships (as a string) in the format from the devtools download API.
type SchemaRelationships struct {
	Schema        string `yaml:"schema"`
	Relationships string `yaml:"relationships"`
}

// Func will decode into the supplied object.
type Func func(out interface{}) ([]byte, bool, error)

// DecoderForURL returns the appropriate decoder for a given URL.
// Some URLs have special handling to dereference to the actual file.
func DecoderForURL(u *url.URL) (d Func, err error) {
	switch s := u.Scheme; s {
	case "file":
		d = fileDecoder(u)
	case "http", "https":
		d = httpDecoder(u)
	case "":
		d = fileDecoder(u)
	default:
		err = fmt.Errorf("%s scheme not supported", s)
	}
	return
}

func fileDecoder(u *url.URL) Func {
	return func(out interface{}) ([]byte, bool, error) {
		file, err := os.Open(u.Path)
		if err != nil {
			return nil, false, err
		}
		data, err := io.ReadAll(file)
		if err != nil {
			return nil, false, err
		}
		isOnlySchema, err := unmarshalAsYAMLOrSchemaWithFile(data, out, u.Path)
		return data, isOnlySchema, err
	}
}

func httpDecoder(u *url.URL) Func {
	rewriteURL(u)
	return directHTTPDecoder(u)
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

func directHTTPDecoder(u *url.URL) Func {
	return func(out interface{}) ([]byte, bool, error) {
		log.Debug().Stringer("url", u).Send()
		r, err := http.Get(u.String())
		if err != nil {
			return nil, false, err
		}
		defer r.Body.Close()
		data, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, false, err
		}

		isOnlySchema, err := unmarshalAsYAMLOrSchema(data, out)
		return data, isOnlySchema, err
	}
}

// Uses the files passed in the args and looks for the specified schemaFile to parse the YAML.
func unmarshalAsYAMLOrSchemaWithFile(data []byte, out interface{}, filename string) (bool, error) {
	if strings.Contains(string(data), "schemaFile:") && !strings.Contains(string(data), "schema:") {
		if err := yaml.Unmarshal(data, out); err != nil {
			return false, err
		}
		validationFile, ok := out.(*validationfile.ValidationFile)
		if !ok {
			return false, fmt.Errorf("could not cast unmarshalled file to validationfile")
		}

		// Need to join the original filepath with the requested filepath
		// to construct the path to the referenced schema file.
		// NOTE: This does not allow for yaml files to transitively reference
		// each other's schemaFile fields.
		// TODO: enable this behavior
		schemaPath := filepath.Join(path.Dir(filename), validationFile.SchemaFile)

		if !filepath.IsLocal(schemaPath) {
			// We want to prevent access of files that are outside of the folder
			// where the command was originally invoked. This should do that.
			return false, fmt.Errorf("schema filepath %s must be local to where the command was invoked", schemaPath)
		}

		file, err := os.Open(schemaPath)
		if err != nil {
			return false, err
		}
		data, err = io.ReadAll(file)
		if err != nil {
			return false, err
		}
	}
	return unmarshalAsYAMLOrSchema(data, out)
}

func unmarshalAsYAMLOrSchema(data []byte, out interface{}) (bool, error) {
	// Check for indications of a schema-only file.
	if !strings.Contains(string(data), "schema:") && !strings.Contains(string(data), "relationships:") {
		if err := compileSchemaFromData(data, out); err != nil {
			return false, err
		}
		return true, nil
	}

	if !strings.Contains(string(data), "schema:") && !strings.Contains(string(data), "schemaFile:") {
		// If there is no schema and no schemaFile and it doesn't compile then it must be yaml with missing fields
		if err := compileSchemaFromData(data, out); err != nil {
			return false, errors.New("either schema or schemaFile must be present")
		}
		return true, nil
	}
	// Try to unparse as YAML for the validation file format.
	if err := yaml.Unmarshal(data, out); err != nil {
		return false, err
	}

	return false, nil
}

func compileSchemaFromData(data []byte, out interface{}) error {
	compiled, serr := compiler.Compile(compiler.InputSchema{
		Source:       input.Source("schema"),
		SchemaString: string(data),
	}, compiler.AllowUnprefixedObjectType())
	if serr != nil {
		return serr
	}

	// If that succeeds, return the compiled schema.
	vfile := *out.(*validationfile.ValidationFile)
	vfile.Schema = blocks.ParsedSchema{
		CompiledSchema: compiled,
		Schema:         string(data),
		SourcePosition: spiceerrors.SourcePosition{LineNumber: 1, ColumnPosition: 1},
	}
	*out.(*validationfile.ValidationFile) = vfile
	return nil
}
