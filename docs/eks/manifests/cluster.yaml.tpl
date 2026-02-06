apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: __CLUSTER_NAME__
  namespace: __NAMESPACE__
spec:
  infrastructureRef:
    apiGroup: infrastructure.cluster.x-k8s.io
    kind: Kany8sCluster
    name: __CLUSTER_NAME__
  controlPlaneRef:
    apiGroup: controlplane.cluster.x-k8s.io
    kind: Kany8sControlPlane
    name: __CLUSTER_NAME__
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: Kany8sCluster
metadata:
  name: __CLUSTER_NAME__
  namespace: __NAMESPACE__
spec: {}
---
apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
kind: Kany8sControlPlane
metadata:
  name: __CLUSTER_NAME__
  namespace: __NAMESPACE__
spec:
  version: "__KUBERNETES_VERSION__"
  resourceGraphDefinitionRef:
    name: eks-control-plane-smoke.kro.run
  kroSpec:
    region: "__AWS_REGION__"
    vpc:
      cidrBlock: "__VPC_CIDR__"
      subnetA:
        cidrBlock: "__SUBNET_A_CIDR__"
        availabilityZone: "__SUBNET_A_AZ__"
      subnetB:
        cidrBlock: "__SUBNET_B_CIDR__"
        availabilityZone: "__SUBNET_B_AZ__"
    # Safer default for testing: restrict to your public IP (/32)
    # publicAccessCIDRs:
    #   - "203.0.113.10/32"
