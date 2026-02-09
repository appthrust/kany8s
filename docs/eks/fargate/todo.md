# TODO: EKS Fargate bootstrap + Karpenter (Flux HelmRelease)

- 作成日: 2026-02-07
- 更新日: 2026-02-09
- ステータス: MVP Done + cleanup verified (optional items remain)

この TODO は `docs/eks/fargate/design.md` の設計を実装に落とすためのチェックリストです。

このファイルの見方:

- これは「実装/運用で必要な論点の TODO」です。実際に叩いた手順/観測ログは `docs/eks/fargate/wip.md`、実行手順は `docs/eks/fargate/README.md` に寄せます。
- 各セクションは概ね依存順（前提 -> BYO拡張 -> Flux導入 -> bootstrapper 実装 -> テスト/自動化）です。
- `[x]` は実装/検証済み、`[ ]` は未対応/未決定です。`(Optional)` は MVP の範囲外ですが将来必要になり得る項目です。

用語:

- management cluster: kind（例: `kind-kany8s-eks`）。ACK/Flux/bootstrapper が動く場所。
- workload cluster: EKS。Fargate と Karpenter が動く場所。
- CAPI Cluster: `cluster.x-k8s.io/* Cluster`（topology variables で EKS 設定を渡す）。
- ACK EKS Cluster: `eks.services.k8s.aws/* Cluster`（EKS の endpoint/OIDC 等を status で読む）。

## Post-MVP backlog

- [ ] Flux components のバージョンを pin する（`install.yaml` を `releases/download/<tag>/...` に固定）
- [ ] envtest を追加（ACK EKS Cluster unstructured -> 派生リソース作成、readiness gate で requeue されることを確認）
- [ ] (Cleanup) delete 中の “workload 側 finalizer 残り” の扱いを決める
  - [ ] (Option B) Flux `HelmRelease` を suspend/uninstall して Karpenter 自体も止める
- [ ] 運用/アップグレード手順（EKS/Karpenter/Flux）を docs 化
- [ ] プロダクション向け NodePool/EC2NodeClass の方針（capacity/requirements/disruption 等）を整理して docs 化
- [ ] (Optional) interruption handling (SQS/EventBridge + `settings.interruptionQueue`)

## Verified (2026-02-08)

- [x] cleanup e2e: CAPI `Cluster` delete で ACK `InstanceProfile` / node `Role` が AWS CLI 介入なしで収束することを確認
- [x] cleanup e2e: CAPI `Cluster` delete で Karpenter が起こした EC2 instance が残らず terminate されることを確認
- [x] (Cleanup hardening) delete 時に workload の provisioning を止めて replacement race を抑止（scale `karpenter` -> 0 / NodePool+NodeClaim+EC2NodeClass の DeleteCollection; kubeconfig Secret GC fallback あり）
- [x] dev reset をスクリプト化（叩き台）: `hack/eks-fargate-dev-reset.sh`

## 0) 設計の前提を固定する（最初にやる）

ここは「何を前提に実装/運用するか」を固定するセクションです。
バージョンや join 方式などが揺れると、後続（IAM/Flux/CRS/cleanup）の設計が連鎖して揺れます。

- [x] 対象の BYO ClusterClass を決める
  - [x] 既存 `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml` を拡張する
  - [ ] もしくは Karpenter/Fargate 専用に `kany8s-eks-byo-fargate` のような別 ClusterClass を追加する
- [ ] 既定のバージョンを固定する
  - [x] EKS (default: 1.35; `docs/eks/values.md` / BYO templates)
  - [x] Karpenter (default: 1.0.8; `internal/controller/plugin/eks/karpenter_bootstrapper_controller.go` の `defaultKarpenterChartVersion`)
  - [ ] Flux (source-controller/helm-controller のバージョン)
- [x] node join の方式を固定する（推奨: AccessEntry）
  - [x] `eks.services.k8s.aws/v1alpha1 AccessEntry` を使う (type=EC2_LINUX)
  - [ ] (fallback) workload 側に `kube-system/aws-auth` を apply する
- [x] Karpenter の install 方法を固定する（この TODO の主経路）
  - [x] Flux の `HelmRelease.spec.kubeConfig.secretRef` を使って remote install
- [ ] Karpenter interruption handling をやるか決める
  - [ ] (Optional) SQS + EventBridge + `settings.interruptionQueue`

## 1) 既存 BYO RGD/ClusterClass を worker 対応にする

ここは「既存の BYO(ControlPlane) を、worker(=Karpenter node) が join できる形に拡張する」セクションです。
主に AccessEntry と endpoint 設定、そして subnet/SG の入力を topology variables として扱えるようにします。

### 1.1 `eks-control-plane-byo.kro.run` を更新（AccessEntry/Private endpoint 対応）

- [x] `docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml` に入力を追加
  - [x] `spec.access.authenticationMode`（default: `API_AND_CONFIG_MAP`）
  - [x] `spec.vpc.endpointPrivateAccess`（default: `true`）
  - [x] `spec.vpc.endpointPublicAccess`（default: `true`）
- [x] ACK EKS Cluster に反映
  - [x] `spec.accessConfig.authenticationMode`
  - [x] `spec.resourcesVPCConfig.endpointPrivateAccess`
  - [x] `spec.resourcesVPCConfig.endpointPublicAccess`
- [x] 既存クラスター（CONFIG_MAP / privateAccess=false）に対して update が収束することを確認（`docs/eks/fargate/wip.md` で観測）

### 1.2 ClusterClass variables/patches を更新

- [x] `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml` を更新
  - [x] variables 追加（例）
    - [x] `eks-access-mode`（default: `API_AND_CONFIG_MAP`）
    - [x] `eks-endpoint-private-access`（default: true）
    - [x] `eks-endpoint-public-access`（default: true）
  - [x] control plane template の patches を追加
    - [x] `/spec/template/spec/kroSpec/access/authenticationMode`
    - [x] `/spec/template/spec/kroSpec/vpc/endpointPrivateAccess`
    - [x] `/spec/template/spec/kroSpec/vpc/endpointPublicAccess`
- [x] `docs/eks/byo-network/manifests/cluster.yaml.tpl` の変数注入を更新

### 1.3 入力の見直し（subnet/SG）

- [x] FargateProfile の subnet 要件（private subnet）をドキュメント化/検証（`docs/eks/fargate/README.md` / `docs/eks/fargate/design.md` / `docs/eks/values.md`）
  - [x] `docs/eks/fargate/design.md` の前提を `docs/eks/values.md` に反映
- [x] Karpenter node 用 security group IDs の扱いを決める
  - [x] `vpc-security-group-ids` を “node 用 SG IDs” として扱う
  - [x] (No command ops) `vpc-security-group-ids` が空なら ACK `SecurityGroup` を自動作成し、Topology variable へ注入して収束させる
  - [x] VPC ID の解決方針を決める（入力 / read-only discovery / bootstrap network）
    - [x] (採用) bootstrapper が AWS API(DescribeSubnets/DescribeVpcs) を read-only で呼び、subnet IDs -> VPC ID/CIDR を解決
  - [ ] (Optional) `karpenter-node-security-group-ids` を別変数に分離
  - [x] (Optional) SG rule の最小セットを固定（ingress: VPC CIDRs all / egress: 0.0.0.0/0 all）
  - [x] (MVP) `vpc-security-group-ids` が空なら node SG を作成し、`status.id` 取得後に Topology variable へ注入する（準備中は requeue）

## 2) Flux を management cluster(kind) に導入する

ここは「remote HelmRelease を使って、workload(EKS)に Karpenter をインストールする」ための前提（Flux components）を準備するセクションです。

### 2.1 Flux components の導入方法を決める

- [x] install 手順を `docs/eks/fargate/todo.md` から `docs/eks/fargate/README.md` に切り出す（MVP）
- [x] Flux のインストール方式を決める
  - [x] `flux install` を前提にする
  - [ ] もしくは `gotk-components.yaml` を vendoring して `kubectl apply` する

### 2.2 最低限必要な CRDs/Controllers を確認

- [x] `source.toolkit.fluxcd.io/*`（OCIRepository など）が使える
- [x] `helm.toolkit.fluxcd.io/*`（HelmRelease）が使える
- [x] `HelmRelease.spec.kubeConfig.secretRef` が使える（remote Helm）

## 3) 実装: `eks-karpenter-bootstrapper`（management cluster で完結するブートストラップ）

ここが本体です。bootstrapper は management で動き、CAPI Cluster を入口にして ACK/IAM/EKS/Flux/CRS を作ります。
狙いは「Node=0 でも `coredns`/`karpenter` を Fargate で起動し、需要が出たら EC2 node が join する」を自動で収束させることです。

### 3.1 置き場所/配布形態

- [x] 新規 binary を追加
  - [x] `cmd/eks-karpenter-bootstrapper/main.go`
  - [x] `internal/controller/plugin/eks/karpenter_bootstrapper_controller.go`
- [x] Kustomize manifests を追加
  - [x] `config/eks-karpenter-bootstrapper/`
- [x] container build 用 Dockerfile を追加
  - [x] `Dockerfile.eks-karpenter-bootstrapper`
- [x] image build/deploy の導線を Makefile に追加
  - [x] `make docker-build-eks-karpenter-bootstrapper`
  - [x] `make deploy-eks-karpenter-bootstrapper`
  - [x] `make undeploy-eks-karpenter-bootstrapper`

### 3.2 Watch 対象と opt-in

この項目は「controller-runtime の reconcile をいつ走らせるか」と「誤爆を避けるための有効化条件」を決める TODO です。

- watch 対象: どのリソースの変化で再 reconcile するか。
  - EKS の endpoint/OIDC issuer などは ACK EKS Cluster の status に出てくるため、CAPI Cluster だけ watch していると status 変化を拾えません。
  - そのため、最低限 `CAPI Cluster` と `ACK EKS Cluster` を watch し、ACK 側の status 更新で requeue するようにします。
- opt-in: デフォルトで全 Cluster に副作用を出さないためのスイッチ。
  - `Cluster.metadata.labels["eks.kany8s.io/karpenter"]=enabled` のようなラベルで明示的に有効化します。
  - これにより「既存の BYO/EKS を壊したくない環境」で安全に導入できます。

- [x] opt-in ラベルを実装
  - [x] `Cluster.metadata.labels["eks.kany8s.io/karpenter"] == "enabled"`
- [x] watch 対象（MVP: Cluster + ACK EKS Cluster。Optional watcher は保留）
  - [x] `cluster.x-k8s.io/v1beta2 Cluster`
  - [x] `eks.services.k8s.aws/v1alpha1 Cluster`（ACK EKS Cluster; endpoint/oidc issuer を読む）
  - [ ] `eks.services.k8s.aws/v1alpha1 FargateProfile` (Optional)
  - [ ] `eks.services.k8s.aws/v1alpha1 AccessEntry` (Optional)
  - [ ] `iam.services.k8s.aws/v1alpha1 OpenIDConnectProvider` (Optional)
  - [ ] `iam.services.k8s.aws/v1alpha1 Role` (Optional)
  - [ ] `iam.services.k8s.aws/v1alpha1 Policy` (Optional)
  - [ ] (Flux) `source.toolkit.fluxcd.io/v1 OCIRepository` (Optional)
  - [ ] (Flux) `helm.toolkit.fluxcd.io/v2 HelmRelease` (Optional)

### 3.3 Name/region の解決

- [x] `eks-kubeconfig-rotator` と同じ name/region 解決ロジックを流用
  - [x] `eksClusterName`（Topology の name ズレ吸収）
  - [x] `ackClusterName`
  - [x] `awsRegion`（annotation/status/region annotation の優先順位）
  - [ ] (Optional) 共通 helper に切り出す

### 3.4 ACK EKS Cluster の readiness gate

- [x] ACK EKS Cluster の status が揃うまで待つ
  - [x] `status.endpoint` が非空になるまで待つ
  - [x] `status.identity.oidc.issuer` が取れるまで待つ
  - [x] `status.ackResourceMetadata.ownerAccountID` が取れるまで待つ（IAM ARN 組み立て用）
  - [x] (Optional) `status.status == ACTIVE` を明示 gate に追加

### 3.5 IRSA 用 OIDC provider

- [x] `OpenIDConnectProvider` を CreateOrUpdate
  - [x] `.spec.url` を `oidc.issuer` に合わせる
  - [x] `thumbprints` の扱いを決める
    - [x] (MVP) 空のまま（AWS 側に取得させる）
    - [ ] (Optional) TLS で自動取得して設定

### 3.6 IAM: Karpenter controller policy/role

- [x] controller policy のソースを固定
  - [x] 参考: `terraform-aws-modules/terraform-aws-eks` v20.29 `modules/karpenter/policy.tf` (v1)
- [x] policy JSON のテンプレート化（`region/accountId/clusterName` を埋める）
- [x] `iam.services.k8s.aws/v1alpha1 Policy` を作成し、`Role.spec.policyRefs` で参照
- [x] `Role.assumeRolePolicyDocument` を OIDC trust にする
  - [x] `system:serviceaccount:karpenter:karpenter`
- [x] 期待する出力（controller role ARN）を後段（HelmRelease values）に注入

### 3.7 IAM: Karpenter node role

- [x] `Role`（trust: ec2.amazonaws.com）を作成
- [x] managed policy を attach
  - [x] `AmazonEKSWorkerNodePolicy`
  - [x] `AmazonEKS_CNI_Policy`
  - [x] `AmazonEC2ContainerRegistryReadOnly`
- [ ] (Optional) `AmazonSSMManagedInstanceCore`

### 3.7.1 IAM: Karpenter node InstanceProfile (cleanup 安定化)

- [x] `iam.services.k8s.aws/v1alpha1 InstanceProfile` を CreateOrUpdate
  - [x] `spec.name: <instance profile name>`（例: `<eksClusterName>-node` を短縮して収める）
  - [x] `spec.roleRef.from.name = <node role CR name>`
- [x] `EC2NodeClass.spec.instanceProfile` で上記 name を参照し、Karpenter に instance profile を自動生成させない

### 3.8 IAM: Fargate Pod execution role

- [x] `Role`（trust: eks-fargate-pods.amazonaws.com）を作成
- [x] managed policy `AmazonEKSFargatePodExecutionRolePolicy` を attach

### 3.9 EKS: AccessEntry（node join）

- [x] `eks.services.k8s.aws/v1alpha1 AccessEntry` を CreateOrUpdate
  - [x] `clusterRef.from.name = <ackClusterName>`
  - [x] `principalARN: <node role arn>`
  - [x] `type: EC2_LINUX`
- [x] EKS control plane 側が `authenticationMode=API_AND_CONFIG_MAP` であることを前提にする
  - [x] そうでない場合はイベントで明確に理由を出して待つ

### 3.10 EKS: FargateProfile（bootstrap compute）

- [x] `FargateProfile/karpenter`
  - [x] `selectors: [{namespace: karpenter}]`
- [x] `FargateProfile/coredns`
  - [x] `selectors: [{namespace: kube-system, labels: {k8s-app: kube-dns}}]`
- [x] `subnets` は `vpc-subnet-ids` をそのまま使う
- [x] `podExecutionRoleRef` を付与

### 3.11 Flux: OCIRepository + HelmRelease（remote Karpenter install）

- [x] flux CRD が無い場合は “待つ/無効化” する（requeue してエラーにしない）
- [x] per-cluster namespace（= Cluster と同じ namespace）に以下を生成
  - [x] `OCIRepository/karpenter`
  - [x] `HelmRelease/karpenter`
- [x] `HelmRelease.spec.kubeConfig.secretRef.name = <capiClusterName>-kubeconfig`
  - [x] key は既定（`data.value`）を前提
- [x] `HelmRelease.spec.values` に必要値を注入
  - [x] `settings.clusterName`（= eksClusterName）
  - [x] `settings.clusterEndpoint`（= ACK EKS Cluster.status.endpoint）
  - [x] `serviceAccount.annotations["eks.amazonaws.com/role-arn"]`（controller role arn）
  - [x] `webhook.enabled=false`
- [x] CRD lifecycle（`install.crds`/`upgrade.crds`）を設定する
- [x] reconcile 間隔を token TTL より十分短くする（例: 5m）

### 3.12 NodePool/EC2NodeClass の適用（MVP）

- [x] 適用方式を決める
  - [ ] (A) plain YAML を bootstrapper が workload に apply（kubectl 相当）
  - [ ] (B) Flux で 2nd HelmRelease として配布（dependsOn で karpenter に依存）
  - [x] (C) ClusterResourceSet を併用
- [x] ClusterResourceSet selector 用に `Cluster` へ `cluster.x-k8s.io/cluster-name=<Cluster.name>` label を付与する
- [x] Karpenter v1 `EC2NodeClass` は `subnetSelectorTerms[].id` / `securityGroupSelectorTerms[].id` が使えることを前提に manifest を作る
  - [ ] CRD schema 確認: https://raw.githubusercontent.com/aws/karpenter-provider-aws/v1.0.8/pkg/apis/crds/karpenter.k8s.aws_ec2nodeclasses.yaml
  - [x] `EC2NodeClass.spec.role` ではなく `EC2NodeClass.spec.instanceProfile` を使う（InstanceProfile は ACK で管理）

## 4) RBAC / credentials / deploy

ここは「management 側で動く bootstrapper が必要な権限/認証を持つ」ことを確認するセクションです。
kind では IRSA が使えないため、ACK と同様に `aws-creds` を Secret として与える前提があります。

- [x] RBAC を追加（bootstrapper）
  - [x] read/patch: `clusters.cluster.x-k8s.io`
  - [x] read/write: `iam.services.k8s.aws/*`
  - [x] read/write: `eks.services.k8s.aws/*`
  - [x] read/write: Flux CRDs（OCIRepository/HelmRelease）
- [x] kind 環境では IRSA を使えないので、ACK と同じ `aws-creds` を volume mount する
  - [x] Secret の namespace/名前を統一し、docs と manifests を揃える
- [x] ログ/イベントの最小セット
  - [x] OIDC issuer 未確定
  - [x] AccessEntry を作れない（authenticationMode が違う等）
  - [x] FargateProfile が作れない（subnet が private ではない等）
  - [x] Flux CRD 未導入

## 5) テスト

ここは「ユニットで壊れやすい部分（テンプレート/生成 spec）を固め、手動 e2e で実クラスタでの収束を確認する」セクションです。

### 5.1 unit

- [x] name 解決（Topology name ズレケース）
- [x] IAM policy JSON テンプレート生成（region/accountId/clusterName）
- [x] 生成する ACK リソース（OIDC/Role/Policy/FargateProfile/AccessEntry）の “期待 spec”
- [x] Flux リソース（OCIRepository/HelmRelease）の “期待 spec”

### 5.2 envtest（可能なら）

- [ ] Cluster + unstructured ACK EKS Cluster を与えて、派生リソースが作られる
- [ ] `ACTIVE` 前/issuer 前は requeue される

### 5.3 manual e2e（kind + AWS）

- [x] `docs/eks/README.md` に従って management cluster をセットアップ
- [x] Flux を導入
- [x] `eks-kubeconfig-rotator` を導入して `<cluster>-kubeconfig` を安定化
- [x] BYO cluster を作成
- [x] opt-in label を付与
- [x] 期待結果
  - [x] ACK: AccessEntry/FargateProfile が作られる
  - [x] workload: `kube-system/coredns` が Running（Fargate）
  - [x] workload: `karpenter` が Running（Fargate）
  - [x] workload: NodePool を作ると node が増える
- [x] cleanup e2e（InstanceProfile; no AWS CLI）
  - [x] CAPI `Cluster` delete で ACK `InstanceProfile` / node `Role` が `DeleteConflict` 等で詰まらずに削除される
  - [x] CAPI `Cluster` delete で `karpenter.sh/discovery=<eksClusterName>` の EC2 instance が残らない

## 6) docs 更新

ここは「再現手順/前提/cleanup」を docs に反映するセクションです。TODO の消化状況に合わせて README/values/cleanup を更新します。

- [x] `docs/eks/fargate/README.md` を追加（実行手順）
  - [x] prerequisites（private subnet/NAT, endpointPrivateAccess, AccessEntry）
  - [x] Flux 導入
  - [x] plugin デプロイ
  - [x] opt-in
  - [x] 検証コマンド
- [x] `docs/eks/values.md` に fargate/karpenter 用の追加値（SG IDs など）を追記
- [x] `docs/eks/cleanup.md` に fargate/karpenter リソースの削除観点を追記

## 7) 追加の自動化（手作業を減らす）

ここは「MVP では手でやっていた作業（ネットワーク準備/Pod作り直し/削除時の詰まり解除など）を自動化する」セクションです。

- [x] (Network) 検証用の NAT あり private subnet を “YAML apply” で用意できるようにする
  - [ ] `docs/eks/byo-network/` の network RGD を拡張（IGW/EIP/NATGW/RouteTable + Subnet.routeTableRefs）
  - [x] NAT あり専用サンプルを追加: `docs/eks/byo-network/manifests/bootstrap-network-private-nat.yaml.tpl`
- [x] (Reset) dev reset をスクリプト化（叩き台）: `hack/eks-fargate-dev-reset.sh`
  - [ ] ACK の削除待ち/Recoverable の見える化（DependencyViolation/ResourceInUseException）
  - [ ] (IAM) Role delete が InstanceProfile で詰まるケースを自動処理する
    - [x] (preferred) Karpenter node InstanceProfile を ACK 管理下に置き、削除時に収束させる（`EC2NodeClass.spec.instanceProfile`）
    - [ ] (fallback) 旧クラスタ向けに `DeleteConflict: Cannot delete entity, must remove roles from instance profile first.` を検知して InstanceProfile を先に削除
  - [x] (verify) InstanceProfile 採用後に cleanup が収束することを e2e で確認する
  - [ ] finalizer を外す手順は “明示的な最後の手段” として docs に切り出す
- [x] (Cleanup) CAPI `Cluster` delete 時に Karpenter EC2 instance を best-effort で terminate する
  - [x] 実装: `internal/controller/plugin/eks/karpenter_bootstrapper_controller.go`（tag: `karpenter.sh/discovery=<eksClusterName>`）
  - [x] 検証: NodeClaim を作ったあとに Cluster delete して instance が残らない（provisioning stop あり）
- [x] (Fargate) FargateProfile 作成前に Pending になった Pod の作り直しを自動化する
  - [x] `eks-karpenter-bootstrapper` が FargateProfile `ACTIVE` 後に remote kubeconfig 経由で `coredns` / `karpenter` を rollout restart
  - [x] これにより `kubectl delete pod` の手作業を不要にする

## DoD

- [x] BYO クラスタで NodeGroup なしの状態から、Fargate + Karpenter で node が自動起動する
  - [x] 観測: `default/karpenter-smoke` -> `NodeClaim` -> EC2 node join -> Pod `Running`
- [x] Karpenter install は Flux HelmRelease (remote kubeconfig) で再現可能
  - [x] 観測: `HelmRelease/*-karpenter` が `Ready=True`
- [x] node join は AccessEntry で完結（aws-auth 依存が無い）
  - [x] 観測: `AccessEntry(type=EC2_LINUX)` 作成のみで node join
- [x] 既存 VPC/Subnet を削除しない（BYO deletion safety 維持）
  - [x] 観測: CAPI `Cluster` delete では network(ACK EC2) は消えず、別途 delete が必要
