// Package version хранит сведения о сборке бинарного файла.
package version

import "fmt"

// Сведения о сборке. Значения подменяются при сборке через -ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Info описывает сборку, из которой получен бинарный файл.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// New возвращает сведения о текущей сборке.
func New() Info {
	return Info{Version: Version, Commit: Commit, Date: Date}
}

// String выводит сведения о сборке для человека.
func (i Info) String() string {
	return fmt.Sprintf("%s (коммит %s, сборка %s)", i.Version, i.Commit, i.Date)
}
