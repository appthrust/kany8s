package eks

import (
	"context"
	"fmt"
	"strings"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ackClusterNameIndexValues(obj client.Object) []string {
	cluster, ok := obj.(*clusterv1.Cluster)
	if !ok || cluster == nil {
		return nil
	}
	_, _, ackClusterName := resolveClusterNames(cluster)
	ackClusterName = strings.TrimSpace(ackClusterName)
	if ackClusterName == "" {
		return nil
	}
	return []string{ackClusterName}
}

func ensureACKClusterNameIndex(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(ctx, &clusterv1.Cluster{}, ackClusterNameIndexKey, ackClusterNameIndexValues); err != nil {
		// Multiple controllers may register the same index on the same manager.
		// Treat duplicate registration as success.
		errText := err.Error()
		if strings.Contains(errText, "already exists") || strings.Contains(errText, "indexer conflict") {
			return nil
		}
		return fmt.Errorf("index CAPI Cluster by ACK cluster name: %w", err)
	}
	return nil
}
