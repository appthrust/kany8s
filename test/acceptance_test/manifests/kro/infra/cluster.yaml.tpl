apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: __CLUSTER_NAME__
  namespace: __NAMESPACE__
spec:
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
    kind: Kany8sCluster
    name: __CLUSTER_NAME__
    namespace: __NAMESPACE__
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: Kany8sCluster
metadata:
  name: __CLUSTER_NAME__
  namespace: __NAMESPACE__
spec:
  resourceGraphDefinitionRef:
    name: __RGD_NAME__
  kroSpec: {}
