package peerdb

import (
	"io"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olivere/elastic/v7"
	"github.com/riverqueue/river"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"
	"gopkg.in/yaml.v3"

	"gitlab.com/peerdb/peerdb/base"
)

// Build contains version and build metadata.
type Build struct {
	Version        string `json:"version,omitempty"`
	BuildTimestamp string `json:"buildTimestamp,omitempty"`
	Revision       string `json:"revision,omitempty"`
}

// Site represents a single site in the PeerDB application with its configuration and state.
type Site struct {
	waf.Site `yaml:",inline"`

	Build *Build `json:"build,omitempty" yaml:"-"`

	Index  string `json:"index,omitempty"  yaml:"index,omitempty"`
	Schema string `json:"schema,omitempty" yaml:"schema,omitempty"`
	Title  string `json:"title,omitempty"  yaml:"title,omitempty"`

	Base        *base.B               `json:"-" yaml:"-"`
	DBPool      *pgxpool.Pool         `json:"-" yaml:"-"`
	ESClient    *elastic.Client       `json:"-" yaml:"-"`
	RiverClient *river.Client[pgx.Tx] `json:"-" yaml:"-"`

	initialized bool

	// TODO: How to keep propertiesTotal in sync with the number of properties available, if they are added or removed after initialization?
	propertiesTotal int64
}

// Decode implements kong.MapperValue to decode Site from JSON/YAML configuration.
func (s *Site) Decode(ctx *kong.DecodeContext) error {
	var value string
	err := ctx.Scan.PopValueInto("value", &value)
	if err != nil {
		return errors.WithStack(err)
	}
	decoder := yaml.NewDecoder(strings.NewReader(value))
	decoder.KnownFields(true)
	err = decoder.Decode(s)
	if err != nil {
		if yamlErr, ok := errors.AsType[*yaml.TypeError](err); ok {
			e := "error"
			if len(yamlErr.Errors) > 1 {
				e = "errors"
			}
			return errors.Errorf("yaml: unmarshal %s: %s", e, strings.Join(yamlErr.Errors, "; "))
		} else if errors.Is(err, io.EOF) {
			return nil
		}
		return errors.WithStack(err)
	}
	return nil
}
