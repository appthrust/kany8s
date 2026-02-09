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
      - name: vpc-subnet-ids
        value:
          - "__SUBNET_ID_1__"
          - "__SUBNET_ID_2__"
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
