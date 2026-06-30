package connection

import (
	"testing"
)

func TestSetRealName(t *testing.T) {
	// Reset pool state for isolated test
	pool = nil

	// Register mappings for the multi-connection use case
	SetRealName("myalias", "postgres")
	SetRealName("yarsew", "postgres")
	SetRealName("kratos", "postgres")

	tests := []struct {
		name     string
		logical  string
		expected string
	}{
		{"myalias maps to postgres", "myalias", "postgres"},
		{"yarsew maps to postgres", "yarsew", "postgres"},
		{"kratos maps to postgres", "kratos", "postgres"},
		{"unmapped returns PGDatabase fallback", "unknown", "postgres"},
		{"empty returns empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved := ResolveDBName(tt.logical)
			if resolved != tt.expected {
				t.Errorf("ResolveDBName(%q) = %q, want %q", tt.logical, resolved, tt.expected)
			}
		})
	}

	// Verify the pool state directly
	p := GetPool()
	p.Mtx.Lock()
	defer p.Mtx.Unlock()

	if real, ok := p.RealName["myalias"]; !ok || real != "postgres" {
		t.Errorf("RealName['myalias'] = %q, want 'postgres'", real)
	}
	if real, ok := p.RealName["yarsew"]; !ok || real != "postgres" {
		t.Errorf("RealName['yarsew'] = %q, want 'postgres'", real)
	}

	// Verify that SetRealName with empty values is a no-op
	SetRealName("", "actual")
	SetRealName("logical", "")
	if _, ok := p.RealName[""]; ok {
		t.Error("empty logical name should not be stored")
	}
	if _, ok := p.RealName["logical"]; ok {
		t.Error("empty actual name should not be stored")
	}
}
