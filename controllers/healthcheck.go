package controllers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prest/prest/v2/adapters/postgres"
	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"

	"log/slog"
)

type CheckList []func(context.Context) error

var DefaultCheckList = CheckList{
	CheckDBHealth,
}

func CheckDBHealth(ctx context.Context) error {
	// Multi-database mode: the default pg.url is intentionally empty (the
	// startup path skips it to avoid dialling a non-existent database), so
	// ping every registered named DB. Any unreachable DB = unhealthy.
	// (config.PrestConf is nil until config.Load(); guard so this is safe in
	// tests and any caller that runs before configuration is parsed.)
	if config.PrestConf != nil && !config.PrestConf.SingleDB && len(config.PrestConf.PGNamedURLs) > 0 {
		for _, u := range config.PrestConf.PGNamedURLs {
			db, err := postgres.GetByName(u.Name)
			if err != nil {
				return fmt.Errorf("health: db %q not in pool: %w", u.Name, err)
			}
			if err := db.PingContext(ctx); err != nil {
				return fmt.Errorf("health: ping %q failed: %w", u.Name, err)
			}
		}
		return nil
	}
	// Single-database mode (or no named DBs registered): probe the default
	// connection. In multi-database deployments the named-URL branch above is
	// taken instead, so this default probe is never reached against an empty
	// pg.url there.
	conn, err := postgres.Get()
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, ";")
	return err
}

func WrappedHealthCheck(checks CheckList) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		timeout, _ := r.Context().Value(pctx.HTTPTimeoutKey).(int)
		ctx, cancel := context.WithTimeout(
			r.Context(), time.Second*time.Duration(timeout))
		defer cancel()
		for _, check := range checks {
			if err := check(ctx); err != nil {
				slog.Error("could not check DB connection", "err", err)
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}
}
