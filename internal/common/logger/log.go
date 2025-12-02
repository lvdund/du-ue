package logger

import (
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var DefaultLogLevel = zerolog.InfoLevel

type Logger struct {
	logger *zerolog.Logger
}

func InitLogger(level string, fields map[string]string) *Logger {
	consoleWriter := zerolog.ConsoleWriter{
		Out:     os.Stderr,
		NoColor: true,
		FormatLevel: func(i any) string {
			// Custom level formatting without bold
			level := strings.ToUpper(fmt.Sprintf("%s", i))
			switch level {
			case "DEBUG":
				return "\033[36m" + level + "\033[0m" // Cyan
			case "INFO":
				return "\033[32m" + level + "\033[0m" // Green
			case "WARN":
				return "\033[33m" + level + "\033[0m" // Yellow
			case "ERROR":
				return "\033[31m" + level + "\033[0m" // Red
			case "FATAL":
				return "\033[35m" + level + "\033[0m" // Magenta
			default:
				return level
			}
		},
	}
	entry := zerolog.New(consoleWriter).With().Timestamp().Logger()
	logger := &Logger{logger: &entry}

	if fields == nil {
		logger.logger = &entry
		return logger
	}

	loggerWithFields := logger.logger.With().Fields(fields)
	for k, v := range fields {
		loggerWithFields = loggerWithFields.Interface(k, v)
	}
	outlog := loggerWithFields.Logger()
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
	l.logger.Info().Msgf(fmt.Sprintf("%-50s\t", format), args...)
}

func (l *Logger) Warn(format string, args ...any) {
	l.logger.Warn().Msgf(fmt.Sprintf("%-50s\t", format), args...)
}

func (l *Logger) Error(format string, args ...any) {
	l.logger.Error().Msgf(fmt.Sprintf("%-50s\t", format), args...)
}

func (l *Logger) Fatal(format string, args ...any) {
	l.logger.Fatal().Msgf(fmt.Sprintf("%-50s\t", format), args...)
}

func (l *Logger) Panic(format string, args ...any) {
	l.logger.Panic().Msgf(fmt.Sprintf("%-50s\t", format), args...)
}

func (l *Logger) Trace(format string, args ...any) {
	l.logger.Trace().Msgf(fmt.Sprintf("%-50s\t", format), args...)
}

func (l *Logger) Debug(format string, args ...any) {
	l.logger.Debug().Msgf(fmt.Sprintf("%-50s\t", format), args...)
}

func (l *Logger) Printf(format string, args ...any) {
	l.logger.Printf(fmt.Sprintf("%-50s\t", format), args...)
}
func (l *Logger) Print(args ...any) {
	l.logger.Print(args...)
}
