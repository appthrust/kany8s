package eks

import (
	"testing"
	"time"
)

func TestComputeNextRequeue(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	policy := RequeuePolicy{
		RefreshBefore:    5 * time.Minute,
		MaxRefresh:       10 * time.Minute,
		FailureBackoff:   30 * time.Second,
		ImmediateRequeue: 15 * time.Second,
	}

	t.Run("caps at max refresh", func(t *testing.T) {
		t.Parallel()
		exp := now.Add(30 * time.Minute)
		if got, want := ComputeNextRequeue(now, exp, policy), 10*time.Minute; got != want {
			t.Fatalf("ComputeNextRequeue() = %s, want %s", got, want)
		}
	})

	t.Run("uses expiration minus refresh-before", func(t *testing.T) {
		t.Parallel()
		exp := now.Add(8 * time.Minute)
		if got, want := ComputeNextRequeue(now, exp, policy), 3*time.Minute; got != want {
			t.Fatalf("ComputeNextRequeue() = %s, want %s", got, want)
		}
	})

	t.Run("immediate when already past refresh threshold", func(t *testing.T) {
		t.Parallel()
		exp := now.Add(2 * time.Minute)
		if got, want := ComputeNextRequeue(now, exp, policy), 15*time.Second; got != want {
			t.Fatalf("ComputeNextRequeue() = %s, want %s", got, want)
		}
	})
}
