package dynamicwatch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type Watcher struct {
	factory dynamicinformer.DynamicSharedInformerFactory
	events  chan<- event.GenericEvent

	mu        sync.Mutex
	informers map[schema.GroupVersionResource]cache.SharedIndexInformer
	stopCh    <-chan struct{}
}

func New(dynClient dynamic.Interface, events chan<- event.GenericEvent) *Watcher {
	return &Watcher{
		factory:   dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynClient, 0*time.Second, "", nil),
		events:    events,
		informers: map[schema.GroupVersionResource]cache.SharedIndexInformer{},
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
	w.mu.Unlock()

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

	select {
	case w.events <- event.GenericEvent{Object: u}:
	default:
	}
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
