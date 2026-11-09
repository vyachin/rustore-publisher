package cli

import (
	"github.com/vyachin/rustore-publisher/internal/app"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

// newRustoreClient загружает учётные данные и собирает клиент API.
func newRustoreClient(a *App, g *globalFlags) (*rustore.Client, error) {
	path := app.ResolveCredentialsPath(g.Credentials)
	credentials, err := app.LoadCredentials(path)
	if err != nil {
		return nil, err
	}

	return rustore.NewClient(rustore.Config{
		BaseURL: a.APIBaseURL,
		Credentials: rustore.Credentials{
			KeyID: credentials.KeyID,
			Key:   credentials.Key,
		},
		RequestTimeout: g.RequestTimeout,
		Now:            a.Now,
		Logger:         a.Logger,
	})
}
