# Kany8s 実装調査レポート

- 対象リポジトリ: `github.com/reoring/kany8s`
- 調査日時: 2026-01-26
- 調査対象コミット: `101f6d32edc7733e12ffca06ab748cecf5386db9` (branch: `main`)
- 調査環境:
  - Go: `go1.25.5` (toolchain `go1.25.5`, `go.mod` は `toolchain go1.25.5`)
  - kubectl: `v1.35.0`
  - kind: `v0.31.0`
  - docker: `29.1.3`
  - kro: `v0.7.1` (ドキュメント記載の tested version)

## 1. 目的

ユーザー要望: 「この Kany8s を実装した。各種ドキュメントに記述されているような内容が実現できているか入念に検査してほしい」

本レポートは、以下を対象に "ドキュメントと実装の整合" と "実際に動く/動かない" を、証拠(コマンド実行結果・コード参照)に基づいて評価する。

## 2. 調査範囲

### 2.1 参照ドキュメント

- `README.md`
- `docs/PRD.md`
- `docs/design.md`
- `docs/reference/rgd-contract.md`
- `docs/reference/rgd-guidelines.md`
- `docs/reference/kro-v0.7.1-kind-notes.md`
- `docs/runbooks/kind-kro.md`
- `docs/runbooks/ack.md`
- `docs/runbooks/e2e.md`
- `docs/runbooks/clusterctl.md`
- `docs/runbooks/release.md`

### 2.2 実装/成果物

- API/CRD: `api/**`, `config/crd/bases/**`
- Controller: `internal/controller/**`
- kro 連携: `internal/kro/**`, `internal/dynamicwatch/**`
- endpoint/kubeconfig: `internal/endpoint/**`, `internal/kubeconfig/**`
- RBAC/マニフェスト: `config/**`
- サンプル: `examples/**`, `config/samples/**`

### 2.3 実行した検証コマンド(証拠)

- Unit/Integration (envtest): `make test`
- Lint: `make lint`
- E2E (kind): `CERT_MANAGER_INSTALL_SKIP=true make test-e2e`
- 手動 smoke test (kind + kro + sample RGD + Kany8s): 実行したが、RGD が kro に reject されるため途中で停止 (詳細は 6.1)

## 3. 全体評価 (結論)

### 3.1 実現できている (OK)

- `Kany8sControlPlane` が "RGD 名を参照 → 生成 instance の GVK を解決 → kro instance を 1:1 作成/更新" し、正規化 status を消費して CAPI contract に必要な field を更新する流れは実装済み。
  - 参照: `internal/controller/kany8scontrolplane_controller.go`
- endpoint parse (仕様: `https://host[:port]` または `host[:port]`, port 省略は 443) はユーティリティ + テストで担保。
  - 参照: `internal/endpoint/parse.go`, `internal/endpoint/parse_test.go`
- kubeconfig Secret の contract (type/label/data.value) を満たす Secret を reconcile で作成/更新する実装とテストがある。
  - 参照: `internal/kubeconfig/secret.go`, `internal/controller/kany8scontrolplane_controller.go`
- dynamic watch (動的 GVK の instance 更新で enqueue) は実装とテストがある。
  - 参照: `internal/dynamicwatch/watcher.go`, `internal/controller/kany8scontrolplane_controller_test.go`
- RBAC は MVP として kro instance を扱えるよう広め(kro.run `resources='*'`)に付与されている。
  - 参照: `internal/controller/kany8scontrolplane_controller.go` RBAC marker, 生成物 `config/rbac/role.yaml`

### 3.2 ドキュメント記載の "デモ手順" が現状成立しない (NG / 重大)

次の理由により、`README.md` の kind + kro デモ手順をそのまま実行しても "RGD apply → 生成 CRD → instance 作成" まで到達できない。

- `examples/kro/**` の複数 RGD が **kro v0.7.1 に reject される記法**(`int(<bool>)`)を含んでいる
  - 例: `examples/kro/ready-endpoint/rgd.yaml` の `status.ready`
  - その結果: RGD の `ResourceGraphAccepted=False` となり、生成 CRD (例: `democontrolplanes.kro.run`) が作られない
  - 詳細: 6.1 を参照

### 3.3 サンプル YAML の apiVersion 不整合 (NG)

- `examples/capi/cluster.yaml` が `Kany8sCluster` の apiVersion に `infrastructure.cluster.x-k8s.io/v1beta2` を指定しているが、実 CRD は `v1alpha1`。
  - 参照: `examples/capi/cluster.yaml`, `config/crd/bases/infrastructure.cluster.x-k8s.io_kany8sclusters.yaml`
  - このままだと apply で失敗する。

### 3.4 ドキュメント上の API バージョン表記のズレ (部分的)

- `README.md` と `docs/PRD.md` に `cluster.x-k8s.io/v1beta1` が残っているが、実際のサンプル/依存は `v1beta2`。
  - 参照: `README.md`, `docs/PRD.md`, `examples/capi/cluster.yaml`, `examples/capi/clusterclass.yaml`

### 3.5 未検証 (スコープ外/環境依存)

- ACK/EKS の実クラウド上での EKS 作成が end-to-end で成功するか (AWS 認証情報が必要)
- `examples/kro/gke/**`, `examples/kro/aks/**` の実動作 (KCC/ASO 等の CRD/認証が必要)
- clusterctl 連携 (runbook はあるが、`clusterctl init` 実行は未実施)

## 4. 要求事項チェックリスト (Docs → 実装の対応)

以下は `README.md` / `docs/PRD.md` / `docs/design.md` / `docs/reference/rgd-contract.md` で読み取れる要件を中心に、実装と証拠を紐付けたもの。

| 要件 | 出典 | 実装/証拠 | 評価 |
|---|---|---|---|
| `Kany8sControlPlane.spec.version` required | `docs/PRD.md` | CRD required (`config/crd/bases/controlplane.cluster.x-k8s.io_kany8scontrolplanes.yaml`) | OK |
| `spec.resourceGraphDefinitionRef.name` required | `docs/PRD.md`, `docs/design.md` | CRD required + controller 使用 (`internal/controller/kany8scontrolplane_controller.go`) | OK |
| `spec.kroSpec` を任意の object として passthrough | `docs/PRD.md`, `README.md` | `apiextensionsv1.JSON` + preserve unknown fields | OK |
| RGD の schema(apiVersion/kind) から instance GVK を解決 | `docs/PRD.md` | `internal/kro/gvk.go` + `internal/kro/gvk_test.go` | OK |
| kro instance 1:1 作成/更新 (name/namespace 同一) | `README.md`, `docs/PRD.md` | CreateOrUpdate + tests | OK |
| kro instance spec に version を必ず注入(上書き) | `README.md`, `docs/PRD.md` | `spec["version"] = cp.Spec.Version` + drift 修正 test | OK |
| status.ready/endpoint/reason/message の安全な読み取り | `docs/reference/rgd-contract.md` | `internal/kro/status.go` | OK |
| endpoint parse (`https://host[:port]` or `host[:port]`, default 443) | `README.md`, `docs/reference/rgd-contract.md` | `internal/endpoint/parse.go` + tests | OK |
| endpoint を `Kany8sControlPlane.spec.controlPlaneEndpoint` に反映 | `README.md`, `docs/design.md` | controller 実装 + tests | OK |
| endpoint 確定後に `status.initialization.controlPlaneInitialized=true` | `README.md`, `docs/PRD.md` | controller 実装 + tests | OK |
| Conditions/failureReason/failureMessage の更新 | `docs/PRD.md` | controller 実装 + tests | OK |
| RGD 未解決時の condition + event + requeue | `docs/PRD.md` | `requeueWithRGDResolutionCondition` + tests | OK |
| kubeconfig Secret の contract を満たす (`<cluster>-kubeconfig`, type/label/data.value) | `docs/design.md` | controller 実装 + `internal/kubeconfig/**` tests | OK |
| dynamic watch (ポーリングに加えて instance 更新で enqueue) | `docs/PRD.md` | `internal/dynamicwatch/watcher.go` + manager テスト | OK |
| `Kany8sCluster` の最小実装で InfrastructureReady を unblock | `docs/PRD.md` | `internal/controller/infrastructure/kany8scluster_controller.go` (Ready=True 固定) | OK (最小) |
| `Kany8sControlPlaneTemplate`/`Kany8sClusterTemplate` API | `docs/PRD.md`, `README.md` | `api/v1alpha1/*template*.go` + `examples/capi/clusterclass.yaml` | OK |
| RGD 正規化 contract を満たす example | `README.md`, `docs/PRD.md` | `examples/kro/**` は存在するが kro v0.7.1 で reject (6.1) | NG |
| CAPI サンプル YAML が apply 可能 | `README.md`, `docs/PRD.md` | `examples/capi/cluster.yaml` に apiVersion 不整合 (3.3) | NG |

## 5. 実装確認: 重要コンポーネントのポイント

### 5.1 `Kany8sControlPlane` controller

- RGD 取得と GVK 解決
  - `kro.ResolveInstanceGVK(ctx, r, cp.Spec.ResourceGraphDefinitionRef.Name)`
  - 参照: `internal/kro/gvk.go`
- kro instance の作成/更新
  - `unstructured.Unstructured` を使用
  - `controllerutil.CreateOrUpdate` で idempotent
  - OwnerReference を `SetControllerReference` で付与
  - `spec.version` を常に `Kany8sControlPlane.spec.version` で上書き
  - 参照: `internal/controller/kany8scontrolplane_controller.go`
- status の消費
  - `kro.ReadInstanceStatus(instance)` で `ready/endpoint/reason/message` を取得
  - `ready && endpoint != ""` を "ControlPlane Ready" として扱う
  - endpoint があれば parse → `spec.controlPlaneEndpoint` を patch
  - endpoint が確定したら `status.initialization.controlPlaneInitialized=true`
  - conditions/failure を status に応じて更新
  - 参照: `internal/controller/kany8scontrolplane_controller.go`

### 5.2 kubeconfig Secret

- kro instance の `status.kubeconfigSecretRef` を source として読み取り、`<cluster>-kubeconfig` Secret を作成/更新
  - `type=cluster.x-k8s.io/secret`
  - `cluster.x-k8s.io/cluster-name=<cluster>` label
  - `data.value` に kubeconfig
  - 参照: `internal/controller/kany8scontrolplane_controller.go`, `internal/kubeconfig/secret.go`
- 単体テストがあり、source 更新に追従することも確認する
  - 参照: `internal/controller/kany8scontrolplane_kubeconfig_test.go`

### 5.3 `Kany8sCluster` controller (Infrastructure provider)

- 現状は "最小" 実装で `Ready=True` を立てるのみ
  - 参照: `internal/controller/infrastructure/kany8scluster_controller.go`
  - テスト: `internal/controller/infrastructure/kany8scluster_reconciler_test.go`

### 5.4 Topology/Template

- Template API:
  - `api/v1alpha1/kany8scontrolplanetemplate_types.go`
  - `api/v1alpha1/kany8sclustertemplate_types.go`
- Topology version の流れ (ClusterClass/Topology → ControlPlane spec.version → kro instance spec.version) は、ユニットテストでシミュレーションしている
  - 参照: `internal/controller/cluster_topology_contract_test.go`

## 6. 不整合/不具合の詳細

### 6.1 `examples/kro/**` の RGD が kro v0.7.1 で reject される (重大)

#### 現象

`examples/kro/ready-endpoint/rgd.yaml` を kro v0.7.1 に apply すると、RGD が `ResourceGraphAccepted=False` になり、生成 CRD が作成されない。

#### エラー(実測)

RGD の status.conditions には以下のようなエラーが記録される:

```text
failed to type-check status expression "int(deployment.?status.?availableReplicas.orValue(0) > 0) == 1" at path "ready":
ERROR: <input>:1:4: found no matching overload for 'int' applied to '(bool)'
```

#### 原因

- `int()` の引数が boolean になっている (`int(<bool>)`)。
  - CEL の `int()` は bool からの変換を受け付けないため、type-check で reject される。

#### 影響

- `README.md` の "kind + kro" デモ手順が成立しない
  - RGD が受理されないため instance CRD が生成されず、`Kany8sControlPlane` が参照する kro instance を作成できない
- `examples/kro/eks/**` や `examples/kro/gke/**`, `examples/kro/aks/**` も同様の書き方を含み、同じ問題を内包する可能性が高い

#### 影響範囲(該当ファイル)

- `examples/kro/ready-endpoint/rgd.yaml`
- `examples/kro/eks/eks-control-plane-rgd.yaml`
- `examples/kro/eks/eks-addons-rgd.yaml`
- `examples/kro/eks/pod-identity-set-rgd.yaml`
- `examples/kro/eks/platform-cluster-rgd.yaml`
- `examples/kro/gke/gke-control-plane-rgd.yaml`
- `examples/kro/aks/aks-control-plane-rgd.yaml`

#### 再現手順(最小)

```bash
kind create cluster --name kany8s --wait 60s
kubectl config use-context kind-kany8s

kubectl create namespace kro-system
kubectl apply -f https://github.com/kubernetes-sigs/kro/releases/download/v0.7.1/kro-core-install-manifests.yaml
kubectl rollout status -n kro-system deploy/kro

kubectl apply -f examples/kro/ready-endpoint/rgd.yaml
kubectl describe rgd demo-control-plane.kro.run
```

#### 備考 (ドキュメントとの齟齬)

- `docs/PRD.md` と `docs/reference/rgd-guidelines.md` に `int(<expr>) == 1` の記述があるが、`<expr>` が bool になるケースではこの形式は成立しない。
  - 参照: `docs/PRD.md`, `docs/reference/rgd-guidelines.md`

### 6.2 `examples/capi/cluster.yaml` の apiVersion 不整合 (重大)

- `examples/capi/cluster.yaml`:
  - `infrastructureRef.apiVersion: infrastructure.cluster.x-k8s.io/v1beta2`
- 実CRD:
  - `infrastructure.cluster.x-k8s.io/v1alpha1` (CRD name: `kany8sclusters.infrastructure.cluster.x-k8s.io`)

このため、サンプルをそのまま apply すると `no matches for kind "Kany8sCluster" in version "infrastructure.cluster.x-k8s.io/v1beta2"` のようなエラーになる。

### 6.3 README/PRD の CAPI apiVersion 表記が古い (中)

- `README.md` と `docs/PRD.md` に `cluster.x-k8s.io/v1beta1` の例が残っている
- 一方で、リポジトリ内のサンプルは `v1beta2` (`examples/capi/**`) で、CAPI 依存も `sigs.k8s.io/cluster-api v1.12.2` (v1beta2 系)

## 7. 検証結果 (コマンド実行ログ要約)

### 7.1 `make test`

- 実行: `make test`
- 結果: 成功 (envtest 使用)
- 主要成功パッケージ: `internal/controller`, `internal/controller/infrastructure`, `internal/endpoint`, `internal/kro`, `internal/kubeconfig`

### 7.2 `make lint`

- 実行: `make lint`
- 結果: `0 issues.`

### 7.3 `CERT_MANAGER_INSTALL_SKIP=true make test-e2e`

- 実行: `CERT_MANAGER_INSTALL_SKIP=true make test-e2e`
- 結果: 成功 (2 specs)
- 注意: e2e は "マネージャ起動 + metrics が取れる" までで、kro 連携や ControlPlane endpoint/initialized 反映の end-to-end はカバーしていない
  - 参照: `test/e2e/e2e_test.go`

### 7.4 手動 smoke test (kind + kro + Kany8s + demo RGD)

#### 目的

`README.md` 記載の流れ (install kro + install Kany8s + apply RGD + apply ControlPlane) が実際に成立するかを、kind 上で最小構成で確認する。

#### 実施内容(概要)

- kind クラスタ作成
- kro v0.7.1 install
- (必要に応じて) kro controller の RBAC 緩和 (`kro:controller:unrestricted`)
- Kany8s install (`make install`, `make deploy`)
- `examples/kro/ready-endpoint/rgd.yaml` apply
- 生成 CRD (`democontrolplanes.kro.run`) の生成・Established を wait

#### 結果

- `examples/kro/ready-endpoint/rgd.yaml` が `ResourceGraphAccepted=False` となり、生成 CRD が作られない。
- `kubectl get rgd demo-control-plane.kro.run -o yaml` で `int(<bool>)` の type-check error が確認できた (6.1 の通り)。

このため、現状のままでは README のデモフローを end-to-end で成立させる検証まで進められない。

## 8. 推奨アクション (優先度順)

### 8.1 最優先: `examples/kro/**` を kro v0.7.1 で受理される形に修正

- 目的: README の "kind + kro" デモを成立させ、Kany8s の核である "RGD → instance → status 消費" を実際に確認可能にする
- 方針案:
  - `int(<bool>)` を撤廃し、`int(<int>)` で数値化した上で比較する、または条件演算子 `cond ? 1 : 0` を使って int を生成する
  - `docs/reference/kro-v0.7.1-kind-notes.md` / `docs/reviews/issues-2026-01-28.md` で記載されている kro v0.7.1 の癖(特に "bool status field が欠落する" 系)に配慮して "常に field が materialize される" 形へ寄せる

### 8.2 次点: `examples/capi/cluster.yaml` の apiVersion を実 CRD に合わせる

- `infrastructureRef.apiVersion` を `infrastructure.cluster.x-k8s.io/v1alpha1` に修正

### 8.3 次点: README/PRD の apiVersion 表記を `v1beta2` に揃える

- ドキュメント・サンプル・依存が揃うことで、利用者が迷いにくくなる

### 8.4 改善: endpoint parse 失敗時の可観測性

- 現状: endpoint parse 失敗時に condition/failure の更新や requeue をせずに return している
  - 参照: `internal/controller/kany8scontrolplane_controller.go`
- 推奨: `Ready=False` (Reason/Message を parse failure に) として surfacing + 適度に requeue

### 8.5 追加テスト: kro 連携を e2e に組み込む

- kind に kro を入れて `examples/kro/ready-endpoint/rgd.yaml` を apply
- `Kany8sControlPlane` を apply
- `spec.controlPlaneEndpoint` と `status.initialization.controlPlaneInitialized` が立つことを wait

## 9. 運用上の注意 / 追加観測

### 9.1 `make deploy` / `make build-installer` が `config/manager/kustomization.yaml` を変更する

- `Makefile` の `deploy` と `build-installer` は `kustomize edit set image controller=${IMG}` を実行するため、作業ツリーが変更される。
  - 参照: `Makefile`
- リリース運用としては「リリースタグの image に固定する」目的でコミットする選択肢もあるが、ローカル検証のたびに差分が出る点には注意が必要。

### 9.2 `config/samples/**` は現状 TODO placeholder

- `config/samples/` 配下のサンプルは、現状 `spec:` が `# TODO(user): Add fields here` のままで、README の実行手順のサンプルとしては利用できない。
  - 参照: `config/samples/controlplane_v1alpha1_kany8scontrolplane.yaml` など

### 9.3 観測性(conditions/events)

- RGD 解決失敗(未存在/不正)では condition を立て、イベントも出す。
  - 参照: `internal/controller/kany8scontrolplane_controller.go`
- 一方で、endpoint parse failure は condition/failure に反映されず requeue もしないため、運用時に原因追跡が難しくなり得る。
  - 参照: `internal/controller/kany8scontrolplane_controller.go`

### 9.4 RBAC の広さ

- MVP として `kro.run` の `resources='*'` を許可しており、最小権限ではない。
  - 参照: `config/rbac/role.yaml`

## 10. 付録: 参照した主なファイル

- `internal/controller/kany8scontrolplane_controller.go`
- `internal/controller/infrastructure/kany8scluster_controller.go`
- `internal/kro/gvk.go`
- `internal/kro/status.go`
- `internal/endpoint/parse.go`
- `internal/kubeconfig/secret.go`
- `internal/dynamicwatch/watcher.go`
- `config/crd/bases/controlplane.cluster.x-k8s.io_kany8scontrolplanes.yaml`
- `config/crd/bases/infrastructure.cluster.x-k8s.io_kany8sclusters.yaml`
- `config/rbac/role.yaml`
- `examples/kro/**`
- `examples/capi/**`
