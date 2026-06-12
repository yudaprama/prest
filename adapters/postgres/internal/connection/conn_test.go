package connection

import (
	"strings"
	"testing"

	config "github.com/prest/prest/v2/config"
)

func init() {
	config.Load()
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
