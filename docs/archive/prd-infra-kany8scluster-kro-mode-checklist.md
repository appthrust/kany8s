# PRD Appendix (Historical): `Kany8sCluster` kro mode checklist

- Source: `docs/PRD.md` (as of 2026-02-02), section "## 14. TODO: Infrastructure (`Kany8sCluster`)"
- Status: Historical (implementation completed; kept for traceability)

## 14. TODO: Infrastructure (`Kany8sCluster`)

目的: `Kany8sCluster` を "stub infra provider" から、kro(RGD) による infra 具象化ができる provider へ拡張する。

方針:

- MVP は後方互換: `spec.resourceGraphDefinitionRef` が無い場合は現状どおり stub として `provisioned=true` を立てる
- `spec.resourceGraphDefinitionRef` が指定された場合のみ kro 連携を有効化する
- `Kany8sCluster` controller は provider-specific な CR を直接読まず、kro instance の正規化 status のみを読む
- 新方針(案A): infra と control plane の値受け渡しが必要な場合は、`Kany8sControlPlane` が参照する "親 RGD" に infra を含めて完結させる。`Kany8sCluster` は control plane の inputs を生成/配布する役割を持たず、汎用 outputs を導入しない。あわせて親 RGD は `controlPlane.status.*` を top-level `status` に投影し、"親だけ見ればよい" 観測性を提供する。

### 14.1 Contract (docs)

- [x] infra RGD instance の status 契約を `docs/rgd-contract.md` に追記する
  - 追加: Infrastructure contract `status.ready` / `status.reason` / `status.message`
  - 追加: `Kany8sCluster.status.initialization.provisioned` への反映ルール（`status.ready=true` -> `provisioned=true`）
  - DoD: RGD 作者が "infra 側は何を出せば良いか" を迷わない

- [x] (Approach A) "親 RGD (infra + control plane)" の status 投影ガイドを `docs/rgd-contract.md` に追記する
  - 仕様: 親 instance は `ready/endpoint` を必ず投影し、`reason/message/kubeconfigSecretRef` も可能な限り透過する
  - DoD: Kany8s とユーザーが provider-specific な子リソースを覗かずに状況判断できる

- [x] (Approach A) infra outputs を control plane spec に渡す際の欠落耐性/必須 field のガイドを `docs/rgd-guidelines.md` に追記する
  - 例: `.?` / `orValue(...)` の使いどころ、配列/文字列の default 戦略、field 未生成時に "評価エラー" を起こさない
  - DoD: infra 待機中でも parent graph がテンプレート評価エラーで破綻しない

### 14.2 API (CRD)

- [x] infra API group に `ResourceGraphDefinitionReference` を追加する（ControlPlane と同等の形）
  - Touch: `api/infrastructure/v1alpha1/` (new file or inline)
  - DoD: `spec.resourceGraphDefinitionRef.name` を型として表現できる

- [x] `Kany8sClusterSpec` に `spec.resourceGraphDefinitionRef` を追加する（kro 連携のスイッチ）
  - Touch: `api/infrastructure/v1alpha1/kany8scluster_types.go`
  - 仕様: 未指定なら stub mode / 指定ありなら kro mode
  - DoD: `make manifests generate test` が通る

- [x] `Kany8sClusterTemplate` にも `resourceGraphDefinitionRef` を追加し、Topology から同等の入力が渡せるようにする
  - Touch: `api/infrastructure/v1alpha1/kany8sclustertemplate_types.go`
  - DoD: ClusterClass/Topology から生成される `Kany8sCluster` が kro mode に入れる

- [x] サンプルを更新する（kro mode の最小例を含める）
  - Touch: `config/samples/infrastructure_v1alpha1_kany8scluster.yaml`, `config/samples/infrastructure_v1alpha1_kany8sclustertemplate.yaml`
  - DoD: `make deploy` 後に samples が apply できる

### 14.3 Controller (kro integration)

- [x] RBAC を追加する（RGD 読み取り + kro instance 作成/更新）
  - Touch: `internal/controller/infrastructure/kany8scluster_controller.go` (+kubebuilder:rbac)
  - DoD: `make manifests` で生成される RBAC に `resourcegraphdefinitions.kro.run` と instance への権限が含まれる

- [x] kro mode の足場を実装する（RGD -> instance GVK 解決 + instance 1:1 create/update）
  - Touch: `internal/controller/infrastructure/kany8scluster_controller.go`
  - 利用: `internal/kro/gvk.go`
  - DoD: `spec.resourceGraphDefinitionRef` が指定された `Kany8sCluster` で kro instance が作成される

- [x] kro instance の spec 反映ルールを固定する（idempotent）
  - Touch: `internal/controller/infrastructure/kany8scluster_controller.go`
  - MVP 既定:
    - `Kany8sCluster.spec.kroSpec` を instance `.spec` に展開
    - `.spec.clusterName` / `.spec.clusterNamespace` は常に `Kany8sCluster` から注入
  - DoD: instance `.spec` の手動変更が reconcile で戻る

- [x] status/conditions を揃える（provisioned/Ready/failure）
  - Touch: `internal/controller/infrastructure/kany8scluster_controller.go`
  - 入力: `internal/kro/status.go` の `status.ready/reason/message`
  - DoD:
    - `status.initialization.provisioned = (instance.status.ready == true)`
    - `Ready` Condition は `provisioned` と同じ意味
    - `failureReason/failureMessage` は terminal error のみ（待機中はクリア）

### 14.4 Tests

- [x] fake client unit test で stub/kro mode の最小フローを固定する
  - Touch: `internal/controller/infrastructure/kany8scluster_reconciler_test.go`
  - ケース例:
    - stub mode: `provisioned=true` + `Ready=True`
    - kro mode: instance 未作成 -> 作成される
    - kro mode: instance `status.ready=true` -> `provisioned=true`
    - kro mode: status 欠落/false -> `provisioned=false` で待機できる
    - kro mode: RGD が無い/不正 -> Condition に理由が出て待機/失敗できる
  - Run: `make test`

- [x] devtools テストを追加/更新する（API/CRD/samples の回帰検知）
  - Touch: `internal/devtools/kany8scluster_api_test.go`, `internal/devtools/crd_bases_test.go` など
  - DoD: `make test` で CRD 生成・samples の整合が壊れたら検知できる

### 14.5 Examples

- [x] infra 向けの最小 RGD 例を追加し、`Kany8sCluster` と組み合わせた例を用意する
  - Add: `examples/kro/infra/` (RGD yaml)
  - Update (or add): `examples/capi/` (Cluster + infraRef の例)
  - DoD: `kubectl apply` で `Kany8sCluster` が kro mode で Provisioned になるデモができる
