package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
)

const exitCode = 2

// These variables should be set during build time using "-X" ldflags.
var (
	version        = ""
	buildTimestamp = ""
	revision       = ""
)

func main() {
	var config Config
	ctx := kong.Parse(&config,
		kong.Vars{
			"version": fmt.Sprintf("version %s (build on %s, git revision %s)", version, buildTimestamp, revision),
		},
		kong.UsageOnError(),
		kong.Writers(
			os.Stderr,
			os.Stderr,
		),
	)

	err := prepare(&config)
	if err != nil {
		fmt.Fprintf(ctx.Stderr, "error: %+v", err)
		ctx.Exit(exitCode)
	}
}
