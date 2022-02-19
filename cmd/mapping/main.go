package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"time"

	"github.com/alecthomas/kong"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

const (
	fileMode = 0o600
	// Exit code 1 is used by Kong.
	errorExitCode = 2
	// Copied from zerolog/console.go.
	colorRed = 31
)

// These variables should be set during build time using "-X" ldflags.
var (
	version        = ""
	buildTimestamp = ""
	revision       = ""
)

type filteredWriter struct {
	Writer zerolog.LevelWriter
	Level  zerolog.Level
}

func (w *filteredWriter) Write(p []byte) (n int, err error) {
	return w.Writer.Write(p)
}

func (w *filteredWriter) WriteLevel(level zerolog.Level, p []byte) (n int, err error) {
	if level >= w.Level {
		return w.Writer.WriteLevel(level, p)
	}
	return len(p), nil
}

// Copied from zerolog/writer.go.
type levelWriterAdapter struct {
	io.Writer
}

func (lw levelWriterAdapter) WriteLevel(_ zerolog.Level, p []byte) (n int, err error) {
	return lw.Write(p)
}

// Copied from zerolog/console.go.
func colorize(s interface{}, c int, disabled bool) string {
	if disabled {
		return fmt.Sprintf("%s", s)
	}
	return fmt.Sprintf("\x1b[%dm%v\x1b[0m", c, s)
}

func formatError(noColor bool) zerolog.Formatter {
	return func(i interface{}) string {
		j, ok := i.([]uint8)
		if !ok {
			return colorize("[error: value is not []uint8]", colorRed, noColor)
		}
		var e struct {
			Error string `json:"error,omitempty"`
		}
		err := json.Unmarshal(json.RawMessage(j), &e)
		if err != nil {
			return colorize(fmt.Sprintf("[error: %s]", err.Error()), colorRed, noColor)
		}
		return colorize(e.Error, colorRed, noColor)
	}
}

func main() {
	var config Config
	kong.Parse(&config,
		kong.Description(
			"All logging goes to stdout. CLI parsing errors, logging errors, and unhandled panics go to stderr.",
		),
		kong.Vars{
			"version": fmt.Sprintf("version %s (build on %s, git revision %s)", version, buildTimestamp, revision),
		},
		kong.UsageOnError(),
		kong.Writers(
			os.Stderr,
			os.Stderr,
		),
		kong.TypeMapper(reflect.TypeOf(zerolog.Level(0)), kong.MapperFunc(func(ctx *kong.DecodeContext, target reflect.Value) error {
			var l string
			err := ctx.Scan.PopValueInto("level", &l)
			if err != nil {
				return err
			}
			level, err := zerolog.ParseLevel(l)
			if err != nil {
				return errors.WithStack(err)
			}
			target.Set(reflect.ValueOf(level))
			return nil
		})),
	)

	// Default exist code.
	exitCode := 0
	defer func() { os.Exit(exitCode) }()

	writers := []io.Writer{}
	switch config.Logging.Console.Type {
	case "color", "nocolor":
		w := zerolog.NewConsoleWriter()
		w.NoColor = config.Logging.Console.Type == "nocolor"
		w.FormatErrFieldValue = formatError(w.NoColor)
		writers = append(writers, &filteredWriter{
			Writer: levelWriterAdapter{w},
			Level:  config.Logging.Console.Level,
		})
	case "json":
		w := os.Stdout
		writers = append(writers, &filteredWriter{
			Writer: levelWriterAdapter{w},
			Level:  config.Logging.Console.Level,
		})
	}
	if config.Logging.File.Path != "" {
		w, err := os.OpenFile(config.Logging.File.Path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, fileMode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot open logging file: %s\n", err.Error())
			// Use the same exit code as Kong does.
			exitCode = 1
			return
		}
		defer w.Close()
		writers = append(writers, &filteredWriter{
			Writer: levelWriterAdapter{w},
			Level:  config.Logging.File.Level,
		})
	}

	writer := zerolog.MultiLevelWriter(writers...)
	logger := zerolog.New(writer).With().Timestamp().Logger()

	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	zerolog.TimestampFunc = func() time.Time {
		return time.Now().UTC()
	}
	zerolog.TimeFieldFormat = "2006-01-02T15:04:05.000Z07:00"
	zerolog.ErrorMarshalFunc = func(ee error) interface{} {
		var j []byte
		var err error
		switch e := ee.(type) { //nolint:errorlint
		case interface {
			MarshalJSON() ([]byte, error)
		}:
			j, err = e.MarshalJSON()
		default:
			j, err = x.MarshalWithoutEscapeHTML(struct {
				Error string `json:"error,omitempty"`
			}{
				Error: ee.Error(),
			})
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshaling error \"%s\" into JSON during logging failed: %s\n", ee.Error(), err.Error())
		}
		return json.RawMessage(j)
	}
	log.Logger = logger

	config.Logger = logger

	err := generate(&config)
	if err != nil {
		log.Error().Err(err).Msg("mapping generation failed")
		exitCode = errorExitCode
	}
}
