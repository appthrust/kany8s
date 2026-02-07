package eks

import (
	"encoding/base64"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
)

func TestBuildTokenKubeconfig(t *testing.T) {
	t.Parallel()

	ca := base64.StdEncoding.EncodeToString([]byte("test-ca"))
	data, err := BuildTokenKubeconfig("demo", "https://example.com", ca, "token-123")
	if err != nil {
		t.Fatalf("BuildTokenKubeconfig() error = %v", err)
	}

	cfg, err := clientcmd.Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.CurrentContext, "demo"; got != want {
		t.Fatalf("current context = %q, want %q", got, want)
	}
	cluster := cfg.Clusters["demo"]
	if cluster == nil {
		t.Fatalf("cluster demo not found")
	}
	if got, want := cluster.Server, "https://example.com"; got != want {
		t.Fatalf("cluster server = %q, want %q", got, want)
	}
	if got, want := string(cluster.CertificateAuthorityData), "test-ca"; got != want {
		t.Fatalf("cluster CA = %q, want %q", got, want)
	}
	auth := cfg.AuthInfos["aws"]
	if auth == nil {
		t.Fatalf("auth aws not found")
	}
	if got, want := auth.Token, "token-123"; got != want {
		t.Fatalf("auth token = %q, want %q", got, want)
	}
}

func TestBuildExecKubeconfig(t *testing.T) {
	t.Parallel()

	ca := base64.StdEncoding.EncodeToString([]byte("test-ca"))
	data, err := BuildExecKubeconfig("demo", "eks-demo", "ap-northeast-1", "https://example.com", ca)
	if err != nil {
		t.Fatalf("BuildExecKubeconfig() error = %v", err)
	}

	cfg, err := clientcmd.Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	auth := cfg.AuthInfos["aws"]
	if auth == nil || auth.Exec == nil {
		t.Fatalf("exec auth config not found")
	}
	if got, want := auth.Exec.Command, "aws"; got != want {
		t.Fatalf("exec command = %q, want %q", got, want)
	}
	wantArgs := []string{"eks", "get-token", "--region", "ap-northeast-1", "--cluster-name", "eks-demo"}
	if len(auth.Exec.Args) != len(wantArgs) {
		t.Fatalf("exec args len = %d, want %d", len(auth.Exec.Args), len(wantArgs))
	}
	for i := range wantArgs {
		if auth.Exec.Args[i] != wantArgs[i] {
			t.Fatalf("exec args[%d] = %q, want %q", i, auth.Exec.Args[i], wantArgs[i])
		}
	}
}
