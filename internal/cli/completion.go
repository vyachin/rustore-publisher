package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

// shells — поддерживаемые оболочки для автодополнения.
var shells = []string{"bash", "zsh", "fish", "powershell"}

func newCompletionCmd(app *App, g *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Вывести скрипт автодополнения оболочки",
		Long:      "Вывести скрипт автодополнения для bash, zsh, fish или powershell.",
		Example:   "  rustore-publisher completion bash",
		ValidArgs: shells,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return exitcode.Wrap(exitcode.KindInvalidArguments, err)
			}
			for _, shell := range shells {
				if args[0] == shell {
					return nil
				}
			}
			return exitcode.Errorf(exitcode.KindInvalidArguments,
				"неизвестная оболочка %q: допустимы %v", args[0], shells)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if g.JSON {
				return exitcode.Errorf(exitcode.KindInvalidArguments,
					"completion не поддерживает --json")
			}

			var err error
			switch args[0] {
			case "bash":
				err = cmd.Root().GenBashCompletionV2(app.Stdout, true)
			case "zsh":
				err = cmd.Root().GenZshCompletion(app.Stdout)
			case "fish":
				err = cmd.Root().GenFishCompletion(app.Stdout, true)
			case "powershell":
				err = cmd.Root().GenPowerShellCompletionWithDesc(app.Stdout)
			default:
				err = fmt.Errorf("неизвестная оболочка %q", args[0])
			}
			if err != nil {
				return exitcode.Wrap(exitcode.KindInternal, err)
			}
			return nil
		},
	}
	return cmd
}
