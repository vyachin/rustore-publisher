package cli

import (
	"io"
	"log/slog"
	"os"

	"github.com/vyachin/rustore-publisher/internal/version"
)

// App — зависимости команд, подменяемые в тестах.
type App struct {
	Stdout  io.Writer
	Stderr  io.Writer
	Logger  *slog.Logger
	Version version.Info
}

// DefaultApp возвращает приложение с потоками процесса.
func DefaultApp() *App {
	return &App{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Logger:  loggingDiscard(),
		Version: version.New(),
	}
}

// loggingDiscard отключает журнал до разбора глобальных флагов.
func loggingDiscard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
