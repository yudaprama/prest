package config

import (
	"fmt"
	"net/url"
	"os"
	"testing"
)

// redactURL hides the password in a Postgres URL for safe logging.
func redactURL(raw string) string {
	if raw == "" {
		return "(empty)"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "(unparseable)"
	}
	if u.User != nil {
		if _, hasPass := u.User.Password(); hasPass {
			u.User = url.UserPassword(u.User.Username(), "REDACTED")
		}
	}
	return u.String()
}

// TestSmokeDotEnvLoad verifies that .env values flow through to
// PrestConf.PGNamedURLs at runtime. Run with:
//
//	PREST_CONF=testdata/pg_urls.toml go test -run TestSmokeDotEnvLoad -v ./config
func TestSmokeDotEnvLoad(t *testing.T) {
	t.Setenv("PREST_CONF", "testdata/pg_urls.toml")
	Load()

	if PrestConf == nil {
		t.Fatal("PrestConf is nil after Load()")
	}
	fmt.Printf("Loaded %d named URL(s) after Load()\n", len(PrestConf.PGNamedURLs))
	for _, u := range PrestConf.PGNamedURLs {
		user := ""
		if u.URL != "" {
			if purl, perr := url.Parse(u.URL); perr == nil {
				user = purl.User.Username()
			}
		}
		fmt.Printf("  - name=%-12s user=%q  url=%s\n", u.Name, user, redactURL(u.URL))
	}
}

// TestSmokeDotEnvLoad_ActualConfig loads the real prest.toml and .env
// from the working directory — the same path `prestd` at runtime uses.
//
//	go test -run TestSmokeDotEnvLoad_ActualConfig -v ./config
func TestSmokeDotEnvLoad_ActualConfig(t *testing.T) {
	// Run from the repo root so ./prest.toml + .env resolve. Tests are
	// normally invoked from inside the config/ package, where those files
	// don't exist; this is a manual smoke check, not a CI test.
	if _, err := os.Stat("../prest.toml"); err != nil {
		t.Skip("run this from `go test ./config/...` at the repo root: ../prest.toml not found")
	}
	if _, err := os.Stat("../.env"); err != nil {
		t.Skip("../.env not present; create one from .env.example before running this test")
	}

	// loadDotEnv reads from the *current* working directory, so chdir
	// to the repo root first.
	if err := os.Chdir(".."); err != nil {
		t.Fatalf("chdir to repo root: %v", err)
	}
	t.Setenv("PREST_CONF", "./prest.toml")

	Load()

	if PrestConf == nil {
		t.Fatal("PrestConf is nil after Load()")
	}
	fmt.Printf("Loaded %d named URL(s) from ./prest.toml + ./.env\n", len(PrestConf.PGNamedURLs))
	for _, u := range PrestConf.PGNamedURLs {
		user := ""
		if u.URL != "" && u.URL != "<nil>" {
			if purl, perr := url.Parse(u.URL); perr == nil {
				user = purl.User.Username()
			}
		}
		fmt.Printf("  - name=%-12s user=%q  url=%s\n", u.Name, user, redactURL(u.URL))
	}

	if len(PrestConf.PGNamedURLs) == 0 {
		t.Log("WARNING: prest.toml has no [[pg.urls]] entries or .env is missing the vars")
	}
}
