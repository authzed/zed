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

	composable "github.com/authzed/spicedb/pkg/composableschemadsl/compiler"
	"github.com/authzed/spicedb/pkg/composableschemadsl/generator"
	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/authzed/spicedb/pkg/schemadsl/input"
	"github.com/authzed/spicedb/pkg/spiceerrors"
	"github.com/authzed/spicedb/pkg/validationfile"
	"github.com/authzed/spicedb/pkg/validationfile/blocks"
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
	case "", "file":
		d = fileDecoder(u)
	case "http", "https":
		d = httpDecoder(u)
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

		isOnlySchema, err := unmarshalAsYAMLOrSchema("", data, out)
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
		schemaPath := filepath.Join(filepath.Dir(filename), validationFile.SchemaFile)

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
	return unmarshalAsYAMLOrSchema(filename, data, out)
}

func unmarshalAsYAMLOrSchema(filename string, data []byte, out interface{}) (bool, error) {
	inputString := string(data)

	// Check for indications of a schema-only file.
	if !strings.Contains(inputString, "schema:") && !strings.Contains(inputString, "relationships:") {
		if err := compileSchemaFromData(filename, inputString, out); err != nil {
			return false, err
		}
		return true, nil
	}

	if !strings.Contains(inputString, "schema:") && !strings.Contains(inputString, "schemaFile:") {
		// If there is no schema and no schemaFile and it doesn't compile then it must be yaml with missing fields
		if err := compileSchemaFromData(filename, inputString, out); err != nil {
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

// compileSchemaFromData attempts to compile using the old DSL and the new composable DSL,
// but prefers the new DSL.
// It returns the errors returned by both compilations.
func compileSchemaFromData(filename, schemaString string, out interface{}) error {
	var (
		standardCompileErr   error
		composableCompiled   *composable.CompiledSchema
		composableCompileErr error
		vfile                validationfile.ValidationFile
	)

	vfile = *out.(*validationfile.ValidationFile)
	vfile.Schema = blocks.SchemaWithPosition{
		SourcePosition: spiceerrors.SourcePosition{LineNumber: 1, ColumnPosition: 1},
	}

	_, standardCompileErr = compiler.Compile(compiler.InputSchema{
		Source:       input.Source("schema"),
		SchemaString: schemaString,
	}, compiler.AllowUnprefixedObjectType())

	if standardCompileErr == nil {
		vfile.Schema.Schema = schemaString
	}

	inputSourceFolder := filepath.Dir(filename)
	composableCompiled, composableCompileErr = composable.Compile(composable.InputSchema{
		SchemaString: schemaString,
	}, composable.AllowUnprefixedObjectType(), composable.SourceFolder(inputSourceFolder))

	if composableCompileErr == nil {
		compiledSchemaString, _, err := generator.GenerateSchema(composableCompiled.OrderedDefinitions)
		if err != nil {
			return fmt.Errorf("could not generate string schema: %w", err)
		}
		vfile.Schema.Schema = compiledSchemaString
	}

	err := errors.Join(standardCompileErr, composableCompileErr)

	*out.(*validationfile.ValidationFile) = vfile
	return err
}
