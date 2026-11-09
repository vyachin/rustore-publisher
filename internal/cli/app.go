// Package cli связывает команды, глобальные флаги и коды завершения процесса.
package cli

import (
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/vyachin/rustore-publisher/internal/version"
)

// App — зависимости команд, подменяемые в тестах.
type App struct {
	Stdout  io.Writer
	Stderr  io.Writer
	Logger  *slog.Logger
	Version version.Info

	// APIBaseURL и Now подменяются в тестах; пустые значения означают
	// адрес RuStore и системные часы.
	APIBaseURL string
	Now        func() time.Time
}

// DefaultApp возвращает приложение с потоками процесса.
func DefaultApp() *App {
	return &App{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Logger:  loggingDiscard(),
		Version: version.New(),
		Now:     time.Now,
	}
}

// loggingDiscard отключает журнал до разбора глобальных флагов.
func loggingDiscard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
