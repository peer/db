package main

import (
	"github.com/alecthomas/kong"
	"github.com/rs/zerolog"
)

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
type Config struct {
	Logger  zerolog.Logger   `kong:"-"`
	Version kong.VersionFlag `short:"V" help:"Show program's version and exit."`
	Logging struct {
		Console struct {
			Type  string        `placeholder:"TYPE" enum:"color,nocolor,json,disable" default:"color" help:"Type of console logging. Possible: ${enum}. Default: ${default}."`
			Level zerolog.Level `placeholder:"LEVEL" enum:"trace,debug,info,warn,error" default:"info" help:"All logs with a level greater than or equal to this level will be written to the console. Possible: ${enum}. Default: ${default}."`
		} `embed:"" prefix:"console."`
		File struct {
			Path  string        `placeholder:"PATH" type:"path" help:"Log to a file (as well)."`
			Level zerolog.Level `placeholder:"LEVEL" enum:"trace,debug,info,warn,error" default:"info" help:"All logs with a level greater than or equal to this level will be written to the file. Possible: ${enum}. Default: ${default}."`
		} `embed:"" prefix:"file."`
	} `embed:"" prefix:"logging."`
}
