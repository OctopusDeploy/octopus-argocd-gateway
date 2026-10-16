package testutilities

import (
	"io"
	"log/slog"
)

func CreateTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
