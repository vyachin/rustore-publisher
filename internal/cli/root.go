package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
	"github.com/vyachin/rustore-publisher/internal/logging"
)

// globalFlags — флаги, общие для всех команд.
type globalFlags struct {
	Debug          bool
	JSON           bool
	Timeout        time.Duration
	RequestTimeout time.Duration
	Credentials    string
	cancel         context.CancelFunc
}

// Execute запускает дерево команд и возвращает код завершения процесса.
func Execute(ctx context.Context, app *App, args []string) int {
	root, g := newRootCmd(app)
	defer func() {
		if g.cancel != nil {
			g.cancel()
		}
	}()

	root.SetArgs(args)
	executed, err := root.ExecuteContextC(ctx)
	if err == nil {
		return int(exitcode.OK)
	}

	var rendered *renderedError
	alreadyRendered := errors.As(err, &rendered)

	code := exitcode.From(err)
	if ctx.Err() != nil {
		code = exitcode.Interrupted
		err = exitcode.Errorf(exitcode.KindInterrupted, "операция прервана")
	}

	if g.JSON && !alreadyRendered {
		result := NewResult(commandName(executed))
		result.Error = NewErrorDetail(err)
		if writeErr := EncodeJSON(app.Stdout, result); writeErr != nil {
			printError(app.Stderr, writeErr, exitcode.Internal)
			return int(exitcode.Internal)
		}
	}

	printError(app.Stderr, err, code)
	return int(code)
}

// NewRootCmd собирает корневую команду со всеми подкомандами.
func NewRootCmd(app *App) *cobra.Command {
	root, _ := newRootCmd(app)
	return root
}

func newRootCmd(app *App) (*cobra.Command, *globalFlags) {
	g := &globalFlags{}

	root := &cobra.Command{
		Use:           "rustore-publisher",
		Short:         "Публикация Android App Bundle в RuStore",
		Long:          "rustore-publisher публикует подписанные Android App Bundle (AAB) в RuStore.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return exitcode.Errorf(exitcode.KindInvalidArguments,
				"неизвестная команда %q для %q", args[0], cmd.CommandPath())
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("timeout") {
				g.Timeout = 30 * time.Minute
			}
			if !cmd.Flags().Changed("request-timeout") {
				g.RequestTimeout = 2 * time.Minute
			}
			if g.Timeout < 0 {
				return exitcode.Errorf(exitcode.KindInvalidArguments,
					"--timeout не может быть отрицательным: %s", g.Timeout)
			}
			if g.RequestTimeout <= 0 {
				return exitcode.Errorf(exitcode.KindInvalidArguments,
					"--request-timeout должен быть положительным: %s", g.RequestTimeout)
			}

			app.Logger = logging.New(logging.Options{Debug: g.Debug, Out: app.Stderr})

			if g.Timeout > 0 {
				ctx, cancel := context.WithTimeout(cmd.Context(), g.Timeout)
				g.cancel = cancel
				cmd.SetContext(ctx)
			}
			return nil
		},
	}

	root.SetOut(app.Stdout)
	root.SetErr(app.Stderr)
	root.SetHelpTemplate(helpTemplate)
	root.SetUsageTemplate(usageTemplate)
	root.SetHelpCommand(newHelpCmd())
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidArguments, err),
			"проверьте имя и значение флага",
		)
	})
	root.CompletionOptions.DisableDefaultCmd = true

	flags := root.PersistentFlags()
	flags.BoolVar(&g.Debug, "debug", false, "подробный журнал в stderr")
	flags.BoolVar(&g.JSON, "json", false, "вывести один объект JSON в stdout")
	// Значения по умолчанию подставляются в PersistentPreRunE, чтобы
	// справка не показывала английское "(default ...)".
	flags.DurationVar(&g.Timeout, "timeout", 0,
		"общий бюджет команды, по умолчанию 30m; 0 отключает ограничение")
	flags.DurationVar(&g.RequestTimeout, "request-timeout", 0,
		"бюджет одного запроса к API, по умолчанию 2m")
	flags.StringVar(&g.Credentials, "credentials", "",
		"путь к файлу учётных данных (или RUSTORE_CREDENTIALS_FILE)")

	root.AddCommand(
		newVersionCmd(app, g),
		newCompletionCmd(app, g),
	)

	root.InitDefaultHelpCmd()
	setHelpFlags(root)

	return root, g
}

// setHelpFlags задаёт русское описание флага -h для всех команд.
func setHelpFlags(cmd *cobra.Command) {
	if cmd.Flags().Lookup("help") == nil {
		cmd.Flags().BoolP("help", "h", false, "показать справку")
	}
	for _, child := range cmd.Commands() {
		setHelpFlags(child)
	}
}

// newHelpCmd заменяет стандартную англоязычную команду help.
func newHelpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "help [команда]",
		Short: "Справка по команде",
		Long:  "Показать справку по указанной команде.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, _, err := cmd.Root().Find(args)
			if err != nil || target == nil || (len(args) > 0 && target == cmd.Root()) {
				return exitcode.Errorf(exitcode.KindInvalidArguments,
					"неизвестная команда %q", strings.Join(args, " "))
			}
			return target.Help()
		},
	}
}

// commandName возвращает имя выполненной команды для JSON-результата.
func commandName(cmd *cobra.Command) string {
	if cmd == nil {
		return "rustore-publisher"
	}
	return cmd.Name()
}

// printError пишет сообщение об ошибке и подсказку один раз.
func printError(w io.Writer, err error, code exitcode.Code) {
	_, _ = fmt.Fprintf(w, "rustore-publisher: %v\n", err)

	if hint := exitcode.HintOf(err); hint != "" {
		_, _ = fmt.Fprintf(w, "  подсказка: %s\n", hint)
	}
	if code == exitcode.Usage {
		_, _ = fmt.Fprintln(w, "Запустите 'rustore-publisher --help', чтобы увидеть справку.")
	}
}
