# BYO Network Design for EKS Control Plane

## TL;DR

- このドキュメントは Cluster API の ClusterClass/Topology を前提に、既存 VPC/Subnet(BYO network) 上へ EKS ControlPlane だけ作る標準パターンを定義する。
- 1 source of truth は `Cluster.spec.topology.variables`。同じ入力を patches で `Kany8sCluster` と `Kany8sControlPlane` の双方へ流す。
- `Kany8sControlPlane` は `Kany8sCluster` を読んで値注入/マージしない（Kany8s core に cross-CRD の outputs/merge 概念を増やさない）。
- `Kany8sCluster` は BYO network 用 RGD（AWS リソースを作らない）で `InfrastructureReady` を「入力が揃った」として成立させる。
- 削除事故を防ぐため、BYO network では VPC/Subnet を graph に入れない（ACK EC2 `VPC`/`Subnet` を作らない/owner にしない）。

---

## Status

- Draft (design doc)
- Target: `kro + AWS ACK` による EKS Control Plane

## Background

Kany8s は Cluster API-facing な provider suite として、以下を提供する。

- Infrastructure: `Kany8sCluster` (`infrastructure.cluster.x-k8s.io`)
- ControlPlane: `Kany8sControlPlane` (`controlplane.cluster.x-k8s.io`)

Kany8s は provider-specific な具象化を kro `ResourceGraphDefinition` (RGD) + 下位 controller（例: AWS ACK）へ委譲し、Kany8s controller 自体は provider-agnostic な facade として振る舞う。

既存の EKS smoke (`docs/eks/README.md`) は、`Kany8sControlPlane` が参照する RGD の中で VPC/Subnet も新規作成してから EKS Cluster(Control Plane) を作る。

一方、実運用では「VPC/Subnet は既に存在し、その上に EKS ControlPlane だけ作りたい（BYO network）」ケースが多い。

このとき Cluster API 的には VPC/Subnet はクラスタ全体の共有インフラであり、通常は `Cluster.spec.infrastructureRef` 側がそれを表現する。しかし Kany8s では infra -> control plane への値渡しを CRD 間で一般化しない方針（`docs/adr/0008-infra-outputs-policy-parent-rgd-approach-a.md`）を採っているため、BYO network での責務分担が曖昧になりやすい。

## Goals

- 既存 VPC/Subnet を利用した EKS ControlPlane 作成の推奨パターンを明確化する。
- `Cluster` を使う場合、`InfrastructureReady`/`ControlPlaneAvailable` の意味が破綻しないようにする。
- Kany8s の「thin provider / provider-agnostic」方針を崩さず、ユーザ入力の重複/事故を減らす。
- Cleanup 時に既存 VPC/Subnet を誤って削除しない。

## Non-goals

- 既存 VPC/Subnet の自動検証を完全に行うこと（最初から強い validation を必須にしない）。
- NodeGroup / MachinePool まで含めた完全な EKS provider を Kany8s で実装すること。
- 既存 AWS リソースの adoption/import をこの設計で確定実装すること（必要なら別 issue）。

## Constraints / Related ADRs

- Infra outputs policy: `docs/adr/0008-infra-outputs-policy-parent-rgd-approach-a.md`
  - Kany8s CRD 間での汎用 outputs パッシングは導入しない。
- Normalized status contract: `docs/adr/0002-normalized-rgd-instance-status-contract.md` / `docs/reference/rgd-contract.md`
  - Kany8s は backend instance の `status.ready/endpoint/reason/message/...` だけを読む。
- kro instance lifecycle/spec injection: `docs/adr/0003-kro-instance-lifecycle-and-spec-injection.md`
  - `Kany8sCluster` は instance spec に `clusterName/clusterNamespace/(opt)clusterUID` を注入する。
  - `Kany8sControlPlane` は instance spec に `version` を注入・上書きする。
- kro authoring pitfalls: `docs/reference/rgd-guidelines.md`
  - kro v0.7.1 では `spec.schema.status` から `schema.*` を参照できない等の制約がある。

## Recommended Design (ClusterClass/Topology only)

この設計は ClusterClass/Topology を前提にする。

- BYO network の入力は `Cluster.spec.topology.variables` に集約する。
- Topology patches で同一入力を `Kany8sCluster` と `Kany8sControlPlane` の両方に流す。
- `Kany8sControlPlane` が `Kany8sCluster` から値を読む/注入する経路は作らない（thin provider 方針、`docs/adr/0008-*` と整合）。

このドキュメントは “manifest 直書き（Topology 不使用）” を扱わない。

### Concrete manifests

- (任意) ネットワーク bootstrap (VPC/Subnet を ACK で新規作成): `docs/eks/byo-network/manifests/bootstrap-network.yaml.tpl`
- BYO infra input-gate RGD: `docs/eks/byo-network/manifests/aws-byo-network-rgd.yaml`
- BYO control plane RGD: `docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml`
- ClusterClass + templates: `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml`
- Topology Cluster template: `docs/eks/byo-network/manifests/cluster.yaml.tpl`

### Network bootstrap (optional; first run)

この設計は「既存 subnet IDs を受け取る」BYO を前提にする。
ただし初回でネットワークが無い場合は、AWS CLI/Console を使わずに ACK(EC2) で VPC/Subnet を作り、作成後に subnet IDs を取り出して BYO の variables に渡せる。

- 手順: `docs/eks/byo-network/README.md` の "ネットワークが無い場合（bootstrap; AWS CLI/Console 不要）" を参照
- 注意: bootstrap で作った VPC/Subnet は EKS Cluster の削除とは独立（BYO の shared infra）。削除したい場合は別途 `kubectl delete` する。

### Responsibilities

Infrastructure (`Kany8sCluster`):

- BYO network の “入力が揃った” を `status.ready` として表現する。
- BYO network の場合、AWS VPC/Subnet を作成・更新・削除しない。
- `Cluster.InfrastructureReady` が単なる stub にならないように、kro mode の RGD を使って readiness を作る。

ControlPlane (`Kany8sControlPlane`):

- EKS 作成に必要な subnetIDs 等は `spec.kroSpec` に渡す。
- EKS RGD は VPC/Subnet を作らず、ID を直接参照する（BYO）。

Data sharing:

- 値の一元化は ClusterClass/Topology の variables/patch で実現する。

### Why this works with Kany8s principles

- Kany8s controller は provider-specific な形（VPC ID 等）を理解せず、RGD instance の normalized status だけを読む。
- `Kany8sControlPlane` が `Cluster.infrastructureRef` を解決して `kroSpec` を注入するような cross-CRD 値渡しはしない。
- “共有入力” は ClusterClass/Topology 機能に寄せ、Kany8s core の責務増を避ける。

## Readiness Semantics

### InfrastructureReady (`Cluster.status.infrastructureReady`)

`Kany8sCluster.status.initialization.provisioned` が `true` になる条件:

- `Kany8sCluster` が kro mode で、参照先 RGD instance が `status.ready=true` を返す。
- BYO network RGD の `status.ready` は「入力が揃った（最低限の条件を満たす）」を表す。

重要:

- これは “AWS 上のネットワークが本当に有効” を保証しない。
- 実際の有効性（subnet が別 AZ か/route があるか/権限があるか等）は、EKS 作成が失敗することで露見し得る（Non-goal の範囲）。

### ControlPlaneAvailable / ControlPlaneInitialized

`Kany8sControlPlane` の Ready は backend status contract に従う（`docs/reference/rgd-contract.md`）。

- `status.ready=true` かつ `status.endpoint` が非空のとき ControlPlaneReady とみなす。
- `status.kubeconfigSecretRef` がある場合は kubeconfig Secret の reconcile 完了も待つ。

## Deletion Safety (BYO Network)

BYO network で既存 VPC/Subnet を誤って削除しないためのルール:

- BYO network 用 RGD は VPC/Subnet を表す外部リソース（ACK EC2 `VPC`/`Subnet` 等）を graph に含めない。
  - 既存リソースを「管理対象」に入れると、削除時に下位 controller が削除を試みる可能性がある。
- BYO network RGD は readiness 判定のための “ダミー” リソース（ConfigMap 等）のみを作る。

EKS control plane 側（IAM Role / EKS Cluster）はクラスタ固有リソースのため、削除で消える前提とする。

## Recommended RGD: BYO Network Input Gate

`Kany8sCluster` を kro mode にし、BYO network の入力が揃うまで `status.ready=false` を返す “input-gate” RGD を使う。

### RGD name / kind

- RGD: `aws-byo-network.kro.run`
- Instance kind: `AWSBYONetwork`

### Status contract (Infrastructure)

`Kany8sCluster` 向け RGD instance は最低限 `status.ready` を提供する（`docs/reference/rgd-contract.md`）。

推奨:

- `status.reason`, `status.message` も提供し、Ready condition に反映されるようにする。

### kro v0.7.1 制約への対応

`spec.schema.status` から `schema.*` が参照できないため、テンプレ側で `schema.spec` を使って readiness を計算し、それを resource id 変数経由で status に持ち上げる。

実装済みマニフェスト:

- `docs/eks/byo-network/manifests/aws-byo-network-rgd.yaml`
  - ConfigMap のみを作成（AWS リソースなし）
  - `status.ready`/`status.reason`/`status.message` を提供
  - `subnetIDs >= 2` を最小入力条件として判定

Notes:

- `>= 2` は EKS の要件（異なる AZ の subnet を 2 つ以上）に寄せた最低限のチェック。
- より強い検証（AZ 分散、subnet の所属 VPC 一致、tag、route/NAT 等）は Non-goal。
- `status.ready` の field materialization を安定させるため、`int(ternary)` パターンを使う（`docs/reference/rgd-guidelines.md`）。

## ControlPlane RGD (BYO Network)

BYO network では ControlPlane RGD は VPC/Subnet を作らず、subnetIDs 等を spec に直接渡す。

この設計では、BYO 向けに以下の RGD を標準とする（※既存の smoke RGD と kind が衝突しないよう schema.kind を分ける）。

- RGD: `eks-control-plane-byo.kro.run`
- Instance kind: `EKSControlPlaneBYO`

要求する入力（`spec`）:

- `version` (Kany8sControlPlane が注入/上書き)
- `region` (ACK の region annotation 用)
- `vpc.subnetIDs` (必須、2 つ以上推奨)
- `vpc.securityGroupIDs` (任意)
- `publicAccessCIDRs` (必須: 明示させる。"0.0.0.0/0" でもよいが意図的に指定する)

出力（`status`）:

- `status.ready` / `status.endpoint`（`docs/reference/rgd-contract.md`）
- `status.reason` / `status.message`（推奨）

実装済みマニフェスト:

- `docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml`
  - ACK IAM Role + ACK EKS Cluster を作成（VPC/Subnet は作成しない）
  - `status.ready`/`status.endpoint`/`status.reason`/`status.message` を提供
  - ACK EKS `Cluster.spec` は BYO 用に `resourcesVPCConfig.subnetIDs` を使う
  - IAM 連携は `roleRef` + region annotation (`services.k8s.aws/region`) を使う

## ClusterClass/Topology: Reference Manifests

この設計の “入力の一元化” は ClusterClass/Topology で完結させる。

### Variable contract (1 source of truth)

- `region` (required)
- `eks-version` (required; EKS expects major.minor like 1.35)
- `vpc-subnet-ids` (required, minItems=2)
- `vpc-security-group-ids` (required; empty list is allowed)
- `eks-public-access-cidrs` (required; 明示が必須)

### Patches

- `vpc.*` は infra (`Kany8sCluster`) と control plane (`Kany8sControlPlane`) の双方に流す
- `region` / `eks.publicAccessCIDRs` は control plane 側に流す

### End-to-end manifests

- `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml`
  - `Kany8sClusterTemplate` / `Kany8sControlPlaneTemplate` / `ClusterClass` を同梱
  - template の namespace は固定していないため、利用時は対象 `Cluster` と同じ namespace に apply する
  - JSON patch を安全にするため、テンプレートで `kroSpec.vpc` を事前作成
- `docs/eks/byo-network/manifests/cluster.yaml.tpl`
  - Topology-only の `Cluster` テンプレート
- `region` / `vpc.subnetIDs` / `vpc.securityGroupIDs` / `eks.publicAccessCIDRs` を variables で指定
- `region` / `vpc-subnet-ids` / `vpc-security-group-ids` / `eks-public-access-cidrs` を variables で指定

## Operational Notes

### Common failure modes

- `InfrastructureReady` がすぐ True になる（stub mode）
  - `Kany8sCluster.spec.resourceGraphDefinitionRef` が未設定。
  - BYO network では kro mode + input-gate RGD を推奨。

- EKS 作成が失敗する
  - subnetIDs が 2 つ未満 / 同一 AZ / region 違い / IAM 権限不足 / ACK controller の region 設定不一致 等。
  - BYO network の設計では強い事前検証はしないため、EKS backend の失敗で気付く可能性がある。

### Guardrails

- BYO network では VPC/Subnet を管理しない RGD と、VPC/Subnet を作る smoke/dev RGD を明確に分ける。
  - 例: `*-smoke.kro.run` と `*-byo.kro.run` などの命名。

## Rejected ideas (kept for context)

### Option 1: ネットワークを ControlPlane RGD 側に寄せる + `Kany8sCluster` は stub

棄却理由:

- `InfrastructureReady` が “ネットワーク入力の確定” を表せず、CAPI の flow が読みづらい（stub の Ready が常に True になり得る）。
- worker/MachinePool などクラスタ共有インフラを参照する将来拡張で責務が破綻しやすい。

### Option 3: `Kany8sControlPlane` が owner `Cluster` の `infrastructureRef` を読んで注入

棄却理由:

- `Kany8sControlPlane` が別 CR（`Kany8sCluster`）を読んで spec を合成する設計は、Kany8s core に “汎用の値渡し/マージ” の責務を持ち込みやすい（`docs/adr/0008-*` の方針と衝突）。
- マージ仕様（優先順位・部分更新・意図せぬ上書き）を定義しないと事故りやすい。
- provider ごとの必須フィールド差を Kany8s core が吸収する圧力が高まり、thin provider の境界が崩れやすい。

### Parent RGD (infra + control plane) で graph 内に閉じる

棄却（このユースケースでは採用しない）理由:

- BYO network は outputs が不要であり、graph 内 value passing を設計に入れる必要がない。
- network を control plane graph に取り込むと、削除・所有権の境界が曖昧になりやすい（BYO の削除事故リスク）。
- 共有インフラは `Cluster.spec.infrastructureRef` で表す、という CAPI の直感から外れやすい。

## Future Work

- BYO network input-gate RGD の “弱い validation” の段階導入（文字列 prefix チェック等）
- EKS BYO 用 RGD の整備（region 明示、private endpoint、kubeconfigSecretRef 等）
- Adoption/import を扱う別設計（retain/deletion policy、break-glass 手順）
