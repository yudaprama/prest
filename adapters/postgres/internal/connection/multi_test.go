package connection

import (
	"os"
	"strings"
	"testing"
)

// TestMultipleDSNs exercises AddURI so a single prest process can hold
// connections to several independent databases (different hosts, credentials,
// etc.) supplied as raw DSNs. DSNs are passed via the PREST_TEST_DSNS env
// var (comma-separated) so credentials never end up in the repo.
func TestMultipleDSNs(t *testing.T) {
	raw := os.Getenv("PREST_TEST_DSNS")
	if raw == "" {
		t.Skip("PREST_TEST_DSNS not set; skipping multi-DSN smoke test")
	}
	parts := strings.Split(raw, ",")
	if len(parts) < 2 {
		t.Skipf("need at least 2 DSNs to validate multi-connection support, got %d", len(parts))
	}

	// Defaults from viper; the test only relies on MaxIdleConn/MaxOpenConn
	// not being negative, so prime the pool if it wasn't created yet.
	_ = GetPool()

	for i, dsn := range parts {
		dsn = strings.TrimSpace(dsn)
		if dsn == "" {
			continue
		}
		name := "dsn_" + itoa(i)
		db, err := AddURI(name, dsn)
		if err != nil {
			t.Fatalf("[%s] AddURI failed: %v", name, err)
		}
		if err := db.Ping(); err != nil {
			t.Fatalf("[%s] ping failed: %v", name, err)
		}
		var v string
		if err := db.Get(&v, "select current_database()"); err != nil {
			t.Fatalf("[%s] query failed: %v", name, err)
		}
		t.Logf("[%s] OK db=%s", name, v)
	}

	// Both DSNs should now be retrievable from the pool by DSN key.
	for i, dsn := range parts {
		dsn = strings.TrimSpace(dsn)
		if dsn == "" {
			continue
		}
		if _, err := getFromPoolByDSN(dsn); err != nil {
			t.Errorf("[%d] expected DSN in pool, got: %v", i, err)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := ""
	for i > 0 {
		digits = string(rune('0'+i%10)) + digits
		i /= 10
	}
	return digits
}
