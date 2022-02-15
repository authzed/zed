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
)

var playgroundPattern = regexp.MustCompile("^.*/s/.*/schema|relationships|assertions|expected.*$")

// SchemaRelationships holds the schema (as a string) and a list of
// relationships (as a string) in the format from the devtools download API.
type SchemaRelationships struct {
	Schema        string `yaml:"schema"`
	Relationships string `yaml:"relationships"`
}

// Func will decode into the supplied object.
type Func func(out interface{}) ([]byte, error)

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
	return func(out interface{}) ([]byte, error) {
		file, err := os.Open(u.Path)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(file)
		if err != nil {
			return nil, err
		}
		return data, yaml.Unmarshal(data, out)
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
	return func(out interface{}) ([]byte, error) {
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

		return data, yaml.Unmarshal(data, out)
	}
}
