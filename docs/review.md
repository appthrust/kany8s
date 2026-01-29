# CRD / Domain Model Review (2026-01-28)

このドキュメントは `docs/idea.md` / `docs/design.md` と既存実装（CRD/コントローラ/例/テスト）を突き合わせ、現在の CRD モデルとドメイン境界が妥当かを点検した結果をまとめたものです。

前提: Kany8s は ControlPlane だけでなく Infrastructure 側も provider suite として提供する方針。

---

## 1. 見たもの（根拠となる箇所）

- 設計/ドキュメント
  - `docs/design.md`
  - `docs/idea.md`
  - `docs/PRD.md`
  - `docs/rgd-contract.md`
  - `docs/rgd-guidelines.md`
  - `docs/kro.md`

- API/CRD
  - `api/v1alpha1/kany8scontrolplane_types.go`
  - `api/v1alpha1/kany8scontrolplanetemplate_types.go`
  - `api/v1alpha1/kany8sclustertemplate_types.go`
  - `api/infrastructure/v1alpha1/kany8scluster_types.go`
  - `config/crd/bases/controlplane.cluster.x-k8s.io_kany8scontrolplanes.yaml`
  - `config/crd/bases/controlplane.cluster.x-k8s.io_kany8scontrolplanetemplates.yaml`
  - `config/crd/bases/controlplane.cluster.x-k8s.io_kany8sclustertemplates.yaml`
  - `config/crd/bases/infrastructure.cluster.x-k8s.io_kany8sclusters.yaml`

- コントローラ実装
  - `internal/controller/kany8scontrolplane_controller.go`
  - `internal/controller/infrastructure/kany8scluster_controller.go`
  - `internal/kro/gvk.go`
  - `internal/kro/status.go`
  - `internal/endpoint/parse.go`
  - `internal/kubeconfig/secret.go`

- 例・テスト
  - `examples/capi/cluster.yaml`
  - `examples/capi/clusterclass.yaml`
  - `examples/kro/ready-endpoint/rgd.yaml`
  - `examples/kro/eks/eks-control-plane-rgd.yaml`
  - `examples/kro/eks/platform-cluster-rgd.yaml`
  - `internal/controller/cluster_topology_contract_test.go`
  - `internal/devtools/template_apis_test.go`
  - `internal/devtools/crd_bases_test.go`
  - `internal/devtools/examples_capi_test.go`
  - `internal/devtools/kany8scluster_api_test.go`

---

## 2. まず結論（妥当性の総評）

- ControlPlane 側のドメイン境界は妥当。
  - 「Kany8s controller は provider 固有 CR を直接読まず、kro instance の正規化 status だけを読む」という設計判断は、実装でも守られている。
  - `spec.resourceGraphDefinitionRef` による RGD 選択、RGD schema から instance GVK 解決、instance 1:1 管理、`spec.version` 注入、endpoint/initialized/conditions 更新、kubeconfig Secret 整形は、コンセプトに沿っている。

- Infrastructure 側は「提供する」方針に対し、CRD/Template/contract 実装が未完成。
  - 特に (1) ClusterClass/Topology の infra template が API group 的に破綻している点と、(2) CAPI v1beta2 の InfrastructureCluster contract で必須となる `status.initialization.provisioned` 相当が欠落している点が致命的。

---

## 3. 良い点（設計と実装が揃っているところ）

### 3.1 provider-agnostic の境界が成立している（ControlPlane）

- Kany8sControlPlane controller は kro instance の `status.ready` / `status.endpoint` / `status.reason` / `status.message` を読むだけで意思決定している（契約は `docs/rgd-contract.md`）。
- endpoint の解釈も provider 非依存（`internal/endpoint/parse.go`）。

### 3.2 RGD の「正規化インターフェース」が実体化している

- `docs/rgd-contract.md` の最小契約（ready/endpoint/reason/message）に合わせて、読み取りヘルパーが用意されている（`internal/kro/status.go`）。
- kro v0.7.1 の落とし穴（bool materialization 等）も `docs/rgd-guidelines.md` / `docs/kro.md` に整理され、例 RGD に反映されている（例: `examples/kro/eks/eks-control-plane-rgd.yaml`）。

### 3.3 CAPI contract に沿った ControlPlane provider の最小実装

- `spec.controlPlaneEndpoint` と `status.initialization.controlPlaneInitialized` を kro instance の endpoint から駆動（`internal/controller/kany8scontrolplane_controller.go`）。
- kubeconfig Secret を provider 非依存で整形（`internal/controller/kany8scontrolplane_controller.go` / `internal/kubeconfig/secret.go`）。

---

## 4. 問題点・課題（優先度つき）

### P0: `Kany8sClusterTemplate` の API group が誤っており、Topology で infra を生成できない

現状:

- `Kany8sCluster` は `infrastructure.cluster.x-k8s.io`（`api/infrastructure/v1alpha1/groupversion_info.go`）。
- しかし `Kany8sClusterTemplate` は `controlplane.cluster.x-k8s.io` 側に存在する（`api/v1alpha1/kany8sclustertemplate_types.go`、CRD も `config/crd/bases/controlplane.cluster.x-k8s.io_kany8sclustertemplates.yaml`）。
- `examples/capi/clusterclass.yaml` でも infra template の apiVersion が `controlplane...` になっている。

なぜ致命的か:

- Topology の template cloning は、template の `spec.template` を “実体オブジェクトの雛形” として扱い、生成されるリソースの apiVersion/kind は template 側に強く依存する。
- `Kany8sClusterTemplate`（controlplane group）から生成されるのは `controlplane.../Kany8sCluster` であり、実在する `infrastructure.../Kany8sCluster` と一致しない。

必要な方向性:

- `Kany8sClusterTemplate` は infrastructure API group 側に置くのが筋（`infrastructure.cluster.x-k8s.io/v1alpha1` / kind `Kany8sClusterTemplate`）。

### P0: `Kany8sCluster` が CAPI v1beta2 InfrastructureCluster contract を満たしていない（`status.initialization.provisioned` 相当が無い）

現状:

- `Kany8sClusterStatus` は `conditions` と `failureReason/failureMessage` のみ（`api/infrastructure/v1alpha1/kany8scluster_types.go`）。
- `internal/controller/infrastructure/kany8scluster_controller.go` も `conditions` を Ready=True にするだけ。

しかし CAPI v1beta2 側では:

- InfrastructureCluster の “provisioned” 判定は v1beta2 contract で `status.initialization.provisioned` を参照する。
- ここが欠落すると Cluster controller は `provisioned=false` と扱い、`Cluster.Status.Initialization.InfrastructureProvisioned=true` 設定等の遷移に進めない。

必要な方向性:

- 最小でも `Kany8sCluster.status.initialization.provisioned` を追加し、controller が True を立てる必要がある。
  - “stub infra provider” を続けるなら、常に True にするでも良いが、仕様として明文化が必要。

### P1: `kroSpec` が object 前提の実装だが、API で保証していない

現状:

- ControlPlane controller は `kroSpec` を `map[string]any` として `json.Unmarshal` し、`spec["version"]` を注入する（`internal/controller/kany8scontrolplane_controller.go`）。
- `kroSpec` は CRD 上 “任意 JSON” で、配列/文字列でも通り得る（`x-kubernetes-preserve-unknown-fields: true`）。

リスク:

- `kroSpec` が object 以外だと reconcile がエラーになり得る。

方向性案:

- CRD（OpenAPI）で `kroSpec` を object に寄せる / webhook で object 以外を弾く / controller で明示的に Condition 化して失敗扱いにする、のいずれか。

### P1: `failureReason/failureMessage` の意味付けが “致命的エラー” と “プロビジョニング中” で混ざり得る

現状:

- ControlPlane 側は NotReady の間も `failureReason/failureMessage` を埋める（`internal/controller/kany8scontrolplane_controller.go`）。

懸念:

- CAPI の慣習では failure* は “terminal failure” を示唆することが多い。
- “待っているだけ” の状態まで failure として扱うと、監視/アラートや上位ロジックが誤解しやすい。

方向性案:

- 進捗（provisioning）は Conditions の Reason/Message に集約し、failure* は回復不能/停止判断のみ、などのルールを決めて揃える。

### P2: `docs/rgd-contract.md` の “required” と実装の “欠落許容” のズレ

現状:

- `docs/rgd-contract.md` は `status.ready`/`status.endpoint` を required とする。
- 実装（`internal/kro/status.go`）は欠落を false/empty として扱える。

注意点:

- kro v0.7.1 は bool status field が欠落し得るため、欠落許容は運用上の保険になる。
- 一方で「欠落しない契約」を守ることが、provider RGD 作者/運用者の理解を簡単にする。

方向性案:

- 契約としては required を維持しつつ、controller は安全に欠落を扱う（現状）を “意図的仕様” として明記する。

### P2: RBAC が広い（MVPとしては理解できるが、将来の最小化ポイント）

現状:

- `internal/controller/kany8scontrolplane_controller.go` の RBAC は `kro.run` group に対して `resources=*` が含まれる。

方向性:

- 動的 GVK の都合上 MVP では現実的だが、将来は生成される instance GVK を絞る・ClusterRole を分割する等の余地。

### P3: `config/samples/*` が TODO のまま（例は `examples/` にある）

現状:

- `config/samples/controlplane_v1alpha1_kany8scontrolplane.yaml` 等が TODO のまま。

影響:

- “kubebuilder の定型サンプル” と “実用例（examples）” が二重化している。
- 利用者の導線が `examples/` で固定なら、samples は削除/更新/参照先誘導の整理が必要。

---

## 5. Infrastructure provider を本当に提供する場合のドメイン論点（未決のまま残る点）

Kany8s が infra 側も提供する場合、次のどちらを目指すかで CRD/contract が変わる。

### 5.1 “最小 infra provider（stub）”

- 目的: `Cluster.spec.infrastructureRef` を満たし、CAPI の provisioning フローを unblock する。
- 必須: CAPI v1beta2 の `status.initialization.provisioned` を True にする、`Ready` Condition を整える。
- kro との連携は不要（`kroSpec` すら不要になる可能性がある）。

### 5.2 “kro で infra も具象化する provider”

- 目的: VPC 等の infra を kro instance として管理し、status 正規化で可観測性を揃える。
- 必須: `Kany8sCluster` でも ControlPlane と同様の構造（`resourceGraphDefinitionRef` + `kroSpec` + 正規化 status）を持つ方が一貫する。
- 難所: infra の “出力（VPC ID 等）を ControlPlane に渡す” 問題をどう解くか。
  - 単純に provider 間で status を参照し合うと境界が汚れやすい。
  - `docs/design.md` の「outputs を core にしない」方針との整合を取り直す必要がある（親 RGD で束ねる/Topology variables に寄せる/別契約導入など）。

---

## 6. すぐに着手すべき修正（推奨順）

1. (P0) `Kany8sClusterTemplate` を infrastructure group に移し、`examples/capi/clusterclass.yaml` の infra ref も修正する。
2. (P0) `Kany8sCluster` に `status.initialization.provisioned` を追加し、controller が True を立てる（stub を続けるなら常に True でも良いが、仕様として明記）。
3. (P1) `kroSpec` の型（object 前提）を API か controller で保証する。
4. (P1) failureReason/failureMessage の運用意味を整理（progress vs terminal）。
5. (P2) RBAC 最小化、samples の導線整理は後続。

---

## 7. 参考メモ（知識として残す）

- Topology の template cloning は `spec.template` を実体オブジェクトの雛形として扱うため、**Template の API group を間違えると生成先の API group も間違う**。
- CAPI v1beta2 の InfrastructureCluster は `status.ready` ではなく `status.initialization.provisioned` を参照する（設計/実装時の落とし穴）。
- kro v0.7.1 の既知罠（`docs/kro.md` / `docs/rgd-guidelines.md`）:
  - `spec.schema.status` で `schema.*` を参照できない
  - `readyWhen` は self resource のみ
  - 文字列テンプレートでリテラル欠落が起こり得るため CEL 1式で連結する
  - bool status が materialize されないことがある（int/ternary トリックが必要）
  - optional resource 参照で status field 自体が欠落し得る
  - `NetworkPolicy` が Ready を妨げる可能性
