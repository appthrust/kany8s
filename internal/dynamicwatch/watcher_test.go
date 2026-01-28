package dynamicwatch

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const dynamicwatchChannelFullMetricName = "kany8s_dynamicwatch_channel_full_total"

func TestWatcher_EnqueueWhenChannelFull_EventEventuallyDelivered(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan event.GenericEvent, 1)
	w := New(fake.NewSimpleDynamicClient(runtime.NewScheme()), events)

	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()
	waitForWatcherStart(t, w)

	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("watcher did not stop")
		}
	})

	a := &unstructured.Unstructured{}
	a.SetAPIVersion("kro.run/v1alpha1")
	a.SetKind("Example")
	a.SetNamespace("default")
	a.SetName("a")

	b := &unstructured.Unstructured{}
	b.SetAPIVersion("kro.run/v1alpha1")
	b.SetKind("Example")
	b.SetNamespace("default")
	b.SetName("b")

	w.enqueue(a)
	w.enqueue(b)

	got1 := <-events
	if got1.Object.GetName() != "a" {
		t.Fatalf("expected first event name %q, got %q", "a", got1.Object.GetName())
	}

	select {
	case got2 := <-events:
		if got2.Object.GetName() != "b" {
			t.Fatalf("expected second event name %q, got %q", "b", got2.Object.GetName())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for coalesced event to be delivered")
	}
}

func TestWatcher_EnqueueWhenChannelFull_IncrementsMetric(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan event.GenericEvent, 1)
	w := New(fake.NewSimpleDynamicClient(runtime.NewScheme()), events)

	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()
	waitForWatcherStart(t, w)

	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("watcher did not stop")
		}
	})

	before := getCounterValue(t, dynamicwatchChannelFullMetricName)

	a := &unstructured.Unstructured{}
	a.SetAPIVersion("kro.run/v1alpha1")
	a.SetKind("Example")
	a.SetNamespace("default")
	a.SetName("a")

	b := &unstructured.Unstructured{}
	b.SetAPIVersion("kro.run/v1alpha1")
	b.SetKind("Example")
	b.SetNamespace("default")
	b.SetName("b")

	w.enqueue(a)
	w.enqueue(b)

	after := getCounterValue(t, dynamicwatchChannelFullMetricName)
	if after <= before {
		t.Fatalf("expected %s to increase (before=%v after=%v)", dynamicwatchChannelFullMetricName, before, after)
	}
}

func waitForWatcherStart(t *testing.T, w *Watcher) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w.mu.Lock()
		started := w.stopCh != nil
		w.mu.Unlock()
		if started {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("watcher did not start")
}

func getCounterValue(t *testing.T, name string) float64 {
	t.Helper()

	mfs, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		ms := mf.GetMetric()
		if len(ms) == 0 {
			t.Fatalf("metric %q has no samples", name)
		}
		if ms[0].GetCounter() == nil {
			t.Fatalf("metric %q is not a counter", name)
		}
		return ms[0].GetCounter().GetValue()
	}

	t.Fatalf("metric %q not found", name)
	return 0
}
