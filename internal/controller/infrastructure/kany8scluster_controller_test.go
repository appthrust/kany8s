/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package infrastructure

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	infrastructurev1alpha1 "github.com/reoring/kany8s/api/infrastructure/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var _ = Describe("Kany8sCluster Controller", func() {
	Context("When reconciling a resource", func() {
		It("should set status.initialization.provisioned and mark Ready=True", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			scheme := runtime.NewScheme()
			Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
			Expect(infrastructurev1alpha1.AddToScheme(scheme)).To(Succeed())

			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme:                 scheme,
				Metrics:                metricsserver.Options{BindAddress: "0"},
				HealthProbeBindAddress: "0",
				LeaderElection:         false,
			})
			Expect(err).NotTo(HaveOccurred())

			r := &Kany8sClusterReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
			Expect(r.SetupWithManager(mgr)).To(Succeed())

			go func() {
				defer GinkgoRecover()
				Expect(mgr.Start(ctx)).To(Succeed())
			}()

			c, err := client.New(cfg, client.Options{Scheme: scheme})
			Expect(err).NotTo(HaveOccurred())

			kc := &infrastructurev1alpha1.Kany8sCluster{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
			Expect(c.Create(ctx, kc)).To(Succeed())
			DeferCleanup(func() {
				_ = c.Delete(context.Background(), kc)
			})

			Eventually(func() error {
				got := &infrastructurev1alpha1.Kany8sCluster{}
				if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
					return err
				}
				if !got.Status.Initialization.Provisioned {
					return fmt.Errorf("status.initialization.provisioned = false, want true")
				}
				cond := meta.FindStatusCondition(got.Status.Conditions, "Ready")
				if cond == nil {
					return fmt.Errorf("expected Ready condition")
				}
				if cond.Status != metav1.ConditionTrue {
					return fmt.Errorf("Ready condition status = %q, want %q", cond.Status, metav1.ConditionTrue)
				}
				if got.Status.FailureReason != nil {
					return fmt.Errorf("status.failureReason = %q, want nil", *got.Status.FailureReason)
				}
				if got.Status.FailureMessage != nil {
					return fmt.Errorf("status.failureMessage = %q, want nil", *got.Status.FailureMessage)
				}
				return nil
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())
		})
	})
})
