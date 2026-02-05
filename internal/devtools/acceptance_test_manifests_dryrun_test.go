package devtools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestKroInfraAcceptanceClusterTemplateRendersAndDryRunsAgainstAPIServer(t *testing.T) {
	root := findRepoRoot(t)

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join(root, "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	crdClient, err := apiextensionsclientset.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("create apiextensions clientset: %v", err)
	}
	installStubCAPIClusterCRD(t, ctx, crdClient)

	k8sClient, err := client.New(cfg, client.Options{Scheme: clientgoscheme.Scheme})
	if err != nil {
		t.Fatalf("create controller-runtime client: %v", err)
	}

	tplPath := filepath.Join(root, "test", "acceptance_test", "manifests", "kro", "infra", "cluster.yaml.tpl")
	tplBytes, err := os.ReadFile(tplPath)
	if err != nil {
		t.Fatalf("read %q: %v", tplPath, err)
	}

	replacer := strings.NewReplacer(
		"__CLUSTER_NAME__", demoClusterName,
		"__NAMESPACE__", demoNamespace,
		"__RGD_NAME__", "demo-infra-ownerref.kro.run",
	)
	rendered := replacer.Replace(string(tplBytes))
	for _, placeholder := range []string{"__CLUSTER_NAME__", "__NAMESPACE__", "__RGD_NAME__"} {
		if strings.Contains(rendered, placeholder) {
			t.Fatalf("%s rendered output still contains %q", filepath.ToSlash(tplPath), placeholder)
		}
	}

	objs := decodeUnstructuredYAMLDocuments(t, tplPath, rendered)
	if got, want := len(objs), 2; got != want {
		t.Fatalf("%s decoded %d objects, want %d", filepath.ToSlash(tplPath), got, want)
	}

	for i := range objs {
		obj := &objs[i]
		if err := k8sClient.Create(ctx, obj, &client.CreateOptions{DryRun: []string{metav1.DryRunAll}}); err != nil {
			t.Fatalf("%s %s/%s dry-run create: %v", obj.GroupVersionKind().String(), obj.GetNamespace(), obj.GetName(), err)
		}
	}
}

func installStubCAPIClusterCRD(t *testing.T, ctx context.Context, crdClient *apiextensionsclientset.Clientset) {
	t.Helper()

	crd := stubCAPIClusterCRD()
	if _, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, crd, metav1.CreateOptions{}); err != nil {
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

func stubCAPIClusterCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "clusters.cluster.x-k8s.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "cluster.x-k8s.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "clusters",
				Singular: "cluster",
				Kind:     "Cluster",
				ListKind: "ClusterList",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1beta2",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"spec": {
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"infrastructureRef": {
											Type: "object",
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"apiVersion": {Type: "string"},
												"kind":       {Type: "string"},
												"name":       {Type: "string"},
												"namespace":  {Type: "string"},
											},
											Required: []string{"apiVersion", "kind", "name", "namespace"},
										},
									},
									Required: []string{"infrastructureRef"},
								},
							},
						},
					},
				},
			},
		},
	}
}
