# WIP: kind(management) + 既存 EKS(BYO) で Fargate + Karpenter を動かす検証

## TL;DR (2026-02-08)

- `eks-karpenter-bootstrapper` + Flux(remote HelmRelease) + CRS で、Node=0 から `coredns`/`karpenter` を Fargate で起動し、NodePool により EC2 node join まで確認
- ブロッカーになりやすい点: private subnet + NAT/VPC endpoints 必須 / Pod は FargateProfile 作成前だと Fargate に載らない（bootstrapper が rollout restart で回避）
- 残タスク: cleanup e2e（InstanceProfile/Role deletion の収束確認）、dev reset スクリプト

- 日付: 2026-02-07 (initial), 2026-02-08 (clean slate + automation)
- management cluster context: `kind-kany8s-eks` (kind cluster: `kany8s-eks`)
- 対象 CAPI Cluster: `default/demo-eks-byo-135-20260207121611`
- 対象 ACK EKS Cluster: `default/demo-eks-byo-135-20260207121611-szh42`
- Update(2026-02-08): 最新 CAPI Cluster: `default/demo-eks-byo-135-20260208121023`
- Update(2026-02-08): 最新 ACK EKS Cluster: `default/demo-eks-byo-135-20260208121023-ghs8f`
- Update(2026-02-08): 最新 CAPI Cluster(再実行): `default/demo-eks-byo-135-20260208070010`
- Update(2026-02-08): 最新 ACK EKS Cluster(再実行): `default/demo-eks-byo-135-20260208070010-xx6m4`
- AWS region: `ap-northeast-1`
- Karpenter chart version: `1.0.8`

このファイルは「今回のセッションで実際に叩いた手順/観測」をメモしています。

## 実施内容

### 1) Flux(Management) を導入

Flux CLI は未導入だったため、`install.yaml` をそのまま apply:

```bash
kubectl apply -f https://github.com/fluxcd/flux2/releases/latest/download/install.yaml
kubectl -n flux-system rollout status deploy/source-controller --timeout=300s
kubectl -n flux-system rollout status deploy/helm-controller --timeout=300s

kubectl api-resources --api-group=source.toolkit.fluxcd.io | rg OCIRepository
kubectl api-resources --api-group=helm.toolkit.fluxcd.io | rg HelmRelease
```

### 2) `eks-karpenter-bootstrapper` を kind(management) にデプロイ

ローカルで image build -> kind load -> apply:

```bash
EKS_KARPENTER_BOOTSTRAPPER_IMG=example.com/eks-karpenter-bootstrapper:dev

make docker-build-eks-karpenter-bootstrapper EKS_KARPENTER_BOOTSTRAPPER_IMG=$EKS_KARPENTER_BOOTSTRAPPER_IMG
kind load docker-image $EKS_KARPENTER_BOOTSTRAPPER_IMG --name kany8s-eks
make deploy-eks-karpenter-bootstrapper EKS_KARPENTER_BOOTSTRAPPER_IMG=$EKS_KARPENTER_BOOTSTRAPPER_IMG
kubectl -n ack-system rollout status deploy/eks-karpenter-bootstrapper --timeout=180s
```

途中で遭遇した問題と対応:

- OIDC Provider の spec フィールド名が ACK の CRD と不一致
  - 修正: `spec.clientIDList` -> `spec.clientIDs`
- bootstrapper が ConfigMap を watch/list できず cache reflector がエラー
  - 修正: `configmaps` に `list/watch` を追加

### 3) BYO RGD/ClusterClass を更新（AccessEntry/endpoint 設定）

management cluster に最新の RGD/ClusterClass を apply:

```bash
kubectl apply -f docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml
kubectl -n default apply -f docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml
```

既存の `Cluster` には次が追加/反映されました（Topology variables）:

- `eks-access-mode=API_AND_CONFIG_MAP`
- `eks-endpoint-private-access=true`
- `eks-endpoint-public-access=true`

### 4) Karpenter node 用 SecurityGroup を用意して Cluster variable を更新

EKS の cluster SG のみだと node SG の扱いが空のため、node 用 SG を作成して `vpc-security-group-ids` に注入しました。

NOTE:

- これは現状の workaround（command ops）。
- 最終的には `eks-karpenter-bootstrapper` が ACK `SecurityGroup` を作成し、Topology variable へ注入して手作業を無くす（`docs/eks/fargate/plan.md`）。
- Update: bootstrapper 側に node SG の自動作成 + `vpc-security-group-ids` 注入を実装したため、この手順は最終的に不要になる。

```bash
AWS_REGION=ap-northeast-1
EKS_NAME=demo-eks-byo-135-20260207121611-szh42

VPC_ID=$(aws eks describe-cluster --region $AWS_REGION --name $EKS_NAME --query 'cluster.resourcesVpcConfig.vpcId' --output text)
CLUSTER_SG=$(aws eks describe-cluster --region $AWS_REGION --name $EKS_NAME --query 'cluster.resourcesVpcConfig.clusterSecurityGroupId' --output text)

SG_NAME="${EKS_NAME}-karpenter-node"
NODE_SG=$(aws ec2 create-security-group --region $AWS_REGION --vpc-id $VPC_ID --group-name $SG_NAME --description "Karpenter nodes for $EKS_NAME" --query GroupId --output text)

# self all
aws ec2 authorize-security-group-ingress --region $AWS_REGION --group-id $NODE_SG \
  --ip-permissions "[{\"IpProtocol\":\"-1\",\"UserIdGroupPairs\":[{\"GroupId\":\"$NODE_SG\"}]}]" || true

# allow kubelet from cluster SG
aws ec2 authorize-security-group-ingress --region $AWS_REGION --group-id $NODE_SG \
  --ip-permissions "[{\"IpProtocol\":\"tcp\",\"FromPort\":10250,\"ToPort\":10250,\"UserIdGroupPairs\":[{\"GroupId\":\"$CLUSTER_SG\"}]}]" || true
```

作成した SG:

- node SG: `sg-0ae149e3291c9b615`

`Cluster` 側の `vpc-security-group-ids` を更新:

```bash
kubectl -n default patch cluster.cluster.x-k8s.io demo-eks-byo-135-20260207121611 \
  --type=json -p "[{'op':'replace','path':'/spec/topology/variables/3/value','value':['sg-0ae149e3291c9b615']}]"
```

### 5) opt-in: Karpenter bootstrap を有効化

```bash
kubectl -n default label cluster.cluster.x-k8s.io demo-eks-byo-135-20260207121611 eks.kany8s.io/karpenter=enabled --overwrite
```

### 6) EKS(ControlPlane) 側の更新が収束するまで待機

RGD/ClusterClass 更新により、ACK EKS Cluster の spec が更新され `UPDATING -> ACTIVE` を待機。

反映された主な値:

- `spec.accessConfig.authenticationMode=API_AND_CONFIG_MAP`
- `spec.resourcesVPCConfig.endpointPrivateAccess=true`
- `spec.resourcesVPCConfig.securityGroupIDs=[sg-0ae149e3291c9b615]`

### 7) bootstrapper が生成したリソースを確認

```bash
CLUSTER_NAME=demo-eks-byo-135-20260207121611
kubectl -n default get \
  openidconnectproviders.iam.services.k8s.aws,roles.iam.services.k8s.aws,policies.iam.services.k8s.aws,\
  accessentries.eks.services.k8s.aws,fargateprofiles.eks.services.k8s.aws,\
  ocirepositories.source.toolkit.fluxcd.io,helmreleases.helm.toolkit.fluxcd.io,\
  configmaps,clusterresourcesets.addons.cluster.x-k8s.io \
  -l cluster.x-k8s.io/cluster-name="$CLUSTER_NAME" -o wide
```

観測:

- IAM/OIDC/Role/Policy/AccessEntry/FargateProfile/Flux/CRS/ConfigMap は生成される
- `OCIRepository` は `READY=True`
- `HelmRelease` は install が 5m timeout で失敗し、`Stalled=True` になる
  - Update: 後続の修正で `timeout` 延長 + `disableWait` + `remediation.retries=-1` を入れ、収束するようにした。

## 現状のステータス (途中)

### Workload cluster (remote kubeconfig で確認)

`<cluster>-kubeconfig` Secret を使って remote へ接続:

```bash
CLUSTER_NAME=demo-eks-byo-135-20260207121611
kube=/tmp/${CLUSTER_NAME}.kubeconfig
kubectl -n default get secret "${CLUSTER_NAME}-kubeconfig" -o jsonpath='{.data.value}' | base64 -d > "$kube"
chmod 600 "$kube"

kubectl --kubeconfig "$kube" get ns
kubectl --kubeconfig "$kube" get nodes -o wide
kubectl --kubeconfig "$kube" get pods -A -o wide
```

観測:

- nodes: 0
- `kube-system/coredns`: Pending (`no nodes available to schedule pods`)
- `karpenter/karpenter`: Pending (`no nodes available to schedule pods`)

### FargateProfile

AWS 側:

```bash
aws eks list-fargate-profiles --region ap-northeast-1 --cluster-name demo-eks-byo-135-20260207121611-szh42
```

観測:

- `karpenter` は `ACTIVE`
- `coredns` は未作成のまま
  - ACK 側の `coredns` FargateProfile は `ResourceInUseException` (別 profile が CREATING の間は作れない) の recoverable に入る

### Flux HelmRelease (Karpenter)

- `default/demo-eks-byo-135-20260207121611-karpenter`: `context deadline exceeded` で install fail

```bash
kubectl -n default get helmrelease.helm.toolkit.fluxcd.io demo-eks-byo-135-20260207121611-karpenter -o yaml
kubectl -n flux-system logs deploy/helm-controller --tail=200
```

## 次にやること (候補)

1) `coredns` FargateProfile を AWS 側でも `ACTIVE` にする
   - ACK の reconcile を促す（annotation 変更など）
   - AWS `list/describe-fargate-profiles` で `coredns` が増えるまで待つ
2) workload で `coredns` が Fargate で Running になることを確認
3) Flux HelmRelease を再試行できる状態に戻す
   - 例: HelmRelease を delete して bootstrapper に再作成させる / remediation 設定を入れる など
4) Karpenter が Running になったら、CRS による `EC2NodeClass/NodePool` apply を確認し、node が起動することを確認

---

## Update: 既存 kind を使って再検証（network 問題の切り分け）

### 8) bootstrapper を最新版に再デプロイ（HelmRelease reliability / SG auto-inject）

- `HelmRelease` の install/upgrade を「いつか収束」に寄せるため、`timeout/disableWait/remediation` を追加
- `vpc-security-group-ids=[]` の場合に ACK `SecurityGroup` を作成し、Topology variable へ注入するように変更

観測:

- `default/*-karpenter` HelmRelease が `Ready=True` になった

### 9) FargateProfile 作成前にできていた Pod を delete して再スケジュール

EKS Fargate は「Pod 作成時点で profile に一致している」必要があるため、profile 作成より前に Pending になっていた Pod は delete して作り直す。

観測:

- `fargate-ip-*` の node が登録され、`coredns` / `karpenter` Pod が node に割り当たった

Update:

- `eks-karpenter-bootstrapper` が FargateProfile `ACTIVE` 後に workload 側で `coredns`/`karpenter` を rollout restart するようにしたため、この手作業は不要になった

### 10) ブロッカー: egress 無しで image pull が timeout

割り当たった後に `ImagePullBackOff` となり、`public.ecr.aws` / ECR への pull が `i/o timeout`。

原因:

- 対象の Subnet に `0.0.0.0/0` route が無く、NAT/VPC endpoints が無い（private subnet としても成立していない）

結論:

- この VPC/Subnet は検証用途として不適（Fargate/Karpenter の前提を満たさない）

### 11) clean slate: EKS + VPC/Subnet を含めて全削除して 1 からやり直す

この開発の中で作った VPC/Subnet を使っていたため、いったん全削除して作り直す。

実施:

- CAPI Cluster を delete
- ACK EKS Cluster を delete
- ACK Subnet/VPC を delete

観測:

- EKS delete 中は Subnet/VPC delete が `DependencyViolation` で recoverable になる
- EKS delete は Fargate profile/ENI の削除待ちで `ResourceInUseException` が出ることがある（時間を置くと解消）

---

## Update: clean slate 後に NAT あり network で 1 から再実行（成功）

- management cluster context: `kind-kany8s-eks`
- 新規 CAPI Cluster: `default/demo-eks-byo-135-20260207222457`
- 新規 ACK EKS Cluster: `default/demo-eks-byo-135-20260207222457-gnx6w`
- 新規 VPC: `vpc-004184b224dfeb561`（CR: `default/demo-eks-byo-135-20260207222457-net-vpc`）
- private subnets:
  - `subnet-0b19159ad71146fda`（CR: `default/demo-eks-byo-135-20260207222457-net-subnet-private-a`）
  - `subnet-0b85b40e5491d7a63`（CR: `default/demo-eks-byo-135-20260207222457-net-subnet-private-b`）

### 12) 旧 VPC/Subnet を全削除（詰まりポイント）

観測:

- ACK `EKS Cluster` 削除中は `ResourceInUseException`（Fargate profiles が DELETING など）で待ちになることがある
- Subnet/VPC 削除は `DependencyViolation` が出やすい（EKS の SG/ENI 等が残っている）

今回やった対処（ログ）:

- 残っていた EKS 由来の SG を手動で削除し、VPC を削除できる状態にした
- AWS 側で VPC を削除済みなのに ACK VPC CR が finalizer で残る場合、最後に finalizer を外して CR を消した
  - NOTE: これは AWS リソースが既に消えているときの “詰まり解除” 用

### 13) NAT ありの検証用 VPC/Subnet を ACK EC2 CR で作成

目的:

- private subnet + NAT を満たし、Fargate 上で `coredns`/`karpenter` が image pull できることを保証する

作成したリソース（management cluster の `default` namespace）:

- `VPC/${CLUSTER_NAME}-net-vpc`
- `InternetGateway/${CLUSTER_NAME}-net-igw`
- `ElasticIPAddress/${CLUSTER_NAME}-net-eip-nat`
- `RouteTable/${CLUSTER_NAME}-net-rtb-public`（0.0.0.0/0 -> IGW）
- `Subnet/${CLUSTER_NAME}-net-subnet-public-a`（NAT 用; routeTableRefs で public RTB に紐付け）
- `NATGateway/${CLUSTER_NAME}-net-natgw`（EIP + public subnet）
- `RouteTable/${CLUSTER_NAME}-net-rtb-private`（0.0.0.0/0 -> NATGW）
- `Subnet/${CLUSTER_NAME}-net-subnet-private-a`（Fargate/Node 用; routeTableRefs で private RTB に紐付け）
- `Subnet/${CLUSTER_NAME}-net-subnet-private-b`（同上）

確認:

- private subnet の route table に `0.0.0.0/0 -> nat-*` があることを確認

### 14) BYO Cluster を apply（subnets=private / SG=[]）

`docs/eks/byo-network/manifests/cluster.yaml.tpl` を render して apply:

- `vpc-subnet-ids` は上で作った private subnet IDs を指定
- `vpc-security-group-ids` は **空（[]）**で apply（node SG 自動作成 + Topology 注入の動作確認）

opt-in:

```bash
kubectl -n default annotate cluster demo-eks-byo-135-20260207222457 eks.kany8s.io/kubeconfig-rotator=enabled --overwrite
kubectl -n default label cluster demo-eks-byo-135-20260207222457 eks.kany8s.io/karpenter=enabled --overwrite
```

観測:

- `vpc-security-group-ids` が bootstrapper により自動注入され、ACK `SecurityGroup` が作成される
  - SG: `securitygroup.ec2.services.k8s.aws/demo-eks-byo-135-20260207222457-karpenter-node-sg` (`sg-018ab8ef07069ce82`)

### 15) management 側の収束

- ACK: OIDC/Role/Policy/AccessEntry/FargateProfile/Flux/CRS/ConfigMap が作成される
- Flux: `HelmRelease/*-karpenter` が `Ready=True`（install succeeded）

注意:

- SG 注入で ACK EKS Cluster が一時的に `UPDATING` になり、その間は FargateProfile の reference resolution が待ちになる
- EKS が `ACTIVE` に戻ると FargateProfile も `Ready=True` になる

### 16) workload 側: Fargate で `coredns` / `karpenter` を Running にする

ポイント:

- Fargate profile 作成より前に生成されて Pending になっていた Pod は、delete して作り直さないと Fargate に載らない

観測:

- `fargate-ip-*` node が登録され、`kube-system/coredns` と `karpenter/*` が `Running`
- NAT により image pull が成功（前回の `i/o timeout` は解消）

### 17) workload 側: NodePool -> node join を確認

テスト用 workload を apply（node を起こすため）:

- `default/karpenter-smoke` Deployment

観測:

- `NodeClaim` が作られ、EC2 node が join
  - `nodeclaim/default-klgbn` / `node/ip-10-37-1-82.ap-northeast-1.compute.internal`
- `default/karpenter-smoke` Pod が EC2 node 上で `Running`

## Cleanup (メモ)

- bootstrapper で作ったリソース一覧:

```bash
CLUSTER_NAME=demo-eks-byo-135-20260207121611
kubectl -n default get \
  openidconnectproviders.iam.services.k8s.aws,roles.iam.services.k8s.aws,policies.iam.services.k8s.aws,\
  accessentries.eks.services.k8s.aws,fargateprofiles.eks.services.k8s.aws,\
  ocirepositories.source.toolkit.fluxcd.io,helmreleases.helm.toolkit.fluxcd.io,\
  configmaps,clusterresourcesets.addons.cluster.x-k8s.io \
  -l cluster.x-k8s.io/cluster-name="$CLUSTER_NAME"
```

- EKS を消す場合は既存の cleanup 手順も参照:
  - `docs/eks/cleanup.md`

---

## Update: 2026-02-08 (clean slate + 自動化確認)

- 新規 CAPI Cluster: `default/demo-eks-byo-135-20260208121023`
- 新規 ACK EKS Cluster: `default/demo-eks-byo-135-20260208121023-ghs8f`
- 新規 VPC: `vpc-04dd0cf7c1350b2ab`（CR: `default/demo-eks-byo-135-20260208121023-net-vpc`）
- private subnets:
  - `subnet-08d53b550da951661`（CR: `default/demo-eks-byo-135-20260208121023-net-subnet-private-a`）
  - `subnet-06ca98e8389786ccd`（CR: `default/demo-eks-byo-135-20260208121023-net-subnet-private-b`）

観測:

- bootstrapper が node SG を自動作成し、Topology の `vpc-security-group-ids` に注入（例: `sg-0776a05159ff8486d`）
- FargateProfiles (`karpenter`/`coredns`) が `ACTIVE` 後、bootstrapper が workload 側で rollout restart を実行
  - `kube-system/coredns` と `karpenter/karpenter` が手作業無しで `Running`
- `default/karpenter-smoke` を apply すると `NodeClaim` が作成され、EC2 node が join して Pod が `Running`

残っている手作業（現状）:

- Reset/cleanup: (旧クラスタ) ACK IAM Role の削除が `DeleteConflict` で詰まることがある（Karpenter が自動生成した InstanceProfile が残っている）
  - 対処例（AWS CLI）:

```bash
ROLE_NAME=<eksClusterName>-node
PROFILE_NAME="$(aws iam list-instance-profiles-for-role --role-name "$ROLE_NAME" --query 'InstanceProfiles[0].InstanceProfileName' --output text)"
aws iam remove-role-from-instance-profile --instance-profile-name "$PROFILE_NAME" --role-name "$ROLE_NAME" || true
aws iam delete-instance-profile --instance-profile-name "$PROFILE_NAME" || true
aws iam delete-role --role-name "$ROLE_NAME" || true
```

- Update: InstanceProfile を ACK 管理に寄せた
  - `eks-karpenter-bootstrapper` が `iam.services.k8s.aws/v1alpha1 InstanceProfile` を作成し、workload の `EC2NodeClass.spec.instanceProfile` で参照する
  - 狙い: Karpenter に instance profile を自動生成させず、cleanup で `DeleteConflict` を踏みにくくする

- kind(management) では IRSA が使えないため、ACK/bootstrappers の `ack-system/aws-creds` Secret は引き続き手作業で用意が必要

次にやること:

1) cleanup e2e: CAPI `Cluster` delete で `InstanceProfile` / node `Role` が AWS CLI 介入無しで消えることを確認
2) dev reset をスクリプト化（EKS + network を一括削除、Recoverable の可視化）

---

## Update: 2026-02-08 (再実行メモ)

### 18) cluster 作成後に "何も動かない/EC2 node が見えない" の確認

ポイント:

- `coredns` / `karpenter` が Fargate で `Ready` でも、追加 workload が無ければ Karpenter は EC2 node を起こさない（Node=0 が正常）

確認（workload kubeconfig）:

```bash
CLUSTER_NAME=demo-eks-byo-135-20260208070010
kube=/tmp/${CLUSTER_NAME}.kubeconfig
kubectl -n default get secret "${CLUSTER_NAME}-kubeconfig" -o jsonpath='{.data.value}' | base64 -d > "$kube"
chmod 600 "$kube"

kubectl --kubeconfig "$kube" get pods -A -o wide
kubectl --kubeconfig "$kube" get nodeclaim -A
kubectl --kubeconfig "$kube" get nodes -o wide
```

### 19) smoke で需要を作り、NodeClaim -> EC2 node join を確認

```bash
kubectl --kubeconfig "$kube" apply -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: karpenter-smoke
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: karpenter-smoke
  template:
    metadata:
      labels:
        app: karpenter-smoke
    spec:
      containers:
      - name: pause
        image: registry.k8s.io/pause:3.10
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
EOF

kubectl --kubeconfig "$kube" get nodeclaim -A -w
kubectl --kubeconfig "$kube" get nodes -w
```

観測:

- `NodeClaim` が作られ、Bottlerocket の EC2 node が join
- `default/karpenter-smoke` Pod が EC2 node 上で `Running`

### 20) 追加: Cluster delete 時の EC2 node cleanup（仕込み）

- `eks-karpenter-bootstrapper` に CAPI `Cluster` の deletion を見て `karpenter.sh/discovery=<eksClusterName>` の EC2 instance を terminate する best-effort cleanup を追加
- 次の宿題: NodeClaim を起こした状態で `Cluster` delete し、EC2 instance が残らないことを確認

---

## Update: 2026-02-08 (Cluster delete 検証)

### 21) CAPI `Cluster` delete -> EC2 node / IAM cleanup の収束

実施:

- CAPI `Cluster` (`demo-eks-byo-135-20260208070010`) を delete

観測:

- bootstrapper が delete 直後に `karpenter.sh/discovery=<eksClusterName>` で EC2 instance terminate を開始（1台は `shutting-down` へ遷移）
- ただし `karpenter-smoke` による需要が残っていると、Karpenter が replacement node を起こしてしまうことがある
  - CAPI `Cluster` オブジェクトが早期に GC されると、bootstrapper 側が追いかけて追加 terminate できない（race）
- 結果として、Karpenter が自動生成した InstanceProfile が残り、ACK の node role delete が `DeleteConflict` で詰まるケースが発生

今回の詰まり解除（AWS CLI; 一時対応）:

```bash
AWS_REGION=ap-northeast-1
EKS_NAME=demo-eks-byo-135-20260208070010-xx6m4

# discovery tag で残っている node instance を terminate
aws ec2 describe-instances --region "$AWS_REGION" \
  --filters Name=tag:karpenter.sh/discovery,Values="$EKS_NAME" Name=instance-state-name,Values=pending,running,stopping,stopped,shutting-down \
  --query 'Reservations[].Instances[].InstanceId' --output text

# node role に紐付く instance profile を削除（role delete の DeleteConflict 回避）
ROLE_NAME="${EKS_NAME}-node"
PROFILE_NAME="$(aws iam list-instance-profiles-for-role --role-name "$ROLE_NAME" --query 'InstanceProfiles[0].InstanceProfileName' --output text)"
aws iam remove-role-from-instance-profile --instance-profile-name "$PROFILE_NAME" --role-name "$ROLE_NAME" || true
aws iam delete-instance-profile --instance-profile-name "$PROFILE_NAME" || true
```

結論:

- 「Cluster delete で EC2 instance が残らない」を保証するには、delete 中の Karpenter の再作成 race を止める（NodePool/EC2NodeClass 削除 or Karpenter uninstall/suspend）+ InstanceProfile 自動削除が必要

---

## Update: 2026-02-08 (cleanup hardening: provisioning stop)

### 22) Cluster delete 時に provisioning を止める（実装）

目的:

- Cluster delete 中に Karpenter が replacement node を起こす race を抑止する

変更:

- `eks-karpenter-bootstrapper` が CAPI `Cluster` deletion を検知したら、EC2 terminate の前に workload 側の provisioning を best-effort で止める
  - `karpenter/karpenter` Deployment を scale `replicas=0`
  - Karpenter CRs を DeleteCollection
    - `nodepools.karpenter.sh/v1`
    - `nodeclaims.karpenter.sh/v1`
    - `ec2nodeclasses.karpenter.k8s.aws/v1`

実装:

- `internal/controller/plugin/eks/karpenter_bootstrapper_controller.go`
- `internal/controller/plugin/eks/karpenter_bootstrapper_workload_provisioning_stop.go`

NOTE:

- remote kubeconfig Secret が既に GC されると workload に触れないため best-effort

### 23) 新規クラスタで動作確認を開始

- CAPI Cluster: `default/demo-eks-byo-135-20260208093206`
- ACK EKS Cluster: `default/demo-eks-byo-135-20260208093206-dxmbm`

現状:

- Flux `HelmRelease/*-karpenter` は `Ready=True`
- FargateProfile は直列制約の影響で `coredns` が `ResourceInUseException`(Recoverable) で待ちになることがある（`karpenter` profile `CREATING` の間は作れない）

次:

- `karpenter-smoke` -> NodeClaim -> EC2 node join を作ったあとに `Cluster` delete して、provisioning stop が race を抑止できるかを確認する

---

## Update: 2026-02-08 (cleanup hardening: kubeconfig GC / region fallback)

### 24) Cluster delete 時の cleanup の確度を上げる（実装）

目的:

- delete 中に kubeconfig Secret が GC されても、workload の provisioning stop を実行できるようにする
- delete 時の region 解決を安定化して、EC2 terminate が skip されにくくする

変更:

- steady-state reconcile で `Cluster.metadata.annotations["eks.kany8s.io/region"]` を補完（削除時の region 解決を安定化）
- delete handler が kubeconfig Secret の annotations から `region` / `eksClusterName` を best-effort で拾う
- workload provisioning stop が kubeconfig Secret 不在でも動くように fallback
  - ACK EKS Cluster の `status.endpoint` / `status.certificateAuthority.data` を使って token kubeconfig を生成
  - aws-iam-authenticator token (`sigs.k8s.io/aws-iam-authenticator`) で kube-apiserver へ接続
- EC2 terminate を 3 回リトライ（replacement の取りこぼし軽減）

実装:

- `internal/controller/plugin/eks/karpenter_bootstrapper_controller.go`
- `internal/controller/plugin/eks/karpenter_bootstrapper_workload_provisioning_stop.go`

### 25) dev reset script（実装; 叩き台）

目的:

- CAPI Cluster delete + (任意) ACK EC2 network delete を 1 コマンドで行い、clean slate を作る

スクリプト:

- `hack/eks-fargate-dev-reset.sh`

実行例:

```bash
CONFIRM=true \
  NAMESPACE=default \
  CLUSTER_NAME=<capi-cluster-name> \
  DELETE_NETWORK=true \
  NETWORK_NAME=<capi-cluster-name>-net \
  bash hack/eks-fargate-dev-reset.sh
```

---

## Update: 2026-02-08 (cleanup e2e: provisioning stop + InstanceProfile)

### 26) `karpenter-smoke` -> NodeClaim -> Cluster delete の収束確認

対象:

- CAPI Cluster: `default/demo-eks-byo-135-20260208093206`
- ACK EKS Cluster: `default/demo-eks-byo-135-20260208093206-dxmbm`

実施:

- workload に `karpenter-smoke` を apply
- `NodeClaim` 作成 + EC2 node join を確認
- CAPI `Cluster` delete

観測:

- workload
  - `NodeClaim/default-d5mw6` が作成され、EC2 node が join
    - instance: `i-0307c3bd91e11ae8d`
    - node: `ip-10-36-1-101.ap-northeast-1.compute.internal`
  - delete 開始後、bootstrapper が provisioning stop を実行
    - `karpenter/karpenter` Deployment が `replicas=0`（replacement 抑止）
    - `NodePool` は削除される
    - `NodeClaim` / `EC2NodeClass` は deletionTimestamp が付き、Karpenter finalizer が残る（Karpenter を止めているため）
      - `nodeclaim` finalizer: `karpenter.sh/termination`
      - `ec2nodeclass` finalizer: `karpenter.k8s.aws/termination`
  - EC2 instance は terminate され、discovery tag で残りが無いことを確認

- AWS / IAM
  - node `Role` / `InstanceProfile` が AWS CLI 介入無しで削除済み（`NoSuchEntity`）
    - role+profile name: `demo-eks-byo-135-20260208093206-dxmbm-node`

- EKS delete
  - ACK EKS Cluster の delete は `ResourceInUseException` (Fargate profile DELETING) の recoverable を挟む
  - その後、AWS 側で `status=DELETING` へ遷移することを確認
