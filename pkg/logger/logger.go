package logger

import (
	"os"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/rs/zerolog"
)

type Logger struct {
	*zerolog.Logger
}

func New(cfg config.Config) *Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	level := zerolog.InfoLevel
	if cfg.IsDev {
		level = zerolog.DebugLevel
	}
	zerolog.SetGlobalLevel(level)

	z := zerolog.New(os.Stdout).With().Timestamp().Logger()

	if cfg.IsDev {
		z = z.Output(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		})
	}

	return &Logger{Logger: &z}
}
