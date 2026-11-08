package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	loggerpkg "github.com/Jidetireni/ara-cooperative/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRequestLoggerPreservesFieldsWithoutCredentials(t *testing.T) {
	t.Parallel()
	core, observed := observer.New(zapcore.InfoLevel)
	base := zap.New(core)
	log := &loggerpkg.Logger{Logger: base, Sugar: base.Sugar()}
	m := &Middleware{Logger: log}
	handler := m.LoggerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/set-password?token=private-token",
		strings.NewReader(`{"password":"private-password"}`))
	request.Header.Set("Authorization", "Bearer private-bearer")
	handler.ServeHTTP(httptest.NewRecorder(), request)
	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("expected one request log, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["path"] != "/api/v1/set-password" || fields["method"] != "POST" || fields["status"] != int64(http.StatusCreated) {
		t.Fatalf("request log lost its structured fields: %v", fields)
	}
	for _, value := range fields {
		if text, ok := value.(string); ok && strings.Contains(text, "private-") {
			t.Fatal("request logger exposed credentials")
		}
	}
}
