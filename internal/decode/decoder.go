package decode

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// SchemaRelationships holds the schema (as a string) and a list of
// relationships (as a string) in the format from the devtools download API.
type SchemaRelationships struct {
	Schema        string `yaml:"schema"`
	Relationships string `yaml:"relationships"`
}

// DecodeFunc will decode into the SchemaRelationships object.
type DecodeFunc func(out *SchemaRelationships) error

// DecoderForURL returns the appropriate decoder for a given URL.
// Some URLs have special handling to dereference to the actual file.
func DecoderForURL(u *url.URL) (d DecodeFunc, err error) {
	switch s := u.Scheme; s {
	case "file":
		d = fileDecoder(u)
	case "http", "https":
		d = httpDecoder(u)
	default:
		err = fmt.Errorf("%s scheme not supported", s)
	}
	return
}

func fileDecoder(u *url.URL) DecodeFunc {
	return func(out *SchemaRelationships) error {
		file, err := os.Open(u.Path)
		if err != nil {
			return err
		}
		return yaml.NewDecoder(file).Decode(&out)
	}
}

func httpDecoder(u *url.URL) DecodeFunc {
	rewriteURL(u)
	return directHttpDecoder(u)
}

func rewriteURL(u *url.URL) {
	switch u.Hostname() {
	case "gist.github.com":
		u.Host = "gist.githubusercontent.com"
		u.Path = path.Join(u.Path, "/raw")
	case "play.authzed.com":
		if ok, _ := path.Match("/s/*/*", u.Path); ok {
			u.Path = u.Path[:strings.LastIndex(u.Path, "/")]
			u.Path += "/download"
		}
	case "pastebin.com":
		if ok, _ := path.Match("/raw/*", u.Path); ok {
			return
		}
		u.Path = path.Join("/raw/", u.Path)
	}
}

func directHttpDecoder(u *url.URL) DecodeFunc {
	return func(out *SchemaRelationships) error {
		r, err := http.Get(u.String())
		if err != nil {
			return err
		}
		return yaml.NewDecoder(r.Body).Decode(&out)
	}
}
