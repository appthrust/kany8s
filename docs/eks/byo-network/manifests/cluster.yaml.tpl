apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: __CLUSTER_NAME__
  namespace: __NAMESPACE__
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
      #     EC2NodeClass subnetSelectorTerms. Must be private (Fargate
      #     rejects public subnets); image pulls require outbound egress,
      #     which can be provided by a NAT default route or by VPC
      #     endpoints (ecr.api, ecr.dkr, s3, sts, logs). >=1 subnet
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
        # JSON array string replacement (e.g. [] or ["sg-aaa","sg-bbb"]).
        value: __SECURITY_GROUP_IDS_JSON__
      - name: eks-public-access-cidrs
        value:
          - "__PUBLIC_ACCESS_CIDR__"
      - name: eks-access-mode
        value: "__EKS_ACCESS_MODE__"
      - name: eks-endpoint-private-access
        value: __EKS_ENDPOINT_PRIVATE_ACCESS__
      - name: eks-endpoint-public-access
        value: __EKS_ENDPOINT_PUBLIC_ACCESS__
