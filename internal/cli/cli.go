package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/kong"
	"github.com/mattn/go-colorable"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gopkg.in/yaml.v3"
)

// These variables should be set during build time using "-X" ldflags.
var (
	Version        = ""
	BuildTimestamp = ""
	Revision       = ""
)

const (
	fileMode = 0o600
	// Exit code 1 is used by Kong, 2 when program panics.
	errorExitCode = 3
)

// Copied from zerolog/console.go.
const (
	colorRed = iota + 31
	colorGreen
	colorYellow
	colorBlue

	colorBold     = 1
	colorDarkGray = 90
)

//nolint:lll
type console struct {
	Type  string        `placeholder:"TYPE" enum:"color,nocolor,json,disable" default:"color" help:"Type of console logging. Possible: ${enum}. Default: ${default}." yaml:"type"`
	Level zerolog.Level `short:"l" placeholder:"LEVEL" enum:"trace,debug,info,warn,error" default:"info" help:"All logs with a level greater than or equal to this level will be written to the console. Possible: ${enum}. Default: ${default}." yaml:"level"`
}

func (c *console) UnmarshalYAML(value *yaml.Node) error {
	var tmp struct {
		Type  string `yaml:"type"`
		Level string `yaml:"level"`
	}

	// TODO: Limit only to known fields.
	//       See: https://github.com/go-yaml/yaml/issues/460
	err := value.Decode(&tmp)
	if err != nil {
		return errors.WithStack(err)
	}
	level, err := zerolog.ParseLevel(tmp.Level)
	if err != nil {
		return errors.WithStack(err)
	}

	c.Type = tmp.Type
	c.Level = level

	return nil
}

//nolint:lll
type file struct {
	Path  string        `placeholder:"PATH" type:"path" help:"Append logs to a file (as well)." yaml:"path"`
	Level zerolog.Level `placeholder:"LEVEL" enum:"trace,debug,info,warn,error" default:"info" help:"All logs with a level greater than or equal to this level will be written to the file. Possible: ${enum}. Default: ${default}." yaml:"level"`
}

func (f *file) UnmarshalYAML(value *yaml.Node) error {
	var tmp struct {
		Path  string `yaml:"path"`
		Level string `yaml:"level"`
	}

	// TODO: Limit only to known fields.
	//       See: https://github.com/go-yaml/yaml/issues/460
	err := value.Decode(&tmp)
	if err != nil {
		return errors.WithStack(err)
	}
	level, err := zerolog.ParseLevel(tmp.Level)
	if err != nil {
		return errors.WithStack(err)
	}

	f.Path = tmp.Path
	f.Level = level

	return nil
}

type LoggingConfig struct {
	Log     zerolog.Logger `kong:"-" yaml:"-"`
	Logging struct {
		Console console `embed:"" prefix:"console." yaml:"console"`
		File    file    `embed:"" prefix:"file." yaml:"file"`
	} `embed:"" prefix:"logging." yaml:"logging"`
}

// filteredWriter writes only logs at Level or above.
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

// Based on zerolog/console.go, but with different colors.
func formatLevel(noColor bool) zerolog.Formatter {
	return func(i interface{}) string {
		var l string
		if ll, ok := i.(string); ok {
			switch ll {
			case zerolog.LevelTraceValue:
				l = colorize("TRC", colorBlue, noColor)
			case zerolog.LevelDebugValue:
				l = "DBG"
			case zerolog.LevelInfoValue:
				l = colorize("INF", colorGreen, noColor)
			case zerolog.LevelWarnValue:
				l = colorize("WRN", colorYellow, noColor)
			case zerolog.LevelErrorValue:
				l = colorize("ERR", colorRed, noColor)
			case zerolog.LevelFatalValue:
				l = colorize("FTL", colorRed, noColor)
			case zerolog.LevelPanicValue:
				l = colorize("PNC", colorRed, noColor)
			default:
				l = "???"
			}
		} else {
			if i == nil {
				l = "???"
			} else {
				l = strings.ToUpper(fmt.Sprintf("%s", i))[0:3]
			}
		}
		return l
	}
}

// Based on zerolog/console.go, but formatted to local timezone.
// See: https://github.com/rs/zerolog/pull/415
func formatTimestamp(timeFormat string, noColor bool) zerolog.Formatter {
	if timeFormat == "" {
		timeFormat = time.Kitchen
	}
	return func(i interface{}) string {
		t := "<nil>"
		switch tt := i.(type) {
		case string:
			ts, err := time.Parse(zerolog.TimeFieldFormat, tt)
			if err != nil {
				t = tt
			} else {
				t = ts.Local().Format(timeFormat)
			}
		case json.Number:
			i, err := tt.Int64()
			if err != nil {
				t = tt.String()
			} else {
				var sec, nsec int64 = i, 0
				switch zerolog.TimeFieldFormat {
				case zerolog.TimeFormatUnixMs:
					nsec = int64(time.Duration(i) * time.Millisecond)
					sec = 0
				case zerolog.TimeFormatUnixMicro:
					nsec = int64(time.Duration(i) * time.Microsecond)
					sec = 0
				}
				ts := time.Unix(sec, nsec)
				t = ts.Format(timeFormat)
			}
		}
		return colorize(t, colorDarkGray, noColor)
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
	Level string      `json:"level,omitempty"`
}

// consoleWriter writes stack traces for errors after the line with the log.
type consoleWriter struct {
	zerolog.ConsoleWriter
	buf  *bytes.Buffer
	out  io.Writer
	lock sync.Mutex
}

func newConsoleWriter(noColor bool) *consoleWriter {
	buf := &bytes.Buffer{}
	w := zerolog.NewConsoleWriter()
	// Embedded ConsoleWriter writes to a buffer, to which this consoleWriter
	// appends a stack trace and only then writes it to stdout.
	w.Out = buf
	w.NoColor = noColor
	w.TimeFormat = "15:04"
	w.FormatErrFieldValue = formatError(w.NoColor)
	w.FormatLevel = formatLevel(w.NoColor)
	w.FormatTimestamp = formatTimestamp(w.TimeFormat, w.NoColor)

	return &consoleWriter{
		ConsoleWriter: w,
		buf:           buf,
		out:           colorable.NewColorable(os.Stdout),
	}
}

func makeMessageBold(p []byte) ([]byte, errors.E) {
	var event map[string]interface{}
	d := json.NewDecoder(bytes.NewReader(p))
	d.UseNumber()
	err := d.Decode(&event)
	if err != nil {
		return p, errors.Errorf("cannot decode event: %w", err)
	}

	if event[zerolog.MessageFieldName] == "" || event[zerolog.MessageFieldName] == nil {
		return p, nil
	}

	switch event[zerolog.LevelFieldName] {
	case zerolog.LevelInfoValue, zerolog.LevelWarnValue, zerolog.LevelErrorValue, zerolog.LevelFatalValue, zerolog.LevelPanicValue:
		// Passthrough.
	default:
		return p, nil
	}

	event[zerolog.MessageFieldName] = colorize(fmt.Sprintf("%s", event[zerolog.MessageFieldName]), colorBold, false)
	return x.MarshalWithoutEscapeHTML(event)
}

func (w *consoleWriter) Write(p []byte) (int, error) {
	w.lock.Lock()
	defer w.lock.Unlock()
	defer w.buf.Reset()

	// Remember the length before we maybe modify p.
	n := len(p)

	var errE errors.E
	if !w.NoColor {
		p, errE = makeMessageBold(p)
		if errE != nil {
			return 0, errE
		}
	}

	_, err := w.ConsoleWriter.Write(p)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	var event eventWithError
	err = json.Unmarshal(p, &event)
	if err != nil {
		return 0, errors.Errorf("cannot decode event: %w", err)
	}

	level, _ := zerolog.ParseLevel(event.Level)

	// Print a stack trace only on error or above levels.
	if level < zerolog.ErrorLevel {
		_, err = w.buf.WriteTo(w.out)
		return n, errors.WithStack(err)
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

	_, err = w.buf.WriteTo(w.out)
	return n, errors.WithStack(err)
}

func extractLoggingConfig(config interface{}) (*LoggingConfig, errors.E) {
	configType := reflect.TypeOf(LoggingConfig{})
	val := reflect.ValueOf(config).Elem()
	typ := val.Type()
	fields := reflect.VisibleFields(typ)
	for _, field := range fields {
		if field.Type == configType {
			return val.FieldByIndex(field.Index).Addr().Interface().(*LoggingConfig), nil
		}
	}

	return nil, errors.Errorf("logging config not found in struct %T", config)
}

func Run(config interface{}, description string, run func(*kong.Context) errors.E) {
	// Inside this function, panicking should be set to false before all regular returns from it.
	panicking := true

	if description == "" {
		description = "All logging goes to stdout. CLI parsing errors, logging errors, and unhandled panics go to stderr."
	} else {
		description += "All logging goes to stdout. CLI parsing errors, logging errors, and unhandled panics go to stderr."
	}
	ctx := kong.Parse(config,
		kong.Description(description),
		kong.Vars{
			"version": fmt.Sprintf("version %s (build on %s, git revision %s)", Version, BuildTimestamp, Revision),
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
	defer func() {
		if !panicking {
			os.Exit(exitCode)
		}
	}()

	loggingConfig, err := extractLoggingConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot extract logging config: %s\n", err.Error())
		// Use the same exit code as Kong does.
		exitCode = 1
		panicking = false
		return
	}

	level := zerolog.Disabled
	writers := []io.Writer{}
	switch loggingConfig.Logging.Console.Type {
	case "color", "nocolor":
		w := newConsoleWriter(loggingConfig.Logging.Console.Type == "nocolor")
		writers = append(writers, &filteredWriter{
			Writer: levelWriterAdapter{w},
			Level:  loggingConfig.Logging.Console.Level,
		})
		if loggingConfig.Logging.Console.Level < level {
			level = loggingConfig.Logging.Console.Level
		}
	case "json":
		w := os.Stdout
		writers = append(writers, &filteredWriter{
			Writer: levelWriterAdapter{w},
			Level:  loggingConfig.Logging.Console.Level,
		})
		if loggingConfig.Logging.Console.Level < level {
			level = loggingConfig.Logging.Console.Level
		}
	}
	if loggingConfig.Logging.File.Path != "" {
		w, err := os.OpenFile(loggingConfig.Logging.File.Path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, fileMode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot open logging file: %s\n", err.Error())
			// Use the same exit code as Kong does.
			exitCode = 1
			panicking = false
			return
		}
		defer w.Close()
		writers = append(writers, &filteredWriter{
			Writer: levelWriterAdapter{w},
			Level:  loggingConfig.Logging.File.Level,
		})
		if loggingConfig.Logging.Console.Level < level {
			level = loggingConfig.Logging.File.Level
		}
	}

	writer := zerolog.MultiLevelWriter(writers...)
	logger := zerolog.New(writer).Level(level).With().Timestamp().Logger()

	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	zerolog.TimestampFunc = func() time.Time {
		return time.Now().UTC()
	}
	zerolog.TimeFieldFormat = "2006-01-02T15:04:05.000Z07:00"
	zerolog.ErrorMarshalFunc = func(ee error) interface{} {
		if ee == nil {
			return json.RawMessage("null")
		}

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
	zerolog.InterfaceMarshalFunc = func(v interface{}) ([]byte, error) {
		return x.MarshalWithoutEscapeHTML(v)
	}
	log.Logger = logger
	loggingConfig.Log = logger
	stdlog.SetFlags(0)
	stdlog.SetOutput(logger)

	err = run(ctx)
	if err != nil {
		log.Error().Err(err).Fields(errors.AllDetails(err)).Send()
		exitCode = errorExitCode
	}

	panicking = false
}
