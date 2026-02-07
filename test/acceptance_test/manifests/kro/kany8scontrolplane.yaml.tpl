apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
kind: Kany8sControlPlane
metadata:
  name: __CLUSTER_NAME__
  namespace: __NAMESPACE__
spec:
  version: "__KUBERNETES_VERSION__"
  resourceGraphDefinitionRef:
    name: __RGD_NAME__
  kroSpec:
    name: __CLUSTER_NAME__
