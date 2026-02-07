package eks

import (
	"encoding/base64"
	"fmt"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func BuildTokenKubeconfig(capiClusterName, endpoint, certificateAuthorityData, bearerToken string) ([]byte, error) {
	clusterName := strings.TrimSpace(capiClusterName)
	if clusterName == "" {
		return nil, fmt.Errorf("cluster name is required")
	}
	server := strings.TrimSpace(endpoint)
	if server == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	caData, err := decodeCertificateAuthorityData(certificateAuthorityData)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(bearerToken)
	if token == "" {
		return nil, fmt.Errorf("bearer token is required")
	}

	cfg := &clientcmdapi.Config{
		APIVersion:     "v1",
		Kind:           "Config",
		CurrentContext: clusterName,
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   server,
				CertificateAuthorityData: caData,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"aws": {
				Token: token,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			clusterName: {
				Cluster:  clusterName,
				AuthInfo: "aws",
			},
		},
	}

	out, err := clientcmd.Write(*cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal token kubeconfig: %w", err)
	}
	return out, nil
}

func BuildExecKubeconfig(capiClusterName, eksClusterName, region, endpoint, certificateAuthorityData string) ([]byte, error) {
	clusterName := strings.TrimSpace(capiClusterName)
	if clusterName == "" {
		return nil, fmt.Errorf("cluster name is required")
	}
	eksName := strings.TrimSpace(eksClusterName)
	if eksName == "" {
		return nil, fmt.Errorf("eks cluster name is required")
	}
	resolvedRegion := strings.TrimSpace(region)
	if resolvedRegion == "" {
		return nil, fmt.Errorf("region is required")
	}
	server := strings.TrimSpace(endpoint)
	if server == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	caData, err := decodeCertificateAuthorityData(certificateAuthorityData)
	if err != nil {
		return nil, err
	}

	cfg := &clientcmdapi.Config{
		APIVersion:     "v1",
		Kind:           "Config",
		CurrentContext: clusterName,
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   server,
				CertificateAuthorityData: caData,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"aws": {
				Exec: &clientcmdapi.ExecConfig{
					APIVersion: "client.authentication.k8s.io/v1beta1",
					Command:    "aws",
					Args: []string{
						"eks",
						"get-token",
						"--region",
						resolvedRegion,
						"--cluster-name",
						eksName,
					},
				},
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			clusterName: {
				Cluster:  clusterName,
				AuthInfo: "aws",
			},
		},
	}

	out, err := clientcmd.Write(*cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal exec kubeconfig: %w", err)
	}
	return out, nil
}

func decodeCertificateAuthorityData(certificateAuthorityData string) ([]byte, error) {
	trimmed := strings.TrimSpace(certificateAuthorityData)
	if trimmed == "" {
		return nil, fmt.Errorf("certificate authority data is required")
	}
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("decode certificate authority data: %w", err)
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("certificate authority data decoded to empty bytes")
	}
	return decoded, nil
}
