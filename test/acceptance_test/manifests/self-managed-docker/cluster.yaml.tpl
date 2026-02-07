apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: __CLUSTER_NAME__
  namespace: __NAMESPACE__
spec:
  infrastructureRef:
    apiGroup: infrastructure.cluster.x-k8s.io
    kind: DockerCluster
    name: __CLUSTER_NAME__
  controlPlaneRef:
    apiGroup: controlplane.cluster.x-k8s.io
    kind: Kany8sControlPlane
    name: __CLUSTER_NAME__
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: DockerCluster
metadata:
  name: __CLUSTER_NAME__
  namespace: __NAMESPACE__
spec: {}
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: DockerMachineTemplate
metadata:
  name: __CLUSTER_NAME__-control-plane
  namespace: __NAMESPACE__
spec:
  template:
    spec:
      customImage: __NODE_IMAGE__
---
apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
kind: Kany8sControlPlane
metadata:
  name: __CLUSTER_NAME__
  namespace: __NAMESPACE__
spec:
  version: __KUBERNETES_VERSION__
  kubeadm:
    machineTemplate:
      infrastructureRef:
        apiGroup: infrastructure.cluster.x-k8s.io
        kind: DockerMachineTemplate
        name: __CLUSTER_NAME__-control-plane
