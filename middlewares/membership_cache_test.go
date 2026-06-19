package middlewares

import (
	"testing"
	"time"
)

func TestTTLCache_SetGet(t *testing.T) {
	c := NewTTLCache(10, time.Minute)
	c.Set("u1", []string{"ws-a", "ws-b"})

	got, ok := c.Get("u1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(got) != 2 || got[0] != "ws-a" || got[1] != "ws-b" {
		t.Fatalf("unexpected value: %v", got)
	}
}

func TestTTLCache_Miss(t *testing.T) {
	c := NewTTLCache(10, time.Minute)
	if _, ok := c.Get("nonexistent"); ok {
		t.Fatal("expected miss for unknown key")
	}
}

func TestTTLCache_Expiry(t *testing.T) {
	c := NewTTLCache(10, 10*time.Millisecond)
	c.Set("u1", []string{"ws-a"})

	time.Sleep(20 * time.Millisecond)

	if _, ok := c.Get("u1"); ok {
		t.Fatal("expected expiry after TTL")
	}
}

func TestTTLCache_Overwrite(t *testing.T) {
	c := NewTTLCache(10, time.Minute)
	c.Set("u1", []string{"ws-old"})
	c.Set("u1", []string{"ws-new"})

	got, ok := c.Get("u1")
	if !ok {
		t.Fatal("expected hit")
	}
	if len(got) != 1 || got[0] != "ws-new" {
		t.Fatalf("expected ws-new, got %v", got)
	}
}

func TestTTLCache_LRUEviction(t *testing.T) {
	c := NewTTLCache(2, time.Minute)

	c.Set("u1", []string{"ws-a"})
	c.Set("u2", []string{"ws-b"})
	c.Set("u3", []string{"ws-c"}) // should evict u1 (least recently used)

	if _, ok := c.Get("u1"); ok {
		t.Fatal("expected u1 to be evicted")
	}
	if _, ok := c.Get("u2"); !ok {
		t.Fatal("expected u2 still present")
	}
	if _, ok := c.Get("u3"); !ok {
		t.Fatal("expected u3 still present")
	}
}

func TestTTLCache_LRUTouchOnGet(t *testing.T) {
	c := NewTTLCache(2, time.Minute)

	c.Set("u1", []string{"ws-a"})
	c.Set("u2", []string{"ws-b"})

	// Touch u1 to mark it as recently used.
	if _, ok := c.Get("u1"); !ok {
		t.Fatal("expected u1 hit")
	}

	// Now adding u3 should evict u2 (least recently used).
	c.Set("u3", []string{"ws-c"})

	if _, ok := c.Get("u1"); !ok {
		t.Fatal("expected u1 to remain (touched)")
	}
	if _, ok := c.Get("u2"); ok {
		t.Fatal("expected u2 to be evicted")
	}
}

func TestTTLCache_ZeroSize(t *testing.T) {
	c := NewTTLCache(0, time.Minute)
	c.Set("u1", []string{"ws-a"})
	// No eviction (maxSize=0 disables), entry should still be there.
	if _, ok := c.Get("u1"); !ok {
		t.Fatal("expected u1 to remain (maxSize=0 means no eviction)")
	}
}