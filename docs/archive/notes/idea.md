# ------------------------------------------------------------
# (A) EKS Cluster: base100
#  - 例として ACK EKS Cluster の主要フィールド（resourcesVPCConfig/logging/kubernetesNetworkConfig）を使用
#  - Cluster CR のフィールド名は ACK の例に合わせています（subnetIDs, securityGroupIDs, publicAccessCIDRs 等）
# ------------------------------------------------------------
apiVersion: eks.services.k8s.aws/v1alpha1
kind: Cluster
metadata:
  name: base100
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  name: base100
  version: "1.34"
  roleARN: arn:aws:iam::774305569312:role/base100-cluster-20251208091128641000000001
  accessConfig:
    authenticationMode: API_AND_CONFIG_MAP
    bootstrapClusterCreatorAdminPermissions: false

  kubernetesNetworkConfig:
    ipFamily: ipv4
    serviceIPv4CIDR: 172.20.0.0/16

  logging:
    clusterLogging:
      - enabled: true
        types:
          - api
          - audit
          - authenticator

  encryptionConfig:
    - provider:
        keyARN: arn:aws:kms:ap-northeast-1:774305569312:key/f1a42640-71c4-4988-8b0d-20ae1046d7e9
      resources:
        - secrets

  resourcesVPCConfig:
    endpointPrivateAccess: true
    endpointPublicAccess: true
    publicAccessCIDRs:
      - 0.0.0.0/0
    subnetIDs:
      - subnet-052e81f9467f8d73d
      - subnet-07771bc9796f2a1bd
      - subnet-0917c41552e84e8aa
    securityGroupIDs:
      - sg-0b0688b785fbff34d

---
# ------------------------------------------------------------
# (B) EKS Addons (state の addon_version / configuration_values を反映)
# ------------------------------------------------------------
apiVersion: eks.services.k8s.aws/v1alpha1
kind: Addon
metadata:
  name: base100-aws-ebs-csi-driver
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  clusterName: base100
  name: aws-ebs-csi-driver
  addonVersion: v1.54.0-eksbuild.1
  preserve: true

---
apiVersion: eks.services.k8s.aws/v1alpha1
kind: Addon
metadata:
  name: base100-coredns
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  clusterName: base100
  name: coredns
  addonVersion: v1.12.3-eksbuild.1
  preserve: true
  configurationValues: |
    {"computeType":"Fargate","resources":{"limits":{"cpu":"0.25","memory":"256M"},"requests":{"cpu":"0.25","memory":"256M"}}}

---
apiVersion: eks.services.k8s.aws/v1alpha1
kind: Addon
metadata:
  name: base100-eks-pod-identity-agent
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  clusterName: base100
  name: eks-pod-identity-agent
  addonVersion: v1.3.10-eksbuild.2
  preserve: true
  configurationValues: |
    {"resources":{"requests":{"cpu":"0.1","memory":"32M"}}}

---
apiVersion: eks.services.k8s.aws/v1alpha1
kind: Addon
metadata:
  name: base100-kube-proxy
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  clusterName: base100
  name: kube-proxy
  addonVersion: v1.34.0-eksbuild.2
  preserve: true
  configurationValues: |
    {"resources":{"requests":{"cpu":"0.1","memory":"64M"}}}

---
apiVersion: eks.services.k8s.aws/v1alpha1
kind: Addon
metadata:
  name: base100-snapshot-controller
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  clusterName: base100
  name: snapshot-controller
  addonVersion: v8.4.0-eksbuild.3
  preserve: true

---
apiVersion: eks.services.k8s.aws/v1alpha1
kind: Addon
metadata:
  name: base100-vpc-cni
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  clusterName: base100
  name: vpc-cni
  addonVersion: v1.20.4-eksbuild.2
  preserve: true
  configurationValues: |
    {"env":{"ENABLE_PREFIX_DELEGATION":"true","WARM_PREFIX_TARGET":"1"},"resources":{"requests":{"cpu":"0.1","memory":"128M"}}}

---
# ------------------------------------------------------------
# (C) EKS PodIdentityAssociation（state の aws_eks_pod_identity_association を CR で再現）
#  公式の PodIdentityAssociation 例と同じフィールド構造 (clusterName/namespace/serviceAccount/roleARN) を使います
# ------------------------------------------------------------
apiVersion: eks.services.k8s.aws/v1alpha1
kind: PodIdentityAssociation
metadata:
  name: base100-external-dns
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  clusterName: base100
  namespace: external-dns
  serviceAccount: external-dns
  roleARN: arn:aws:iam::774305569312:role/base100-ExternalDNSRole

---
apiVersion: eks.services.k8s.aws/v1alpha1
kind: PodIdentityAssociation
metadata:
  name: base100-aws-load-balancer-controller
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  clusterName: base100
  namespace: kube-system
  serviceAccount: aws-load-balancer-controller
  roleARN: arn:aws:iam::774305569312:role/base100-AWSLoadBalancerControllerRole

---
apiVersion: eks.services.k8s.aws/v1alpha1
kind: PodIdentityAssociation
metadata:
  name: base100-ebs-csi
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  clusterName: base100
  namespace: kube-system
  serviceAccount: ebs-csi-controller-sa
  roleARN: arn:aws:iam::774305569312:role/base100-ebs-csi-driver

---
apiVersion: eks.services.k8s.aws/v1alpha1
kind: PodIdentityAssociation
metadata:
  name: base100-loki
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  clusterName: base100
  namespace: loki
  serviceAccount: loki
  roleARN: arn:aws:iam::774305569312:role/base100-LokiRole

---
apiVersion: eks.services.k8s.aws/v1alpha1
kind: PodIdentityAssociation
metadata:
  name: base100-mimir
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  clusterName: base100
  namespace: mimir
  serviceAccount: mimir
  roleARN: arn:aws:iam::774305569312:role/base100-MimirRole

---
apiVersion: eks.services.k8s.aws/v1alpha1
kind: PodIdentityAssociation
metadata:
  name: base100-tempo
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  clusterName: base100
  namespace: tempo
  serviceAccount: tempo
  roleARN: arn:aws:iam::774305569312:role/base100-TempoRole

---
# ------------------------------------------------------------
# (D) IAM Roles（state の assume_role_policy をそのまま移植する）
#  ACK の docs にある Role の書き方（assumeRolePolicyDocument/policies/inlinePolicies）に合わせます
# ------------------------------------------------------------
apiVersion: iam.services.k8s.aws/v1alpha1
kind: Role
metadata:
  name: base100-aws-load-balancer-controller-role
spec:
  name: base100-AWSLoadBalancerControllerRole
  assumeRolePolicyDocument: |
    {"Statement":[{"Action":["sts:AssumeRole","sts:TagSession"],"Effect":"Allow","Principal":{"Service":"pods.eks.amazonaws.com"},"Sid":"AWSLoadBalancerController"}],"Version":"2012-10-17"}
  # state では “managed_policy_arns” に custom policy ARN がぶら下がっています。
  # ここでは attachment 差分を避けるため、その custom policy JSON を inlinePolicies に移植するのが安全です。
  inlinePolicies:
    base100-AWSLoadBalancerControllerPolicy: |
      <PUT_THE_EXACT_POLICY_JSON_FROM_state.aws_iam_policy.aws_load_balancer_controller.policy>

---
apiVersion: iam.services.k8s.aws/v1alpha1
kind: Role
metadata:
  name: base100-external-dns-role
spec:
  name: base100-ExternalDNSRole
  assumeRolePolicyDocument: |
    {"Statement":[{"Action":["sts:AssumeRole","sts:TagSession"],"Effect":"Allow","Principal":{"Service":"pods.eks.amazonaws.com"},"Sid":"ExternalDNSPodIdentity"}],"Version":"2012-10-17"}
  inlinePolicies:
    base100-ExternalDNSPolicy: |
      {"Statement":[{"Action":["route53:ChangeResourceRecordSets"],"Effect":"Allow","Resource":"arn:aws:route53:::hostedzone/Z0601732AQ9KUHIX88EO","Sid":"ChangeRecordsInSpecificZone"},{"Action":["route53:ListHostedZones","route53:ListResourceRecordSets","route53:ListTagsForResources","route53:GetHostedZone","route53:GetChange"],"Effect":"Allow","Resource":"*","Sid":"ListAndDiscover"}],"Version":"2012-10-17"}

---
apiVersion: iam.services.k8s.aws/v1alpha1
kind: Role
metadata:
  name: base100-loki-role
spec:
  name: base100-LokiRole
  assumeRolePolicyDocument: |
    {"Statement":[{"Action":["sts:AssumeRole","sts:TagSession"],"Effect":"Allow","Principal":{"Service":"pods.eks.amazonaws.com"},"Sid":"PodIdentityAssumeRole"}],"Version":"2012-10-17"}
  inlinePolicies:
    base100-LokiS3Policy: |
      {"Statement":[{"Action":["s3:ListBucket","s3:GetBucketLocation","s3:ListBucketMultipartUploads"],"Effect":"Allow","Resource":"*","Sid":"BucketLevel"},{"Action":["s3:GetObject","s3:PutObject","s3:DeleteObject","s3:AbortMultipartUpload","s3:ListMultipartUploadParts"],"Effect":"Allow","Resource":"arn:aws:s3:::appthrust-monitoring-loki-704bf9ec1e4b2fcf/*","Sid":"ObjectLevel"}],"Version":"2012-10-17"}

---
apiVersion: iam.services.k8s.aws/v1alpha1
kind: Role
metadata:
  name: base100-mimir-role
spec:
  name: base100-MimirRole
  assumeRolePolicyDocument: |
    {"Statement":[{"Action":["sts:AssumeRole","sts:TagSession"],"Effect":"Allow","Principal":{"Service":"pods.eks.amazonaws.com"},"Sid":"PodIdentityAssumeRole"}],"Version":"2012-10-17"}
  inlinePolicies:
    base100-MimirS3Policy: |
      {"Statement":[{"Action":["s3:ListBucket","s3:GetObject","s3:DeleteObject","s3:PutObject"],"Effect":"Allow","Resource":["arn:aws:s3:::appthrust-monitoring-mimir-437248f14a9f15ae/*","arn:aws:s3:::appthrust-monitoring-mimir-437248f14a9f15ae"],"Sid":"Statement"}],"Version":"2012-10-17"}

---
apiVersion: iam.services.k8s.aws/v1alpha1
kind: Role
metadata:
  name: base100-tempo-role
spec:
  name: base100-TempoRole
  assumeRolePolicyDocument: |
    {"Statement":[{"Action":["sts:AssumeRole","sts:TagSession"],"Effect":"Allow","Principal":{"Service":"pods.eks.amazonaws.com"},"Sid":"PodIdentityAssumeRole"}],"Version":"2012-10-17"}
  inlinePolicies:
    base100-TempoS3Policy: |
      {"Statement":[{"Action":["s3:ListBucket","s3:GetBucketLocation","s3:ListBucketMultipartUploads"],"Effect":"Allow","Resource":"*","Sid":"BucketLevel"},{"Action":["s3:GetObject","s3:PutObject","s3:DeleteObject","s3:AbortMultipartUpload","s3:ListMultipartUploadParts"],"Effect":"Allow","Resource":"arn:aws:s3:::appthrust-monitoring-tempo-dfe75216e90ba8f0/*","Sid":"ObjectLevel"}],"Version":"2012-10-17"}

---
# ------------------------------------------------------------
# (E) S3 buckets (observability_s3_buckets)
#  Bucket spec の encryption/publicAccessBlock/tagging/tagSet は Bucket reference に記載のフィールド名に合わせます
# ------------------------------------------------------------
apiVersion: s3.services.k8s.aws/v1alpha1
kind: Bucket
metadata:
  name: appthrust-monitoring-loki-704bf9ec1e4b2fcf
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  name: appthrust-monitoring-loki-704bf9ec1e4b2fcf
  encryption:
    rules:
      - applyServerSideEncryptionByDefault:
          sseAlgorithm: AES256
        bucketKeyEnabled: false
  publicAccessBlock:
    blockPublicACLs: true
    blockPublicPolicy: true
    ignorePublicACLs: true
    restrictPublicBuckets: true
  tagging:
    tagSet:
      - key: Terraform
        value: "true"
      - key: app
        value: loki
      - key: component
        value: monitoring
  # versioning: 未指定＝無効(デフォルト)に寄せる（state でも enabled:false）

---
apiVersion: s3.services.k8s.aws/v1alpha1
kind: Bucket
metadata:
  name: appthrust-monitoring-mimir-437248f14a9f15ae
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  name: appthrust-monitoring-mimir-437248f14a9f15ae
  encryption:
    rules:
      - applyServerSideEncryptionByDefault:
          sseAlgorithm: AES256
        bucketKeyEnabled: false
  publicAccessBlock:
    blockPublicACLs: true
    blockPublicPolicy: true
    ignorePublicACLs: true
    restrictPublicBuckets: true
  tagging:
    tagSet:
      - key: Terraform
        value: "true"
      - key: app
        value: mimir
      - key: component
        value: monitoring

---
apiVersion: s3.services.k8s.aws/v1alpha1
kind: Bucket
metadata:
  name: appthrust-monitoring-tempo-dfe75216e90ba8f0
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  name: appthrust-monitoring-tempo-dfe75216e90ba8f0
  encryption:
    rules:
      - applyServerSideEncryptionByDefault:
          sseAlgorithm: AES256
        bucketKeyEnabled: false
  publicAccessBlock:
    blockPublicACLs: true
    blockPublicPolicy: true
    ignorePublicACLs: true
    restrictPublicBuckets: true
  tagging:
    tagSet:
      - key: Terraform
        value: "true"
      - key: app
        value: tempo
      - key: component
        value: monitoring

---
# ------------------------------------------------------------
# (F) SQS Queue: Karpenter-base100
#  tutorial の Queue spec（queueName/policy）に加えて、state の属性を attributes で寄せる
# ------------------------------------------------------------
apiVersion: sqs.services.k8s.aws/v1alpha1
kind: Queue
metadata:
  name: karpenter-base100
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  queueName: Karpenter-base100
  attributes:
    MessageRetentionPeriod: "300"
    VisibilityTimeout: "30"
    SqsManagedSseEnabled: "true"
  policy: |
    {
      "Version": "2012-10-17",
      "Statement": [
        {
          "Sid": "SqsWrite",
          "Effect": "Allow",
          "Action": "sqs:SendMessage",
          "Resource": "arn:aws:sqs:ap-northeast-1:774305569312:Karpenter-base100",
          "Principal": { "Service": ["sqs.amazonaws.com", "events.amazonaws.com"] }
        },
        {
          "Sid": "DenyHTTP",
          "Effect": "Deny",
          "Action": "sqs:*",
          "Resource": "arn:aws:sqs:ap-northeast-1:774305569312:Karpenter-base100",
          "Principal": "*",
          "Condition": { "StringEquals": { "aws:SecureTransport": "false" } }
        }
      ]
    }

---
# ------------------------------------------------------------
# (G) EventBridge Rules -> SQS Target
#  tutorial の Rule spec（eventPattern/targets）を使って state の event_pattern に合わせる
# ------------------------------------------------------------
apiVersion: eventbridge.services.k8s.aws/v1alpha1
kind: Rule
metadata:
  name: karpenter-instance-rebalance
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  name: KarpenterInstanceRebalance-2025120809214959270000000f
  description: "Karpenter interrupt - EC2 instance rebalance recommendation"
  eventPattern: |
    {"detail-type":["EC2 Instance Rebalance Recommendation"],"source":["aws.ec2"]}
  targets:
    - arn: arn:aws:sqs:ap-northeast-1:774305569312:Karpenter-base100
      id: KarpenterInterruptionQueueTarget
      retryPolicy:
        maximumRetryAttempts: 0

---
apiVersion: eventbridge.services.k8s.aws/v1alpha1
kind: Rule
metadata:
  name: karpenter-spot-interrupt
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  name: KarpenterSpotInterrupt-20251208092149593400000010
  description: "Karpenter interrupt - EC2 spot instance interruption warning"
  eventPattern: |
    {"detail-type":["EC2 Spot Instance Interruption Warning"],"source":["aws.ec2"]}
  targets:
    - arn: arn:aws:sqs:ap-northeast-1:774305569312:Karpenter-base100
      id: KarpenterInterruptionQueueTarget
      retryPolicy:
        maximumRetryAttempts: 0

---
apiVersion: eventbridge.services.k8s.aws/v1alpha1
kind: Rule
metadata:
  name: karpenter-instance-state-change
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  name: KarpenterInstanceStateChange-20251208092149755500000017
  description: "Karpenter interrupt - EC2 instance state-change notification"
  eventPattern: |
    {"detail-type":["EC2 Instance State-change Notification"],"source":["aws.ec2"]}
  targets:
    - arn: arn:aws:sqs:ap-northeast-1:774305569312:Karpenter-base100
      id: KarpenterInterruptionQueueTarget
      retryPolicy:
        maximumRetryAttempts: 0

---
apiVersion: eventbridge.services.k8s.aws/v1alpha1
kind: Rule
metadata:
  name: karpenter-health-event
  annotations:
    services.k8s.aws/region: ap-northeast-1
spec:
  name: KarpenterHealthEvent-20251208092149874400000018
  description: "Karpenter interrupt - AWS health event"
  eventPattern: |
    {"detail-type":["AWS Health Event"],"source":["aws.health"]}
  targets:
    - arn: arn:aws:sqs:ap-northeast-1:774305569312:Karpenter-base100
      id: KarpenterInterruptionQueueTarget
      retryPolicy:
        maximumRetryAttempts: 0

---

このAckのCustomResourceを考えました。Kroを使用して少い数のリソースでこれらのリソースを作成するようにしたいです。

一点考慮したいのは、 今後Cluster API Kro (Kany8s) を作成したいという点です。このKany8sは、 Cluster API と連携して、 さまざまなクラウドプロバイダが提供しているマネージドKubernetesをKroと連携して作成できるようにする Cluster API Providerです。なので抽象的なControlPlaneを定義すると、Kroで具象的なリソースが作成されるというところです。


---- 

以下、「少ない数のKroインスタンスで、提示いただいたACK CR群（EKS Cluster/Addons/PodIdentityAssociation/IAM Role/S3/SQS/EventBridge）を組み立てる」ための設計案と、「将来のKany8s（Cluster API ProviderとしてのKro連携）」を見据えた抽象化の切り方です。

1) まず結論：**“分割RGD + 親RGDで束ねる（RGD Chaining）”**がKany8sに一番つながりやすい
**小さく責務を分けたRGD（EKS・IAM・Observability・KarpenterInterruptなど）**を作り、

親RGDがそれらのRGDインスタンスを「部品」として呼び出して束ねるのが、将来 “抽象ControlPlane → プロバイダ具象化” に移行しやすいです。

根拠（kroの合成・再利用の公式説明）
original_text

“RGD chaining allows you to compose complex applications by building on top of existing ResourceGraphDefinitions… you can create instances of one RGD within another RGD's resource graph.” 

source
kro docs – RGD Chaining 

note
“巨大な1枚RGD”にすると、将来プロバイダ追加（AKS/GKE等）で再利用が効きにくいです。RGDを部品化しておくと、Kany8s側で「AWS向けはEKS部品を使う」「GCP向けはGKE部品を使う」がやりやすくなります。

2) 依存関係の要点：IAM Role → EKS Cluster → Addon/PodIdentityAssociation を強制する
ACKは参照先が無い状態で作ると、あとから揃っても復帰しない“Terminal”に落ちるケースが報告されています。なので kroのDAG/順序制御で先にRole等を作らせるのが重要です。

根拠（roleが無いとTerminalになり復帰しない事例）
original_text

“if role does not exists when cluster is created (race condition), it shows ACK.Terminal condition in cluster status and never gets resolved even role is created successfully in next 1-2 seconds.” 

source
aws-controllers-k8s/community issue #1844 

note
あなたの構成は Cluster.spec.roleARN を参照するので、Role作成をkro側で先行させる（依存関係を張る／readyWhenで待つ）設計が安全です。

kroが順序を扱う仕組み（DAG）
original_text

“kro… Treats your resources as a Directed Acyclic Graph (DAG) to understand their dependencies… detects the correct deployment order” 

source
kro docs – Quick Start 

note
kroの式参照（${...}）で依存関係が推論されるので、RoleのARN/Name等をCluster側テンプレートで参照する形に寄せると、自然に順序が付きます（明示依存より壊れにくい）。

3) “部品RGD”のおすすめ分割（今のYAMLをそのまま包含できる粒度）
あなたの提示（A〜G）は、だいたい次の5部品に分けると良いです。

EKSControlPlane（ACK EKS Cluster + AccessConfig + logging + encryption + VPC）

EKSAddons（Addon群）

PodIdentitySet（PodIdentityAssociation群 + その前提Role群）

ObservabilityBuckets（S3 3つ）

KarpenterInterrupt（SQS Queue + EventBridge Rules）

親RGD（例：PlatformCluster）は上のRGDインスタンスを呼び、入力（clusterName/region/version/subnets等）を配って束ねます。

4) ACKリソースのフィールド形状（kroのテンプレ化で効くポイント）
PodIdentityAssociation の必須キー（clusterName/namespace/serviceAccount/roleARN）
original_text

“You create a Pod Identity Association that links: * An IAM role… * A Kubernetes service account… * A namespace…” 

“spec: clusterName: … namespace: … serviceAccount: … roleARN: …” 

source
ACK docs – Configure IAM Permissions (Pod Identity Association例) 

note
あなたの(C)はこの形に揃っていて、kro schemaで podIdentities: map 的にまとめるのがやりやすいです（例：external-dns, loki, mimir, tempo…）。

SQS Queue の最低限（queueName/policy）
original_text

“apiVersion: sqs.services.k8s.aws/v1alpha1 … kind: Queue … spec: queueName: ${QUEUE_NAME} … policy: | { … }” 

source
ACK tutorial – Manage SQS queues with ACK SQS Controller 

note
あなたの(F)は attributes も含めているので、schemaで queue.attributes を任意にしてテンプレへ流し込むのがよいです。

EventBridge Rule → SQS Target（targets/arn/id/retryPolicy）
original_text

“kind: Rule … spec: … eventPattern: | { … } … targets: - arn: $TARGET_QUEUE_ARN … id: … retryPolicy: maximumRetryAttempts: 0” 

source
ACK tutorial – EventBridge example 

note
あなたの(G)と形が一致しています。kro側では「QueueのARNを式で参照」できると依存が自動で付きます（後述）。

5) 具体例：親RGD（PlatformCluster）が部品RGDを呼ぶ形（RGD Chaining）
ここでは「親RGDが部品RGDのインスタンスを resources に置く」パターンだけ示します（部品RGDの中身は次セクションの雛形へ）。

apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: platform-cluster
spec:
  schema:
    apiVersion: kro.run/v1alpha1
    kind: PlatformCluster
    spec:
      name: string
      region: string | default="ap-northeast-1"
      kubernetesVersion: string
      vpc:
        subnetIDs: string[]
        securityGroupIDs: string[]
        endpointPrivateAccess: boolean | default=true
        endpointPublicAccess: boolean | default=true
        publicAccessCIDRs: string[] | default=["0.0.0.0/0"]
      observability:
        lokiBucketName: string
        mimirBucketName: string
        tempoBucketName: string
      karpenter:
        queueName: string
      podIdentities:
        externalDNSRoleName: string
        albControllerRoleName: string
        ebsCsiRoleName: string
        lokiRoleName: string
        mimirRoleName: string
        tempoRoleName: string
    status:
      # 例：部品RGDが出す ready や ARN 等を上に持ち上げる
      controlPlaneReady: ${eks.status.ready}
      interruptionQueueARN: ${karpenter.status.queueARN}

  resources:
    - id: observability
      template:
        apiVersion: kro.run/v1alpha1
        kind: ObservabilityBuckets
        metadata:
          name: ${schema.spec.name}-obs
        spec:
          region: ${schema.spec.region}
          lokiBucketName: ${schema.spec.observability.lokiBucketName}
          mimirBucketName: ${schema.spec.observability.mimirBucketName}
          tempoBucketName: ${schema.spec.observability.tempoBucketName}

    - id: iamAndPodIdentity
      template:
        apiVersion: kro.run/v1alpha1
        kind: PodIdentitySet
        metadata:
          name: ${schema.spec.name}-pi
        spec:
          region: ${schema.spec.region}
          clusterName: ${schema.spec.name}
          podIdentities: ${schema.spec.podIdentities}
      # ここで readyWhen を使って「Role群が出来た」を明示してもOK（部品側でreadyを立てるのが綺麗）

    - id: eks
      template:
        apiVersion: kro.run/v1alpha1
        kind: EKSControlPlane
        metadata:
          name: ${schema.spec.name}-cp
        spec:
          name: ${schema.spec.name}
          region: ${schema.spec.region}
          kubernetesVersion: ${schema.spec.kubernetesVersion}
          vpc: ${schema.spec.vpc}
      # 依存：iamAndPodIdentity の出力（roleARN等）を eks 側で参照する形にするとDAGが自動で付く

    - id: addons
      template:
        apiVersion: kro.run/v1alpha1
        kind: EKSAddons
        metadata:
          name: ${schema.spec.name}-addons
        spec:
          region: ${schema.spec.region}
          clusterName: ${schema.spec.name}
      # 依存：cluster が出来てから。addons側で ${eks.status.ready} を参照すると自動依存になります

    - id: karpenter
      template:
        apiVersion: kro.run/v1alpha1
        kind: KarpenterInterrupt
        metadata:
          name: ${schema.spec.name}-karp
        spec:
          region: ${schema.spec.region}
          queueName: ${schema.spec.karpenter.queueName}
これが「将来Kany8s」へ効く理由
親RGDの PlatformCluster.spec は プロバイダ非依存の入力（name/region/version/vpc/observability 等）に近い形にできる

AWS以外を追加したら、親RGDはそのまま、差し替えるのは EKSControlPlane 相当部品だけ、にしやすい

6) 部品RGDの雛形（ACK CRを直接resources.templateに置く）
(a) KarpenterInterrupt（SQS + EventBridge Rules）例
SQS/Ruleの形はACKチュートリアルの通りで、そのままテンプレ化できます。 

apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: karpenter-interrupt
spec:
  schema:
    apiVersion: kro.run/v1alpha1
    kind: KarpenterInterrupt
    spec:
      region: string
      queueName: string
      messageRetentionPeriod: string | default="300"
      visibilityTimeout: string | default="30"
      sseEnabled: string | default="true"
    status:
      queueARN: ${queue.status.ackResourceMetadata.arn}

  resources:
    - id: queue
      template:
        apiVersion: sqs.services.k8s.aws/v1alpha1
        kind: Queue
        metadata:
          name: ${schema.spec.queueName}
          annotations:
            services.k8s.aws/region: ${schema.spec.region}
        spec:
          queueName: ${schema.spec.queueName}
          attributes:
            MessageRetentionPeriod: ${schema.spec.messageRetentionPeriod}
            VisibilityTimeout: ${schema.spec.visibilityTimeout}
            SqsManagedSseEnabled: ${schema.spec.sseEnabled}
          # policy はあなたの現行JSONをそのまま貼る（kro schemaで文字列として受けてもよい）

    - id: ruleSpotInterrupt
      template:
        apiVersion: eventbridge.services.k8s.aws/v1alpha1
        kind: Rule
        metadata:
          name: ${schema.spec.queueName}-spot-interrupt
          annotations:
            services.k8s.aws/region: ${schema.spec.region}
        spec:
          name: ${schema.spec.queueName}-spot-interrupt
          description: "Karpenter interrupt - EC2 spot instance interruption warning"
          eventPattern: |
            {"detail-type":["EC2 Spot Instance Interruption Warning"],"source":["aws.ec2"]}
          targets:
            - arn: ${queue.status.ackResourceMetadata.arn}   # ※実際はSQSのARNフィールドに合わせる
              id: KarpenterInterruptionQueueTarget
              retryPolicy:
                maximumRetryAttempts: 0
ここは実際には「QueueのARNが status.ackResourceMetadata.arn で取れるか」をあなたの環境のACK SQS CRDで確認して、フィールドを合わせてください（上は雛形です）。
ただし「RuleがQueueのstatusを参照」する形にできると、kroが依存関係を自動で張ります。 

7) Kany8sを見据えた「抽象ControlPlane」の置き場所（Cluster APIとの整合）
Kany8sを作るなら、最終的に Cluster APIの“ControlPlane provider contract”を満たすCRD が必要になります。

original_text

“Control plane providers MUST implement a ControlPlane resource using Kubernetes’ CustomResourceDefinition (CRD).” 

source
Cluster API Book – Contract rules for ControlPlane 

note
Kany8sの“表の顔”は、このControlPlane CR（例：Kany8sControlPlane）にして、そのspecを入力として kro（親RGD）を起動し、ACKや各クラウドの具象リソースを作るのが一番自然です。

また、Cluster APIではEKS等は「外部/マネージドControlPlane」に分類されます。

original_text

“External or Managed control planes are offered and controlled by some system other than Cluster API, such as GKE, AKS, EKS, or IKS.” 

source
Cluster API Book – Concepts 

note
つまりKany8sは「EKSという外部ControlPlaneを作る責務」を持ちやすいので、あなたが今やっているACK EKS Cluster生成は、方向性としてかなり合っています。

8) 次にやると一気に前に進む“実装タスク”3つ
部品RGDを先に作る（KarpenterInterrupt から始めるのが簡単）

SQS + EventBridge は自己完結しやすい（EKSクラスタ作成より安全）

PodIdentitySet 部品RGDで “Role→PodIdentityAssociation” の順序を確実化

Terminal落ちを避ける（上のissue根拠） 

親RGD（PlatformCluster）で部品RGDをチェーン

将来Kany8sでこの親RGDを“具象化エンジン”として再利用 

必要なら、あなたの提示YAML（A〜G）を **「部品RGD 5枚 + 親RGD 1枚」**に、実フィールド（ARN参照やstatus参照）まで合わせた“動く雛形”に落として提示します（特に Queue ARN / EKS Cluster Ready判定 / Addon依存 の3点は環境のCRDフィールドに合わせて詰めるのがコツです）。





