package infrastructure

import (
	"context"
	"testing"

	infrastructurev1alpha1 "github.com/reoring/kany8s/api/infrastructure/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestKany8sClusterReconciler_SetsReadyConditionTrue(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := infrastructurev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sCluster scheme: %v", err)
	}

	kc := &infrastructurev1alpha1.Kany8sCluster{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kc).WithStatusSubresource(kc).Build()
	r := &Kany8sClusterReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &infrastructurev1alpha1.Kany8sCluster{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get Kany8sCluster: %v", err)
	}

	cond := meta.FindStatusCondition(got.Status.Conditions, "Ready")
	if cond == nil {
		t.Fatalf("expected Ready condition")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Fatalf("Ready condition status = %q, want %q", cond.Status, metav1.ConditionTrue)
	}
}
