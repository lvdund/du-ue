package logger

import (
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var DefaultLogLevel = zerolog.InfoLevel
var GlobalTimeFormat = "15:04:05" // Standard format for precision (HH:MM:SS)

func SetTimeFormat(format string) {
	if format != "" {
		GlobalTimeFormat = format
	}
}

type Logger struct {
	logger *zerolog.Logger
}

func NewConsoleWriter() zerolog.ConsoleWriter {
	return zerolog.ConsoleWriter{
		Out:        os.Stderr,
		NoColor:    true,
		TimeFormat: GlobalTimeFormat,
		FormatMessage: func(i interface{}) string {
			if i == nil {
				return fmt.Sprintf("%-100s", "")
			}
			msg := fmt.Sprintf("%s", i)
			// Pad to 100 characters for perfect alignment of key-value fields.
			if len(msg) < 100 {
				return msg + strings.Repeat(" ", 100-len(msg))
			}
			return msg + " "
		},
		FormatLevel: func(i any) string {
			level := strings.ToUpper(fmt.Sprintf("%s", i))
			var color string
			switch level {
			case "DEBUG":
				color = "\033[36m" // Cyan
			case "INFO":
				color = "\033[32m" // Green
			case "WARN":
				color = "\033[33m" // Yellow
			case "ERROR":
				color = "\033[31m" // Red
			case "FATAL":
				color = "\033[35m" // Magenta
			default:
				color = "\033[37m" // White
			}
			return fmt.Sprintf("%-5s", color+level+"\033[0m")
		},
		FormatTimestamp: func(i interface{}) string {
			return "\033[90m" + fmt.Sprintf("%s", i) + "\033[0m" // Grey timestamp
		},
		FormatFieldName: func(i interface{}) string {
			return fmt.Sprintf("\033[36m%s\033[0m\033[90m=\033[0m", i) // Cyan key with grey =
		},
		FormatFieldValue: func(i interface{}) string {
			val := fmt.Sprintf("%v", i)
			// Optional: custom colors for specific values
			if val == "du" || val == "f1ap_client" {
				return "\033[33m" + val + "\033[0m" // Yellow for DU/F1
			}
			if val == "ue" || strings.HasPrefix(val, "0000") {
				return "\033[32m" + val + "\033[0m" // Green for UE
			}
			return "\033[37m" + val + "\033[0m" // White for others
		},
	}
}

func InitLogger(level string, fields map[string]string) *Logger {
	consoleWriter := NewConsoleWriter()
	entry := zerolog.New(consoleWriter).With().Timestamp().Logger()
	logger := &Logger{logger: &entry}

	if fields == nil {
		return logger
	}

	// Important fields to prioritize and keep in order for visual alignment
	order := []string{"du_id", "mod", "msin", "pduId"}
	
	ctx := logger.logger.With()
	
	// First add ordered fields
	added := make(map[string]bool)
	for _, key := range order {
		if val, ok := fields[key]; ok {
			ctx = ctx.Str(key, val)
			added[key] = true
		}
	}
	
	// Then add remaining fields (if any)
	for k, v := range fields {
		if !added[k] {
			ctx = ctx.Str(k, v)
		}
	}
	
	outlog := ctx.Logger()
	logger.logger = &outlog
	return logger
}

func ParseLogLevel(level string) {
	var logLevel zerolog.Level
	var err error

	if len(level) > 0 {
		if logLevel, err = zerolog.ParseLevel(strings.ToLower(level)); err != nil {
			log.Error().Err(err).Msg("Failed to parse log level -> set InfoLevel")
			zerolog.SetGlobalLevel(DefaultLogLevel)
		} else {
			zerolog.SetGlobalLevel(logLevel)
		}
	}
}

func (l *Logger) Info(format string, args ...any) {
	if len(args) > 0 {
		l.logger.Info().Msgf(format, args...)
	} else {
		l.logger.Info().Msg(format)
	}
}

func (l *Logger) Warn(format string, args ...any) {
	if len(args) > 0 {
		l.logger.Warn().Msgf(format, args...)
	} else {
		l.logger.Warn().Msg(format)
	}
}

func (l *Logger) Error(format string, args ...any) {
	if len(args) > 0 {
		l.logger.Error().Msgf(format, args...)
	} else {
		l.logger.Error().Msg(format)
	}
}

func (l *Logger) Fatal(format string, args ...any) {
	if len(args) > 0 {
		l.logger.Fatal().Msgf(format, args...)
	} else {
		l.logger.Fatal().Msg(format)
	}
}

func (l *Logger) Panic(format string, args ...any) {
	if len(args) > 0 {
		l.logger.Panic().Msgf(format, args...)
	} else {
		l.logger.Panic().Msg(format)
	}
}

func (l *Logger) Trace(format string, args ...any) {
	if len(args) > 0 {
		l.logger.Trace().Msgf(format, args...)
	} else {
		l.logger.Trace().Msg(format)
	}
}

func (l *Logger) Debug(format string, args ...any) {
	if len(args) > 0 {
		l.logger.Debug().Msgf(format, args...)
	} else {
		l.logger.Debug().Msg(format)
	}
}

func (l *Logger) Printf(format string, args ...any) {
	if len(args) > 0 {
		l.logger.Printf(format, args...)
	} else {
		l.logger.Print(format)
	}
}
func (l *Logger) Print(args ...any) {
	l.logger.Print(args...)
}
