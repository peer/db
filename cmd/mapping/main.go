package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"sync"
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
	colorRed  = 31
	colorBold = 1
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

// formatError extracts just the error message from error's JSON.
func formatError(noColor bool) zerolog.Formatter {
	return func(i interface{}) string {
		j, ok := i.([]byte)
		if !ok {
			return colorize("[error: value is not []byte]", colorRed, noColor)
		}
		var e struct {
			Error string `json:"error,omitempty"`
		}
		err := json.Unmarshal(json.RawMessage(j), &e)
		if err != nil {
			return colorize(fmt.Sprintf("[error: %s]", err.Error()), colorRed, noColor)
		}
		return colorize(colorize(e.Error, colorRed, noColor), colorBold, noColor)
	}
}

type eventError struct {
	Error string `json:"error,omitempty"`
	Stack []struct {
		Name string `json:"name,omitempty"`
		File string `json:"file,omitempty"`
		Line int    `json:"line,omitempty"`
	} `json:"stack,omitempty"`
	Cause *eventError `json:"cause,omitempty"`
}

type eventWithError struct {
	Error *eventError `json:"error,omitempty"`
}

type consoleWriter struct {
	zerolog.ConsoleWriter
	buf  *bytes.Buffer
	lock sync.Mutex
}

func newConsoleWriter(noColor bool) *consoleWriter {
	buf := &bytes.Buffer{}
	w := zerolog.NewConsoleWriter()
	w.Out = buf
	w.NoColor = noColor
	w.FormatErrFieldValue = formatError(w.NoColor)

	return &consoleWriter{ConsoleWriter: w, buf: buf}
}

func (w *consoleWriter) Write(p []byte) (int, error) {
	w.lock.Lock()
	defer w.lock.Unlock()

	_, err := w.ConsoleWriter.Write(p)
	if err != nil {
		return 0, err
	}

	var event eventWithError
	err = json.Unmarshal(p, &event)
	if err != nil {
		return 0, errors.Errorf("cannot decode event: %w", err)
	}

	ee := event.Error
	first := true
	for ee != nil {
		if !first {
			w.buf.WriteString(colorize("\nThe above error was caused by the following error:\n\n", colorRed, w.NoColor))
			if ee.Error != "" {
				w.buf.WriteString(colorize(colorize(ee.Error, colorRed, w.NoColor), colorBold, w.NoColor))
				w.buf.WriteString("\n")
			}
		}
		first = false
		if len(ee.Stack) > 0 {
			w.buf.WriteString(colorize("Stack trace (most recent call first):\n", colorRed, w.NoColor))
			for _, s := range ee.Stack {
				w.buf.WriteString(colorize(s.Name, colorRed, w.NoColor))
				w.buf.WriteString("\n\t")
				w.buf.WriteString(colorize(s.File, colorRed, w.NoColor))
				w.buf.WriteString(colorize(":", colorRed, w.NoColor))
				w.buf.WriteString(colorize(strconv.Itoa(s.Line), colorRed, w.NoColor))
				w.buf.WriteString("\n")
			}
		}
		ee = ee.Cause
	}

	_, err = w.buf.WriteTo(os.Stdout)
	return len(p), err
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
		w := newConsoleWriter(config.Logging.Console.Type == "nocolor")
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
