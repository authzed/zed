package decode

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"github.com/authzed/spicedb/pkg/spiceerrors"
	"github.com/authzed/spicedb/pkg/validationfile"
	"github.com/authzed/spicedb/pkg/validationfile/blocks"
)

const (
	FileTypeUnknown = iota
	FileTypeYaml
	FileTypeZed
)

var playgroundPattern = regexp.MustCompile("^.*/s/.*/schema|relationships|assertions|expected.*$")

// Func will decode into the supplied object.
type Func func(out any) ([]byte, error)

// FetchURL interprets the URL and fetches using the appropriate transport.
func FetchURL(u *url.URL) (contents []byte, err error) {
	switch s := u.Scheme; s {
	case "", "file":
		return fetchFile(u)
	case "http", "https":
		return fetchHTTP(u)
	default:
		return nil, fmt.Errorf("%s scheme not supported", s)
	}
}

func fetchFile(u *url.URL) ([]byte, error) {
	fs := os.DirFS(".")
	file, err := fs.Open(u.Path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return data, err
}

func fetchHTTP(u *url.URL) ([]byte, error) {
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

func UnmarshalValidationFile(contents []byte) (validationfile.ValidationFile, error) {
	// Try first to unmarshal as yaml. If it can't be unmarshalled, we assume it's a schema.
	// TODO: make sure this doesn't attempt to unmarshal relation schema: user
	vFile, err := UnmarshalYAMLValidationFile(contents)
	if err == nil {
		return vFile, nil
	}

	// yaml unmarshalling was unsuccessful, so we construct a vfile that assumes the schema
	// is the bytes and return it.
	return UnmarshalSchemaValidationFile(contents), nil
}

// TODO: use os.OpenRoot to avoid needing to check whether the file is local
// or DirFS(). Not sure which.
func UnmarshalYAMLValidationFile(yamlBytes []byte) (validationfile.ValidationFile, error) {
	var vFile validationfile.ValidationFile
	// Try to unmarshal as YAML for the validation file format.
	err := yaml.Unmarshal(yamlBytes, &vFile)
	return vFile, err
}

// TODO: does this actually do anything?
// I think the idea is that we'd just pass it to the validation logic.
func UnmarshalSchemaValidationFile(schemaBytes []byte) validationfile.ValidationFile {
	var vFile validationfile.ValidationFile

	vFile.Schema = blocks.SchemaWithPosition{
		SourcePosition: spiceerrors.SourcePosition{LineNumber: 1, ColumnPosition: 1},
		Schema:         string(schemaBytes),
	}

	return vFile
}
