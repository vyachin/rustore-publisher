// Package logging настраивает журнал процесса.
package logging

import (
	"io"
	"log/slog"
)

// Options управляет созданием журнала.
type Options struct {
	Debug bool
	Out   io.Writer
}

// New возвращает текстовый журнал в opts.Out. Debug включает уровень DEBUG
// и указание источника записи.
func New(opts Options) *slog.Logger {
	level := slog.LevelInfo
	if opts.Debug {
		level = slog.LevelDebug
	}

	return slog.New(slog.NewTextHandler(opts.Out, &slog.HandlerOptions{
		Level:     level,
		AddSource: opts.Debug,
	}))
}
