package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

func newVersionCmd(app *App, g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Показать сведения о сборке",
		Long:    "Показать версию, коммит и дату сборки rustore-publisher.",
		Example: "  rustore-publisher version\n  rustore-publisher --json version",
		Args:    noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if g.JSON {
				return EncodeJSON(app.Stdout, VersionResult{
					SchemaVersion: SchemaVersion,
					Command:       "version",
					Version:       app.Version.Version,
					Commit:        app.Version.Commit,
					BuildDate:     app.Version.Date,
				})
			}

			_, _ = fmt.Fprintf(app.Stdout, "rustore-publisher %s\n", app.Version)
			return nil
		},
	}
}

// noArgs запрещает позиционные аргументы с кодом ошибки ввода.
func noArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return exitcode.Wrap(exitcode.KindInvalidArguments, err)
	}
	return nil
}
