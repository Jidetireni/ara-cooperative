package logger

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/Jidetireni/ara-cooperative/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	*zap.Logger
	Sugar *zap.SugaredLogger
}

func New(cfg config.Config) (*Logger, error) {
	options := zap.NewProductionConfig()
	if cfg.IsDev {
		options = zap.NewDevelopmentConfig()
	}
	options.OutputPaths = []string{"stdout"}
	options.ErrorOutputPaths = []string{"stderr"}
	options.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	base, err := options.Build()
	if err != nil {
		return nil, fmt.Errorf("initialize logger: %w", err)
	}
	return &Logger{Logger: base, Sugar: base.Sugar()}, nil
}

func (l *Logger) Sync() error {
	err := l.Logger.Sync()
	// Standard output can reject fsync even though log writes succeed.
	unsupportedSync := errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTTY)
	if unsupportedSync || errors.Is(err, syscall.EBADF) {
		return nil
	}
	return err
}
