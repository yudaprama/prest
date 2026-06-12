package main

import (
	"fmt"
	"log/slog"

	"github.com/prest/prest/v2/adapters/postgres"
	"github.com/prest/prest/v2/cmd"
	"github.com/prest/prest/v2/config"
)

func main() {
	config.Load()
	registerExtraURLs()
	cmd.Execute()
}

func registerExtraURLs() {
	if config.PrestConf == nil {
		return
	}
	// Named URLs ([[pg.urls]] format with name+url fields)
	for _, nc := range config.PrestConf.PGNamedURLs {
		if nc.URL == "" {
			continue
		}
		if _, err := postgres.AddURI(nc.Name, nc.URL); err != nil {
			slog.Error("register named pg url", "name", nc.Name, "err", err)
		}
	}
	// Legacy string URLs (pg.urls = [...] format) — derive name from URL
	for i, u := range config.PrestConf.PGURLs {
		if u == "" {
			continue
		}
		name := config.DBNameFromURL(u)
		if name == "" {
			name = fmt.Sprintf("pg_urls_%d", i)
		}
		if _, err := postgres.AddURI(name, u); err != nil {
			slog.Error("register extra pg url", "idx", i, "err", err)
		}
	}
}
