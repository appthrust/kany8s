package kubeconfig

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ClusterNameLabelKey = "cluster.x-k8s.io/cluster-name"
	DataKey             = "value"
)

const SecretType corev1.SecretType = "cluster.x-k8s.io/secret"

func SecretName(clusterName string) (string, error) {
	name := strings.TrimSpace(clusterName)
	if name == "" {
		return "", fmt.Errorf("cluster name is required")
	}
	return name + "-kubeconfig", nil
}

func NewSecret(clusterName, namespace string, kubeconfig []byte) (*corev1.Secret, error) {
	cluster := strings.TrimSpace(clusterName)
	if cluster == "" {
		return nil, fmt.Errorf("cluster name is required")
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	secretName, err := SecretName(cluster)
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				ClusterNameLabelKey: cluster,
			},
		},
		Type: SecretType,
		Data: map[string][]byte{
			DataKey: kubeconfig,
		},
	}, nil
}
