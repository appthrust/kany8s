package eks

import (
	"testing"
	"time"
)

func TestEventStateCache_ShouldEmit_WithinTTLIsDeduped(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)
	c := &eventStateCache{
		last:       map[string]eventSignature{},
		ttl:        10 * time.Minute,
		maxEntries: 100,
		now:        func() time.Time { return now },
	}

	if !c.shouldEmit("controller", "default", "demo", "Normal", "Synced", "ok") {
		t.Fatalf("first emit = false, want true")
	}
	if c.shouldEmit("controller", "default", "demo", "Normal", "Synced", "ok") {
		t.Fatalf("second emit = true, want false")
	}
}

func TestEventStateCache_ShouldEmit_AfterTTLExpires(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)
	c := &eventStateCache{
		last:       map[string]eventSignature{},
		ttl:        1 * time.Minute,
		maxEntries: 100,
		now:        func() time.Time { return now },
	}

	if !c.shouldEmit("controller", "default", "demo", "Normal", "Synced", "ok") {
		t.Fatalf("first emit = false, want true")
	}
	now = now.Add(2 * time.Minute)
	if !c.shouldEmit("controller", "default", "demo", "Normal", "Synced", "ok") {
		t.Fatalf("emit after ttl = false, want true")
	}
}

func TestEventStateCache_ShouldEmit_EnforcesMaxEntries(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)
	c := &eventStateCache{
		last:       map[string]eventSignature{},
		ttl:        0,
		maxEntries: 2,
		now:        func() time.Time { return now },
	}

	if !c.shouldEmit("controller", "default", "a", "Normal", "R", "m1") {
		t.Fatalf("emit a = false, want true")
	}
	now = now.Add(time.Second)
	if !c.shouldEmit("controller", "default", "b", "Normal", "R", "m2") {
		t.Fatalf("emit b = false, want true")
	}
	now = now.Add(time.Second)
	if !c.shouldEmit("controller", "default", "c", "Normal", "R", "m3") {
		t.Fatalf("emit c = false, want true")
	}
	if got, want := len(c.last), 2; got != want {
		t.Fatalf("cache size = %d, want %d", got, want)
	}
}
