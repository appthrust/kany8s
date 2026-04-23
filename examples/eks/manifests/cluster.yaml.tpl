apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: __CLUSTER_NAME__
  namespace: __NAMESPACE__
  # Standard: enable both plugins for this example.
  labels:
    eks.kany8s.io/karpenter: enabled
  annotations:
    eks.kany8s.io/kubeconfig-rotator: enabled
spec:
  topology:
    classRef:
      name: kany8s-eks-byo
    version: "__KUBERNETES_VERSION__"
    variables:
      - name: region
        value: "__AWS_REGION__"
      - name: eks-version
        value: "__EKS_VERSION__"
      # Subnets are split by purpose:
      #   - vpc-control-plane-subnet-ids: feeds EKS resourcesVPCConfig.subnetIDs.
      #     >=2 across >=2 AZs. NAT egress NOT required (control plane ENIs do
      #     not originate outbound traffic). Class depends on endpoint mode.
      #   - vpc-node-subnet-ids: feeds karpenter Fargate profile + default
      #     EC2NodeClass subnetSelectorTerms. Must be private with NAT
      #     default route (Fargate rejects public subnets). >=1 subnet
      #     required; >=2 across >=2 AZs recommended for HA.
      - name: vpc-control-plane-subnet-ids
        value:
          - "__CONTROL_PLANE_SUBNET_ID_1__"
          - "__CONTROL_PLANE_SUBNET_ID_2__"
      - name: vpc-node-subnet-ids
        value:
          - "__NODE_SUBNET_ID_1__"
          - "__NODE_SUBNET_ID_2__"
      - name: vpc-security-group-ids
        # Standard: let eks-karpenter-bootstrapper create/inject the node SG.
        # NOTE: When empty, the bootstrapper may also patch this control-plane SG list,
        # which can trigger an EKS VpcConfigUpdate. If you delete immediately after, you
        # may hit "Cannot delete because cluster currently has an update in progress".
        value: []
      - name: eks-public-access-cidrs
        value:
          - "__PUBLIC_ACCESS_CIDR__"
      # Standard recommended values (override if needed).
      - name: eks-access-mode
        value: API_AND_CONFIG_MAP
      - name: eks-endpoint-private-access
        value: true
      - name: eks-endpoint-public-access
        value: true
