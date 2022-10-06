package cli

import (
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/errors"
	"gopkg.in/yaml.v3"
)

type ConfigFlag string

func (c ConfigFlag) BeforeResolve(app *kong.Kong, ctx *kong.Context, trace *kong.Path) error {
	path := string(ctx.FlagValue(trace.Flag).(ConfigFlag))
	file, err := os.Open(kong.ExpandPath(path))
	if err != nil {
		return errors.WithStack(err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	err = decoder.Decode(app.Model.Target.Addr().Interface())
	if err != nil {
		var yamlErr *yaml.TypeError
		if errors.As(err, &yamlErr) {
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
