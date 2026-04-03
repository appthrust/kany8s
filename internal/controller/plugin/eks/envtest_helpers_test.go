package eks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type envtestHarness struct {
	testEnv *envtest.Environment
	cfg     *rest.Config
	client  client.Client
	scheme  *runtime.Scheme
}

func startEKSEnvtestHarness(t *testing.T, crds ...*apiextensionsv1.CustomResourceDefinition) *envtestHarness {
	t.Helper()

	assetsDir := os.Getenv("KUBEBUILDER_ASSETS")
	if assetsDir == "" {
		assetsDir = filepath.Join(string(os.PathSeparator), "usr", "local", "kubebuilder", "bin")
	}
	for _, bin := range []string{"etcd", "kube-apiserver"} {
		if _, err := os.Stat(filepath.Join(assetsDir, bin)); err != nil {
			t.Skipf("envtest assets not found (%s): %v", filepath.ToSlash(filepath.Join(assetsDir, bin)), err)
		}
	}

	testEnv := &envtest.Environment{
		BinaryAssetsDirectory: assetsDir,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := testEnv.Stop(); err != nil {
			t.Fatalf("stop envtest: %v", err)
		}
	})

	crdClient, err := apiextensionsclientset.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("new apiextensions client: %v", err)
	}
	installStubCRDs(t, crdClient, crds...)

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new controller-runtime client: %v", err)
	}

	return &envtestHarness{
		testEnv: testEnv,
		cfg:     cfg,
		client:  k8sClient,
		scheme:  scheme,
	}
}

func installStubCRDs(t *testing.T, crdClient *apiextensionsclientset.Clientset, crds ...*apiextensionsv1.CustomResourceDefinition) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	for _, crd := range crds {
		crd := crd.DeepCopy()
		if _, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, crd, metav1.CreateOptions{}); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			t.Fatalf("create stub CRD %q: %v", crd.Name, err)
		}
		if err := wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 20*time.Second, true, func(ctx context.Context) (bool, error) {
			got, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			for _, cond := range got.Status.Conditions {
				if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
					return true, nil
				}
			}
			return false, nil
		}); err != nil {
			t.Fatalf("wait for stub CRD %q to establish: %v", crd.Name, err)
		}
	}
}

func stubNamespacedCRD(group, version, kind, plural string) *apiextensionsv1.CustomResourceDefinition {
	preserveUnknown := true
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s.%s", plural, group),
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   plural,
				Singular: strings.ToLower(kind),
				Kind:     kind,
				ListKind: kind + "List",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    version,
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"spec": {
									Type:                   "object",
									XPreserveUnknownFields: &preserveUnknown,
								},
								"status": {
									Type:                   "object",
									XPreserveUnknownFields: &preserveUnknown,
								},
							},
						},
					},
				},
			},
		},
	}
}

func stubCAPIClusterCRD() *apiextensionsv1.CustomResourceDefinition {
	return stubNamespacedCRD("cluster.x-k8s.io", "v1beta2", "Cluster", "clusters")
}
