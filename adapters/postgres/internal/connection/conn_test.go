package connection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	config "github.com/prest/prest/v2/config"
)

func init() {
	// Locate the project root prest.toml when running tests from this
	// package directory. The default config lookup uses ./prest.toml which
	// does not exist in adapters/postgres/internal/connection/.
	if os.Getenv("PREST_CONF") == "" {
		if p, ok := findProjectConfig(); ok {
			os.Setenv("PREST_CONF", p)
		}
	}
	config.Load()

	// Populate PREST_TEST_DSNS from the loaded config so TestMultipleDSNs
	// exercises the named DSNs declared in prest.toml. If a caller has
	// already set PREST_TEST_DSNS, that value wins (credentials are
	// already in the environment, no need to override).
	if os.Getenv("PREST_TEST_DSNS") == "" && config.PrestConf != nil {
		var dsns []string
		for _, nc := range config.PrestConf.PGNamedURLs {
			if nc.URL != "" {
				dsns = append(dsns, nc.URL)
			}
		}
		for _, u := range config.PrestConf.PGURLs {
			if u != "" {
				dsns = append(dsns, u)
			}
		}
		if len(dsns) > 0 {
			os.Setenv("PREST_TEST_DSNS", strings.Join(dsns, ","))
		}
	}
}

// findProjectConfig walks up from the current working directory looking for
// prest.toml. Returns the absolute path when found.
func findProjectConfig() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	dir := cwd
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "prest.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

func TestGet(t *testing.T) {
	t.Log("Open connection")
	db, err := Get()
	if err != nil {
		if strings.Contains(err.Error(), "connect: connection refused") {
			 t.Skip("Postgres not available, skipping integration test")
			 return
		}
		 t.Fatalf("Expected err equal to nil but got %q", err.Error())
	}

	t.Log("Ping Pong")
	err = db.Ping()
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}
}

func TestMustGet(t *testing.T) {
	t.Log("Open connection")
	db := MustGet()
	if db == nil {
		 t.Skip("Postgres not available, skipping integration test")
		 return
	}

	t.Log("Ping Pong")
	err := db.Ping()
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}
}
