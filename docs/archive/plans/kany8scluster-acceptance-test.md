# Kany8sCluster (Infrastructure) Acceptance Test

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** `Kany8sCluster` (Infrastructure provider) の kro mode を kind 上で実際に動かし、CAPI v1beta2 InfrastructureCluster contract（`status.initialization.provisioned` / `Ready` Condition）を満たすことを acceptance test で検証できるようにする。

**Architecture:** kro をインストールした kind 管理クラスタ上で、(1) infra 用 RGD を apply して instance CRD を生成し、(2) `Kany8sCluster` を apply、(3) Kany8s が kro instance を 1:1 作成し、instance `status.ready` を `status.initialization.provisioned` と `Ready` condition に反映できることを確認する。

**Tech Stack:** Cluster API (v1beta2 contract), kro (RGD), controller-runtime, kind, shell-based acceptance scripts。

---

## 背景 / 問題

- 既存の kro acceptance は主に `Kany8sControlPlane` の "status reflection" を扱っている。
  - 例: `test/acceptance_test/manifests/kro/kany8scontrolplane.yaml.tpl`
- `Kany8sCluster` は CAPI `Cluster.spec.infrastructureRef` を満たすために必須だが、
  - kro mode（`spec.resourceGraphDefinitionRef` を指定した場合）での **実環境(kro + kind)統合の検証**が不足しており、何を保証しているテストなのかが分かりにくい。
- ユニットテスト/フェイククライアントでは kro の CRD 生成や実際の unstructured resource のライフサイクル、RBAC/インストール手順などはカバーしにくい。

## 現状（コード上の挙動）

- API: `api/infrastructure/v1alpha1/kany8scluster_types.go`
  - `spec.resourceGraphDefinitionRef` が unset の場合: stub mode（常に provisioned/Ready を立てる）
  - set の場合: kro mode（RGD schema から instance GVK を解決し、instance を 1:1 作成/更新する）
- Controller: `internal/controller/infrastructure/kany8scluster_controller.go`
  - kro mode:
    - `kro.ResolveInstanceGVK(...)` で instance GVK を解決
    - kro instance を `CreateOrUpdate` で作る（owner ref を付与）
    - `kro.ReadInstanceStatus(...)` の `status.ready/reason/message` を読み、
      - `status.initialization.provisioned` に反映
      - `Ready` condition（type=`Ready`）に反映
- Contract: `docs/rgd-contract.md` の "Infrastructure (for `Kany8sCluster`)" セクション

## Non-goals

- 実クラウドの VPC/VNet などを作ること（acceptance はローカルで完結）
- `Kany8sCluster` の outputs を `Kany8sControlPlane` に受け渡す仕組みを導入すること（`docs/PRD-details.md` 方針に従いスコープ外）
- Cluster API の完全なプロビジョニングフロー（workload cluster 作成）を infra kro mode で実証すること（別途）

## 提案: 追加する Acceptance Test

### 追加するテスト名（目的ベース命名）

- Make target:
  - `make test-acceptance-kro-infra-reflection`
  - `make test-acceptance-kro-infra-reflection-keep`
- Script:
  - `hack/acceptance-test-kro-infra-reflection.sh`
- Wrapper runner:
  - `test/acceptance_test/run-acceptance-kro-infra-reflection.sh`

注: 既存の `test-acceptance-*` は legacy alias を残す方針があるため、必要なら alias も追加する。

### 使用する RGD（ローカル完結）

`examples/kro/infra/rgd.yaml` は以下の点で acceptance に向いている:

- 依存が少ない（ConfigMap 1つ）
- schema が `clusterName/clusterNamespace` を必須にしており、Kany8s の注入ロジック検証に適合
- `status.ready/reason/message` を明示しており、`docs/rgd-contract.md` の infra contract に一致

acceptance は `examples/` ではなく `test/acceptance_test/manifests/` を source-of-truth にしたいので、以下にコピー/配置する。

- `test/acceptance_test/manifests/kro/infra/rgd.yaml` (new)

### 追加する manifest template

- `test/acceptance_test/manifests/kro/kany8scluster.yaml.tpl` (new)

例（テンプレの完成形イメージ）:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: Kany8sCluster
metadata:
  name: __CLUSTER_NAME__
  namespace: __NAMESPACE__
spec:
  resourceGraphDefinitionRef:
    name: __RGD_NAME__
  # kroSpec はこのテストでは必須ではない（RGD が clusterName/clusterNamespace のみ要求するため）
```

### テストが検証すること（成功条件）

kind 管理クラスタ上で:

1) kro infra RGD の apply

- `rgd/demo-infra.kro.run` が `ResourceGraphAccepted=True`
- instance CRD が作成される（例: `demoinfrastructures.kro.run`）

2) `Kany8sCluster` の kro mode

- `Kany8sCluster/<name>` を apply すると kro instance が 1:1 作成される
  - instance: `DemoInfrastructure/<name>`（RGD の schema から導出される kind）
- Kany8s により instance `.spec.clusterName/.spec.clusterNamespace` が注入されている
- instance `status.ready=true` を契機に、Kany8s が以下を満たす:
  - `Kany8sCluster.status.initialization.provisioned=true`
  - `Kany8sCluster Ready=True`
  - `Kany8sCluster.status.failureReason/failureMessage` は通常系で nil

（任意/将来）stub mode の確認:

- `spec.resourceGraphDefinitionRef` なしの `Kany8sCluster` が即座に `provisioned=true` / `Ready=True` になる

---

## 実装プラン（Bite-sized tasks）

### Task 1: Acceptance 用 infra RGD を追加

**Files:**
- Create: `test/acceptance_test/manifests/kro/infra/rgd.yaml`

**Step 1: ファイル追加**

- 元: `examples/kro/infra/rgd.yaml`
- そのままコピー（必要なら `metadata.name` を固定: `demo-infra.kro.run`）

**Step 2: 動作確認（静的）**

Run: `kubectl apply --dry-run=client -f test/acceptance_test/manifests/kro/infra/rgd.yaml`
Expected: exit 0

### Task 2: `Kany8sCluster` manifest template を追加

**Files:**
- Create: `test/acceptance_test/manifests/kro/kany8scluster.yaml.tpl`

**Step 1: テンプレ作成**

- `__CLUSTER_NAME__`, `__NAMESPACE__`, `__RGD_NAME__` を置換可能にする

**Step 2: 置換後 YAML の dry-run**

Run: `sed ... kany8scluster.yaml.tpl | kubectl apply --dry-run=client -f -`
Expected: exit 0

### Task 3: `hack/acceptance-test-kro-infra-reflection.sh` を追加

**Files:**
- Create: `hack/acceptance-test-kro-infra-reflection.sh`
- Modify (optional): `test/acceptance_test/run-all.sh`

**Step 1: script を scaffold**

- 既存の `hack/acceptance-test-kro-reflection.sh` と同様の骨格を流用
- ただし controlplane ではなく infra を対象にする:
  - apply: `test/acceptance_test/manifests/kro/infra/rgd.yaml`
  - apply: `test/acceptance_test/manifests/kro/kany8scluster.yaml.tpl`
- Assert:
  - `kubectl wait --for=condition=Ready kany8scluster/<name>`
  - `kubectl wait --for=jsonpath='{.status.initialization.provisioned}'=true kany8scluster/<name>`
  - instance CR の存在（CRD 名は固定の期待値にするか、`kubectl get rgd ... -o jsonpath` から導出）
  - instance `.spec.clusterName/.spec.clusterNamespace` の検証

**Step 2: bash syntax check**

Run: `bash -n hack/acceptance-test-kro-infra-reflection.sh`
Expected: exit 0

### Task 4: Make target / wrapper runner を追加

**Files:**
- Modify: `Makefile`
- Create: `test/acceptance_test/run-acceptance-kro-infra-reflection.sh`
- Modify: `test/acceptance_test/README.md`

**Step 1: Make target を追加**

Targets:

- `test-acceptance-kro-infra-reflection`
- `test-acceptance-kro-infra-reflection-keep`

**Step 2: wrapper runner を追加**

- 他 runner と同様に pre-clean kind cluster を行い `hack/` に委譲

### Task 5: repo-policy テストを更新

**Files:**
- Modify: `internal/devtools/acceptance_test_script_test.go`

**Step 1: fail-first（RED）**

- 新しい target / script の存在確認を追加し、`go test ./internal/devtools` が FAIL することを確認

**Step 2: 実装後に GREEN**

Run: `go test ./internal/devtools`
Expected: PASS

### Task 6: ドキュメントを更新

**Files:**
- Modify: `docs/README.md`
- Modify: `docs/e2e-and-acceptance-test.md`
- Modify: `docs/codebase.md`

**Step 1: 新しい target 名を追記**

- 目的ベース命名に合わせて、一覧に `test-acceptance-kro-infra-reflection` を追加

### Task 7: まとめて検証

Run:

- `go test ./...`
- `bash -n hack/acceptance-test-kro-infra-reflection.sh`

Optional (ローカルで実行できる場合):

- `make test-acceptance-kro-infra-reflection`

---

## 関連リンク

- `api/infrastructure/v1alpha1/kany8scluster_types.go`
- `internal/controller/infrastructure/kany8scluster_controller.go`
- `docs/rgd-contract.md` (Infrastructure contract)
- `examples/kro/infra/rgd.yaml` (ベースにする demo infra RGD)
- `test/acceptance_test/manifests/kro/kany8scontrolplane.yaml.tpl` (既存の controlplane 側テンプレ)
