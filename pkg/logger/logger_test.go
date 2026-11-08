package logger

import (
	"testing"

	"github.com/Jidetireni/ara-cooperative/internal/config"
	"go.uber.org/zap/zapcore"
)

func TestLoggerLevels(t *testing.T) {
	for _, dev := range []bool{false, true} {
		name := "production"
		if dev {
			name = "development"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			log, err := New(config.Config{IsDev: dev})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := log.Sync(); err != nil {
					t.Error(err)
				}
			})
			if log.Sugar == nil || !log.Core().Enabled(zapcore.InfoLevel) || log.Core().Enabled(zapcore.DebugLevel) != dev {
				t.Fatal("logger level or sugar configuration is incorrect")
			}
		})
	}
}
