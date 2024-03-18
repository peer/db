package main

import (
	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/tozd/go/cli"
)

func main() {
	var config Config
	cli.Run(&config, kong.Vars{
		"defaultOutput": DefaultOutput,
	}, func(_ *kong.Context) errors.E {
		return generate(&config)
	})
}
