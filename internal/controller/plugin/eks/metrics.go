package eks

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	metricControllerRotator      = "eks-kubeconfig-rotator"
	metricControllerBootstrapper = "eks-karpenter-bootstrapper"
)

var (
	reconcileErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kany8s_eks_plugin_reconcile_errors_total",
			Help: "Total number of reconcile errors by EKS plugin controller.",
		},
		[]string{"controller"},
	)
	tokenGenerationFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kany8s_eks_plugin_token_generation_failures_total",
			Help: "Total number of EKS token generation failures by controller.",
		},
		[]string{"controller"},
	)
	ownershipConflictsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kany8s_eks_plugin_ownership_conflicts_total",
			Help: "Total number of managed resource ownership conflicts by controller and resource kind.",
		},
		[]string{"controller", "resource"},
	)
	lastSuccessfulSyncUnix = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kany8s_eks_plugin_last_successful_sync_unix_seconds",
			Help: "Unix timestamp of the last successful sync by controller.",
		},
		[]string{"controller"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		reconcileErrorsTotal,
		tokenGenerationFailuresTotal,
		ownershipConflictsTotal,
		lastSuccessfulSyncUnix,
	)
}

func recordReconcileError(controller string) {
	reconcileErrorsTotal.WithLabelValues(strings.TrimSpace(controller)).Inc()
}

func recordTokenGenerationFailure(controller string) {
	tokenGenerationFailuresTotal.WithLabelValues(strings.TrimSpace(controller)).Inc()
}

func recordOwnershipConflict(controller, resource string) {
	ownershipConflictsTotal.WithLabelValues(strings.TrimSpace(controller), strings.TrimSpace(resource)).Inc()
}

func recordSuccessfulSync(controller string, now time.Time) {
	lastSuccessfulSyncUnix.WithLabelValues(strings.TrimSpace(controller)).Set(float64(now.UTC().Unix()))
}
