package dynamicwatch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var dynamicwatchChannelFullTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "kany8s_dynamicwatch_channel_full_total",
	Help: "Number of dynamicwatch events that could not be enqueued because the channel was full.",
})

func init() {
	metrics.Registry.MustRegister(dynamicwatchChannelFullTotal)
}

type Watcher struct {
	factory dynamicinformer.DynamicSharedInformerFactory
	events  chan<- event.GenericEvent
	logger  logrLogger

	mu        sync.Mutex
	informers map[schema.GroupVersionResource]cache.SharedIndexInformer
	stopCh    <-chan struct{}

	flusherStarted bool
	flushCh        chan struct{}
	pending        map[string]*unstructured.Unstructured

	nextChannelFullLog time.Time
}

func New(dynClient dynamic.Interface, events chan<- event.GenericEvent) *Watcher {
	return &Watcher{
		factory:   dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynClient, 0*time.Second, "", nil),
		events:    events,
		logger:    log.Log.WithName("dynamicwatch"),
		informers: map[schema.GroupVersionResource]cache.SharedIndexInformer{},
		flushCh:   make(chan struct{}, 1),
		pending:   map[string]*unstructured.Unstructured{},
	}
}

func (w *Watcher) NeedLeaderElection() bool {
	return true
}

func (w *Watcher) Start(ctx context.Context) error {
	w.mu.Lock()
	stopCh := w.stopCh
	if stopCh == nil {
		w.stopCh = ctx.Done()
		stopCh = w.stopCh
	}
	startFlusher := !w.flusherStarted
	if startFlusher {
		w.flusherStarted = true
	}
	w.mu.Unlock()

	if startFlusher {
		go w.flushLoop(stopCh)
	}

	w.factory.Start(stopCh)
	<-stopCh
	return nil
}

func (w *Watcher) EnsureWatch(ctx context.Context, gvk schema.GroupVersionKind) error {
	if gvk.Kind == "" {
		return fmt.Errorf("gvk.kind is required")
	}

	pluralGVR, _ := meta.UnsafeGuessKindToResource(gvk)
	gvr := pluralGVR
	if gvr.Resource == "" {
		return fmt.Errorf("resolved gvr.resource is empty for gvk %s", gvk.String())
	}

	var informer cache.SharedIndexInformer
	var stopCh <-chan struct{}

	w.mu.Lock()
	stopCh = w.stopCh
	if existing, ok := w.informers[gvr]; ok {
		informer = existing
		w.mu.Unlock()
		if stopCh != nil {
			cache.WaitForCacheSync(ctx.Done(), informer.HasSynced)
		}
		return nil
	}

	gi := w.factory.ForResource(gvr)
	informer = gi.Informer()
	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.enqueue,
		UpdateFunc: func(_, newObj any) { w.enqueue(newObj) },
		DeleteFunc: w.enqueue,
	}); err != nil {
		w.mu.Unlock()
		return err
	}
	w.informers[gvr] = informer
	w.mu.Unlock()

	if stopCh != nil {
		w.factory.Start(stopCh)
		cache.WaitForCacheSync(ctx.Done(), informer.HasSynced)
	}

	return nil
}

func (w *Watcher) enqueue(obj any) {
	u := extractUnstructured(obj)
	if u == nil {
		return
	}
	if u.GetNamespace() == "" {
		return
	}

	e := &unstructured.Unstructured{}
	e.SetAPIVersion(u.GetAPIVersion())
	e.SetKind(u.GetKind())
	e.SetNamespace(u.GetNamespace())
	e.SetName(u.GetName())

	select {
	case w.events <- event.GenericEvent{Object: e}:
	default:
		dynamicwatchChannelFullTotal.Inc()
		w.enqueuePending(e)
		w.maybeLogChannelFull(e)
		w.signalFlush()
	}
}

const flushInterval = 50 * time.Millisecond

func (w *Watcher) signalFlush() {
	select {
	case w.flushCh <- struct{}{}:
	default:
	}
}

func (w *Watcher) enqueuePending(u *unstructured.Unstructured) {
	key := pendingKey(u)

	w.mu.Lock()
	w.pending[key] = u
	w.mu.Unlock()
}

func (w *Watcher) flushLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			w.flushPending()
		case <-w.flushCh:
			w.flushPending()
		}
	}
}

func (w *Watcher) flushPending() {
	for {
		var key string
		var obj *unstructured.Unstructured

		w.mu.Lock()
		for k, u := range w.pending {
			key = k
			obj = u
			break
		}
		w.mu.Unlock()

		if obj == nil {
			return
		}

		select {
		case w.events <- event.GenericEvent{Object: obj}:
			w.mu.Lock()
			if cur, ok := w.pending[key]; ok && cur == obj {
				delete(w.pending, key)
			}
			w.mu.Unlock()
		default:
			return
		}
	}
}

const channelFullLogInterval = 30 * time.Second

func (w *Watcher) maybeLogChannelFull(u *unstructured.Unstructured) {
	now := time.Now()

	w.mu.Lock()
	shouldLog := w.nextChannelFullLog.IsZero() || now.After(w.nextChannelFullLog)
	if shouldLog {
		w.nextChannelFullLog = now.Add(channelFullLogInterval)
	}
	w.mu.Unlock()

	if shouldLog {
		w.logger.Info("dynamicwatch channel full; coalescing events", "namespace", u.GetNamespace(), "name", u.GetName(), "apiVersion", u.GetAPIVersion(), "kind", u.GetKind())
	}
}

type logrLogger interface {
	Info(msg string, keysAndValues ...any)
}

func pendingKey(u *unstructured.Unstructured) string {
	gvk := u.GroupVersionKind()
	return gvk.Group + "/" + gvk.Version + "/" + gvk.Kind + "/" + u.GetNamespace() + "/" + u.GetName()
}

func extractUnstructured(obj any) *unstructured.Unstructured {
	switch o := obj.(type) {
	case *unstructured.Unstructured:
		return o
	case unstructured.Unstructured:
		return o.DeepCopy()
	case cache.DeletedFinalStateUnknown:
		if u, ok := o.Obj.(*unstructured.Unstructured); ok {
			return u
		}
	case *cache.DeletedFinalStateUnknown:
		if u, ok := o.Obj.(*unstructured.Unstructured); ok {
			return u
		}
	}
	return nil
}
