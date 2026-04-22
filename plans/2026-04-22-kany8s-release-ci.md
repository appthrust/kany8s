---
title: kany8s リリース CI 構築プラン（capt v0.4.9 参照・v2）
date: 2026-04-22
owner: kahirokunn
status: draft
revision: 2
references:
  - https://github.com/appthrust/capt/releases/tag/v0.4.9
  - /srv/platform/refs/capt (tag v0.4.9)
  - https://cluster-api.sigs.k8s.io/developer/providers/contracts
  - https://github.com/kubernetes-sigs/cluster-api-operator
---

# kany8s リリース CI 構築プラン v2

## 1. ゴール

- `appthrust/kany8s` を **clusterctl / cluster-api-operator から直接 install 可能な Provider** としてリリースする。
- `main` に merge されると **release-please が Release PR を自動生成**、その PR を merge すると `v${VERSION}` タグが push され、`release.yml` が自動発火。
- コンテナイメージ・リリースアセットは全て **public**（ghcr.io）。

### 対象イメージ（階層命名に統一）

- `ghcr.io/appthrust/kany8s/manager:vX.Y.Z` — CAPI provider の controller（**リリース必須**）
- `ghcr.io/appthrust/kany8s/eks-kubeconfig-rotator:vX.Y.Z` — 内部プラグイン（manager と同期リリース）
- `ghcr.io/appthrust/kany8s/eks-karpenter-bootstrapper:vX.Y.Z` — 内部プラグイン（manager と同期リリース）

> `ghcr.io/appthrust/kany8s-*` のハイフン区切りだと package 名が別々に切られるため、`ghcr.io/appthrust/kany8s/<component>` の**階層命名に揃える**（ユーザ指示）。

### Release 資産（GitHub Release に添付）

- `metadata.yaml`
- `infrastructure-components.yaml`
- `control-plane-components.yaml`
- `cluster-template.yaml`
- `clusterctl-config.yaml`（ユーザ向けサンプル）

## 2. 参考にした capt v0.4.9 の構成（検証済）

```
capt/
├── config/
│   ├── clusterapi/
│   │   ├── infrastructure/     ← CRDs のみ（base）
│   │   │   ├── bases/*.yaml
│   │   │   └── kustomization.yaml (commonLabels: infrastructure-capt)
│   │   └── controlplane/       ← CRDs のみ（base）
│   ├── clusterctl/
│   │   ├── infrastructure/
│   │   │   ├── kustomization.yaml (resources: ../../clusterapi/infrastructure, rbac, ../../manager)
│   │   │   └── rbac/kustomization.yaml (共有RBAC + infrastructure-role + binding)
│   │   └── controlplane/
│   │       ├── kustomization.yaml (resources: ../../clusterapi/controlplane, rbac, ../../manager)
│   │       └── rbac/kustomization.yaml (共有RBAC + controlplane-role + binding)
│   └── manager/
│       ├── kustomization.yaml
│       ├── manager.yaml
│       ├── infrastructure-args-patch.yaml   ← --enable-controller=infrastructure
│       └── controlplane-args-patch.yaml     ← --enable-controller=control-plane
├── hack/capi/
│   ├── metadata.yaml     ← releaseSeries 宣言
│   └── config.yaml       ← ローカル clusterctl 用
└── .github/workflows/
    ├── release.yml       ← v* / v*-rc* tag トリガ
    ├── release-helm.yml  ← main push で OCI chart push
    ├── ci.yml            ← PR / main push で lint+test
    └── smoke-test.yml    ← kind + clusterctl init gate
```

### 特に重要な仕様（v1 で誤っていたポイント）

1. **ビルド対象は `config/clusterctl/<group>`**、`config/clusterapi/<group>` ではない。後者は CRD base のみ。
2. **RBAC は group-scoped** にするため `controller-gen rbac:roleName=manager-role-<group> paths=...` を group ごとに実行し、`config/rbac/<group>-role.yaml` を生成。共有RBAC（service_account, leader_election 等）と組み合わせる。
3. **manager Deployment は単一バイナリを2 instance 起動**。`--enable-controller=infrastructure` / `--enable-controller=control-plane` でどの reconciler を動かすかを分離する。**kany8s の `cmd/main.go` は現状このフラグを実装していない**ため、**Phase 0 として実装が必須**。
4. **kustomize build 時に `--load-restrictor LoadRestrictionsNone`** が必要（`../../manager` など相対パス越境）。
5. **Provider ラベル** `cluster.x-k8s.io/provider=infrastructure-kany8s` / `control-plane-kany8s` を kustomize `commonLabels` で全リソースに付与。
6. **contract は CAPI core の contract を指す**。kany8s 自身の CRD が `v1alpha1` でも、`Cluster.spec.infrastructureRef` を v1beta1 で参照する限り `contract: v1beta1`。`v1alpha1` と書くと clusterctl が拒否する。

## 3. kany8s 現状との差分

| 項目 | 現状 | 必要アクション |
|---|---|---|
| `PROJECT` | `multigroup: true` / controlplane + infrastructure 群 ✅ | そのまま |
| `cmd/main.go` flags | `leader-elect`, `enable-webhooks` のみ | **`--enable-controller` を追加（Phase 0）** |
| `config/rbac/role.yaml` | 単一 `manager-role` | group-scoped role を生成（`manager-role-infrastructure`, `manager-role-controlplane`） |
| `config/clusterapi/<group>/` | 未作成 | 新規（CRDs のみ） |
| `config/clusterctl/<group>/` | 未作成 | 新規（CRDs + rbac + manager） |
| `config/manager/*-args-patch.yaml` | 未作成 | 新規2ファイル |
| `hack/capi/{metadata.yaml,config.yaml}` | 未作成 | 新規 |
| `templates/cluster-template.yaml` | 未作成 | 新規（kubeadm + kro の両 backend 用） |
| `Makefile` | `build-installer` のみ | `clusterctl-setup` / `clusterapi-manifests` を追加 |
| `VERSION` file | なし | release-please 管理 |
| `.github/workflows/release*.yml` | なし | 新規3本（release / release-please / smoke-test） |
| Go 版 | `go 1.25.7` | `actions/setup-go@v5` + `go-version-file: go.mod`（ハードコード禁止） |
| ControlPlane kinds | `Kany8sControlPlane` / `Kany8sControlPlaneTemplate` / `Kany8sKubeadmControlPlane` | **3 kind すべてを単一 ControlPlaneProvider に同梱**（ユーザ決定） |

## 4. 設計詳細

### 4.1 kustomize overlay 構造（v1 から全面修正）

```
config/
├── clusterapi/
│   ├── infrastructure/
│   │   ├── bases/
│   │   │   ├── infrastructure.cluster.x-k8s.io_kany8sclusters.yaml
│   │   │   └── infrastructure.cluster.x-k8s.io_kany8sclustertemplates.yaml
│   │   └── kustomization.yaml
│   ├── controlplane/
│   │   ├── bases/
│   │   │   ├── controlplane.cluster.x-k8s.io_kany8scontrolplanes.yaml
│   │   │   ├── controlplane.cluster.x-k8s.io_kany8scontrolplanetemplates.yaml
│   │   │   └── controlplane.cluster.x-k8s.io_kany8skubeadmcontrolplanes.yaml
│   │   └── kustomization.yaml
│   ├── kustomizeconfig.yaml
│   └── kustomization.yaml
├── clusterctl/
│   ├── infrastructure/
│   │   ├── kustomization.yaml      # resources: ../../clusterapi/infrastructure, rbac, ../../manager
│   │   └── rbac/kustomization.yaml
│   └── controlplane/
│       ├── kustomization.yaml      # resources: ../../clusterapi/controlplane, rbac, ../../manager
│       └── rbac/kustomization.yaml
└── manager/
    ├── kustomization.yaml
    ├── manager.yaml
    ├── infrastructure-args-patch.yaml
    └── controlplane-args-patch.yaml
```

`config/clusterapi/infrastructure/kustomization.yaml`（CRDs のみ）:
```yaml
namespace: kany8s-system
namePrefix: kany8s-
nameSuffix: -infrastructure
resources:
- bases/infrastructure.cluster.x-k8s.io_kany8sclusters.yaml
- bases/infrastructure.cluster.x-k8s.io_kany8sclustertemplates.yaml
commonLabels:
  cluster.x-k8s.io/provider: infrastructure-kany8s
  cluster.x-k8s.io/v1beta1: v1beta1
```

`config/clusterctl/infrastructure/kustomization.yaml`（composition）:
```yaml
namespace: kany8s-system
namePrefix: kany8s-
nameSuffix: -infrastructure
resources:
- ../../clusterapi/infrastructure
- rbac
- ../../manager
commonLabels:
  cluster.x-k8s.io/provider: infrastructure-kany8s
  cluster.x-k8s.io/v1beta1: v1beta1
configurations:
- ../../clusterapi/kustomizeconfig.yaml
```

`config/manager/infrastructure-args-patch.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
  namespace: system
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --leader-elect
        - --health-probe-bind-address=:8081
        - --enable-controller=infrastructure
```

control-plane 側も対称的に `--enable-controller=control-plane` を渡す。

### 4.2 Makefile target

```makefile
CLUSTERCTL_NAME ?= kany8s
IMG ?= ghcr.io/appthrust/kany8s/manager:v0.0.0

.PHONY: clusterapi-manifests
clusterapi-manifests: controller-gen
	mkdir -p config/clusterapi/infrastructure/bases
	mkdir -p config/clusterapi/controlplane/bases
	mkdir -p config/rbac
	# group-scoped RBAC
	$(CONTROLLER_GEN) rbac:roleName=manager-role-infrastructure \
	    paths="./internal/controller/infrastructure/..." \
	    output:stdout > config/rbac/infrastructure-role.yaml
	$(CONTROLLER_GEN) rbac:roleName=manager-role-controlplane \
	    paths="./internal/controller/controlplane/..." \
	    output:stdout > config/rbac/controlplane-role.yaml
	# group-scoped CRDs
	$(CONTROLLER_GEN) crd:generateEmbeddedObjectMeta=true webhook \
	    paths="./api/infrastructure/..." \
	    output:crd:artifacts:config=config/clusterapi/infrastructure/bases
	$(CONTROLLER_GEN) crd:generateEmbeddedObjectMeta=true webhook \
	    paths="./api/v1alpha1/..." \
	    output:crd:artifacts:config=config/clusterapi/controlplane/bases

.PHONY: clusterctl-setup
clusterctl-setup: clusterapi-manifests kustomize
	mkdir -p out/infrastructure-$(CLUSTERCTL_NAME)/v0.0.0
	mkdir -p out/control-plane-$(CLUSTERCTL_NAME)/v0.0.0
	# image 差し替え
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	# infrastructure 用 args patch を一時追加
	cd config/manager && $(KUSTOMIZE) edit add patch --path infrastructure-args-patch.yaml
	$(KUSTOMIZE) build --load-restrictor LoadRestrictionsNone \
	    config/clusterctl/infrastructure \
	    > out/infrastructure-$(CLUSTERCTL_NAME)/v0.0.0/infrastructure-components.yaml
	cd config/manager && $(KUSTOMIZE) edit remove patch --path infrastructure-args-patch.yaml
	# control-plane 用 args patch
	cd config/manager && $(KUSTOMIZE) edit add patch --path controlplane-args-patch.yaml
	$(KUSTOMIZE) build --load-restrictor LoadRestrictionsNone \
	    config/clusterctl/controlplane \
	    > out/control-plane-$(CLUSTERCTL_NAME)/v0.0.0/control-plane-components.yaml
	cd config/manager && $(KUSTOMIZE) edit remove patch --path controlplane-args-patch.yaml
	git restore config/manager/kustomization.yaml
	cp hack/capi/metadata.yaml out/infrastructure-$(CLUSTERCTL_NAME)/v0.0.0/metadata.yaml
	cp hack/capi/metadata.yaml out/control-plane-$(CLUSTERCTL_NAME)/v0.0.0/metadata.yaml
	sed -e 's#%pwd%#'`pwd`'#g' ./hack/capi/config.yaml > capi-local-config.yaml
```

### 4.3 metadata.yaml

`contract:` は **CAPI core の contract** を指す。kany8s は `v1beta1` の `Cluster` を参照する → `v1beta1`。

```yaml
apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3
kind: Metadata
releaseSeries:
# リリースごとに major/minor 降順で追記していく（v0.2 が出たら先頭に足す）
- major: 0
  minor: 1
  contract: v1beta1
```

**運用ルール**: `v0.1.z` が出ている間は 1行のみ。`v0.2.0` を出すときに `major: 0, minor: 2, contract: v1beta1` を**先頭に追加**（降順維持）。release-please ではこの更新を自動化できないため、**MINOR 昇格時のみ手動 PR** とする（AGENTS.md の "CHANGELOG は release CI 専用" ルールと整合、これは metadata.yaml なので対象外）。

### 4.4 clusterctl-config.yaml

**固定バージョン推奨**（`latest` は pre-release を拾えない）:

```yaml
providers:
  - name: "kany8s"
    url: "https://github.com/appthrust/kany8s/releases/download/v0.1.0/infrastructure-components.yaml"
    type: "InfrastructureProvider"
  - name: "kany8s"
    url: "https://github.com/appthrust/kany8s/releases/download/v0.1.0/control-plane-components.yaml"
    type: "ControlPlaneProvider"
```

ドキュメントには「推奨は `download/v0.1.0/`、どうしても latest を使う場合は stable release しか指していないことを確認」と注記。

### 4.5 ControlPlane 3 kind の同梱方針

ユーザ決定: **3 kind すべてを単一 ControlPlaneProvider に同梱**する。

- `config/clusterapi/controlplane/bases/` に以下3つの CRD を配置:
  - `controlplane.cluster.x-k8s.io_kany8scontrolplanes.yaml`
  - `controlplane.cluster.x-k8s.io_kany8scontrolplanetemplates.yaml`
  - `controlplane.cluster.x-k8s.io_kany8skubeadmcontrolplanes.yaml`
- controller-gen の `paths` は `./api/v1alpha1/...`（PROJECT の `path: github.com/reoring/kany8s/api/v1alpha1`）で 3 kind すべてを拾う。
- manager の `--enable-controller=control-plane` で3 reconciler を同時起動。

### 4.6 cert-manager / conversion webhook

- kany8s の現 `config/certmanager/` と `config/webhook/` を `config/clusterctl/<group>/` の resources に含める。
- `config/clusterctl/<group>/patches/convert_webhook.yaml` で CRD に `conversion.strategy=Webhook` の CA injection を注入（capt 同様）。
- **cert-manager 自体はユーザ責任**で事前 install。`INSTALL.md` に「clusterctl init が cert-manager を自動 install する」旨を記載（clusterctl の既定挙動）。

### 4.7 cmd/main.go への `--enable-controller` 実装（Phase 0, 事前作業）

`cmd/main.go` に以下を追加:

```go
var enabledControllers []string
flag.Var(
    (*stringSliceFlag)(&enabledControllers),
    "enable-controller",
    "Which controller group to enable: infrastructure, control-plane. Can be repeated.",
)
```

`Reconciler.SetupWithManager` 呼び出しを `enabledControllers` でゲート:

```go
enabled := toSet(enabledControllers)
if len(enabled) == 0 || enabled["infrastructure"] {
    // register infrastructure controllers
}
if len(enabled) == 0 || enabled["control-plane"] {
    // register control plane controllers
}
```

- 未指定時は従来通り**全 controller 有効**（後方互換 shim は禁止ルールだが、v0 未リリースなので追加・削除いずれも自由）。
- 既定は全部有効にして、clusterctl overlay では必ず明示 flag を渡すことで Pod 1本あたり 1 group だけ走るようにする。

### 4.8 GitHub Actions

#### `.github/workflows/release.yml`（新規）

トリガ: `push: tags: ['v*']`。`workflow_dispatch`（`inputs.dry_run: true` で push 無効化）も受け付ける。

```yaml
concurrency:
  group: release-${{ github.ref }}
  cancel-in-progress: false
```

Jobs:
1. **`build-images`**
   - strategy matrix: `{ component: [manager, eks-kubeconfig-rotator, eks-karpenter-bootstrapper], arch: [amd64, arm64] }`
   - `runs-on`: `ubuntu-24.04`（amd64） / `ubuntu-24.04-arm`（arm64, native runner）
   - `actions/setup-go@v5` + `go-version-file: go.mod`（**ハードコードしない**）
   - `docker/build-push-action@v6` で `ghcr.io/appthrust/kany8s/<component>:${TAG}-<arch>` を push
2. **`create-manifest`** (`needs: build-images`)
   - 3 components それぞれに `docker manifest create` で multi-arch manifest list を生成
3. **`create-release`**
   - `softprops/action-gh-release@v2` を採用（2026 時点で主流）。`draft: true`、`prerelease: ${{ contains(github.ref, '-') }}`。
4. **`upload-assets`** (`needs: [create-manifest, create-release]`)
   - `make clusterctl-setup IMG=ghcr.io/appthrust/kany8s/manager:${TAG}`
   - アセット5点を Release に attach:
     - `hack/capi/metadata.yaml`
     - `out/infrastructure-kany8s/v0.0.0/infrastructure-components.yaml`
     - `out/control-plane-kany8s/v0.0.0/control-plane-components.yaml`
     - `templates/cluster-template.yaml`
     - `clusterctl-config.yaml`
5. **`publish-release`** (`needs: upload-assets`)
   - draft を publish に昇格（安定版のみ、pre-release は draft のまま残す運用も可）。

Permissions:
- `contents: write` / `packages: write` / `id-token: write`（将来の OIDC image signing 用に予約）。

**dry-run 分岐**:
```yaml
- uses: docker/build-push-action@v6
  with:
    push: ${{ !(inputs.dry_run == true) }}
```

#### `.github/workflows/release-please.yml`（新規）

トリガ: `push: branches: [main]`。

```yaml
jobs:
  release-please:
    runs-on: ubuntu-latest
    permissions:
      contents: write
      pull-requests: write
    steps:
      - uses: googleapis/release-please-action@v4
        with:
          release-type: simple   # VERSION + CHANGELOG.md 管理（Go module semver は未使用）
          token: ${{ secrets.RELEASE_PLEASE_TOKEN }}  # ← GITHUB_TOKEN では release.yml が発火しない
```

**トークン要件（致命的）**:
- `GITHUB_TOKEN` が push した tag は **downstream workflow（release.yml）をトリガしない**（GitHub 仕様）。
- **必須アクション**: `secrets.RELEASE_PLEASE_TOKEN` に以下のいずれかを設定:
  - **GitHub App のトークン**（推奨、`tibdex/github-app-token` or `actions/create-github-app-token` で生成）
  - **`workflow` scope を持つ PAT**（運用が古い／個人アカウント依存）
- README / RELEASE.md に「GitHub App を appthrust org に install し、`RELEASE_PLEASE_TOKEN` secret を設定する」手順を記載。

設定ファイル:
- `.release-please-manifest.json`: `{"\.": "0.0.0"}` 初期値
- `release-please-config.json`: `release-type: simple` / `packages."\."` の定義 / `extra-files: ["VERSION"]`

**重要**: `CHANGELOG.md` は release-please のみが書き換える。手動編集禁止（プロジェクト CLAUDE.md 規約）。

#### `.github/workflows/smoke-test.yml`（新規）

トリガ: `pull_request` / `push: branches: [main]`。

- kind cluster 作成
- `make clusterctl-setup IMG=ghcr.io/appthrust/kany8s/manager:dev`
- ローカル build image を `kind load docker-image` で注入
- `clusterctl init --config capi-local-config.yaml --infrastructure kany8s --control-plane kany8s`
- `kubectl wait --for=condition=Available deployment -n kany8s-system --all --timeout=300s`

**branch protection**: `main` への merge 条件として以下 status check を required に設定:
- `ci` (lint + test)
- `smoke-test`

これで smoke が落ちた状態で release-please の PR が main に入ることを防ぐ。

#### 既存の `.github/workflows/ci.yaml` / `lint.yml` / `test.yml`

- `ci.yaml`: lint + test を続投。Go version は `go-version-file: go.mod` に統一。
- `test-e2e.yml`: smoke-test.yml と重複する部分があれば統合を検討（scope 外、既存維持可）。

### 4.9 cluster-api-operator 対応（検証は Phase 5 で実施）

ユーザ指示: **まずリリースができてから検証**。ここでは使用例の CR 雛形のみ記載。

```yaml
apiVersion: operator.cluster.x-k8s.io/v1alpha2
kind: InfrastructureProvider
metadata:
  name: kany8s
  namespace: kany8s-infrastructure-system
spec:
  deployment:
    tolerations:
      - key: "cni.istio.io/not-ready"
        operator: "Exists"
        effect: "NoSchedule"
  fetchConfig:
    url: "https://github.com/appthrust/kany8s/releases/v0.1.0/infrastructure-components.yaml"
  version: v0.1.0
---
apiVersion: operator.cluster.x-k8s.io/v1alpha2
kind: ControlPlaneProvider
metadata:
  name: kany8s
  namespace: kany8s-control-plane-system
spec:
  fetchConfig:
    url: "https://github.com/appthrust/kany8s/releases/v0.1.0/control-plane-components.yaml"
  version: v0.1.0
```

> URL に `/download/` が入らないのは cluster-api-operator の fetchConfig.url が独自に GitHub Release API を解決するため（ユーザ提供の kubevirt 例と同形式）。

## 5. 実装フェーズ分割

### Phase 0 — 事前作業（CI と独立、先行 merge 可）

- `cmd/main.go` に `--enable-controller=<infrastructure|control-plane>` フラグを追加（§4.7）。
- `Makefile` の `clusterapi-manifests` target を追加（group-scoped RBAC / CRD を生成）。
- **検証**: `make clusterapi-manifests` が `config/rbac/{infrastructure,controlplane}-role.yaml` と `config/clusterapi/{infrastructure,controlplane}/bases/*.yaml` を生成することを確認。

### Phase 1 — clusterctl 契約アセット

- `config/clusterapi/{infrastructure,controlplane}/kustomization.yaml`（CRDs only）
- `config/clusterctl/{infrastructure,controlplane}/kustomization.yaml`（composition）
- `config/clusterctl/{infrastructure,controlplane}/rbac/kustomization.yaml`
- `config/manager/{infrastructure,controlplane}-args-patch.yaml`
- `config/clusterctl/<group>/patches/convert_webhook.yaml`（cert-manager CA injection）
- `hack/capi/metadata.yaml`（`contract: v1beta1`）
- `hack/capi/config.yaml`（ローカル clusterctl 用）
- `templates/cluster-template.yaml`（kubeadm + kro の両 backend sample）
- `clusterctl-config.yaml`（ユーザ向け、固定バージョン URL）
- `Makefile` に `clusterctl-setup` target 追加（§4.2）
- **検証**: 手元 kind で `make clusterctl-setup && clusterctl init --config capi-local-config.yaml --infrastructure kany8s --control-plane kany8s` が成功し、`kany8s-system` に2つの manager Deployment (infra / control-plane) が Ready。

### Phase 2 — Release workflow（手動タグ）

- `.github/workflows/release.yml` を §4.8 の仕様で作成
- 3 images × 2 arch matrix build、階層命名 `ghcr.io/appthrust/kany8s/<component>`
- `workflow_dispatch: inputs.dry_run` 分岐を実装
- `concurrency` guard
- **検証**: `workflow_dispatch + dry_run=true` で push 無しの build を確認 → 手動で `v0.1.0-rc1` タグを push → draft Release にアセット5点が attach される。GHCR 3 package が Public になっていることを `gh api` で確認。

### Phase 3 — Auto-tagging（release-please）

- `.github/workflows/release-please.yml` を §4.8 の仕様で作成
- `RELEASE_PLEASE_TOKEN` secret（GitHub App 推奨）を org/repo に設定
- `release-please-config.json` / `.release-please-manifest.json` / `VERSION` を root に配置
- `RELEASE.md` に GitHub App install 手順を明記
- **検証**: `feat: xxx` commit を main に push → Release PR が自動生成 → PR を merge → tag `v0.1.0` が push される → `release.yml` が自動発火する（**ここで RELEASE_PLEASE_TOKEN が GITHUB_TOKEN のままだと発火しないので要確認**）。

### Phase 4 — Smoke test gate

- `.github/workflows/smoke-test.yml` 作成
- `main` branch protection で `ci` + `smoke-test` を required checks に設定
- **検証**: smoke を意図的に壊した PR が merge できないことを確認。

### Phase 5 — cluster-api-operator 検証

- 別クラスタに `cluster-api-operator` を install
- §4.9 の `InfrastructureProvider` / `ControlPlaneProvider` CR を apply
- Pod Ready まで確認、`INSTALL.md` に手順を反映
- この段階では **Phase 2-4 の手動リリースで既に GitHub Release 資産が存在する前提**（ユーザ指示どおり、検証はリリース後で OK）。

## 6. 受け入れ基準

- [ ] `make clusterapi-manifests` で group-scoped RBAC + CRD が生成される。
- [ ] `make clusterctl-setup IMG=...` で `out/` に `infrastructure-components.yaml` / `control-plane-components.yaml` / `metadata.yaml` が生成される。
- [ ] 生成された components.yaml に **CRDs + RBAC + 2 ServiceAccount + 2 Deployment + Webhook 設定** が全て含まれる（kustomize build の出力を `yq 'keys'` で確認）。
- [ ] `clusterctl init --config capi-local-config.yaml --infrastructure kany8s --control-plane kany8s` がローカル kind で成功、`kany8s-system` に 2 manager Pod が起動（infra / control-plane 分離）。
- [ ] `workflow_dispatch + dry_run=true` で build が通る。
- [ ] `v0.1.0-rc1` 手動タグ push で `release.yml` が draft Release を作成し、アセット5点と 3 multi-arch image が push される。
- [ ] GHCR 3 package が Public visibility（`gh api /user/packages/...` または UI 設定）。
- [ ] main に `feat:` commit を merge → release-please PR 自動生成 → PR merge → tag push → `release.yml` 自動発火（`RELEASE_PLEASE_TOKEN` 設定済）。
- [ ] smoke-test が main branch protection で required checks になっている。
- [ ] `InfrastructureProvider` / `ControlPlaneProvider` CR で cluster-api-operator が Pod を Ready にする（Phase 5）。

## 7. 未決事項（全体方針は v2 で決定済、細部のみ残）

1. **初期タグ**: `v0.1.0-rc1` → `v0.1.0` の順で出す（推奨）。release-please config の初期 version は `0.0.0` にして、最初の feat commit で `0.1.0` に bump させる運用。
2. **RELEASE_PLEASE_TOKEN の実体**: GitHub App（推奨） or Fine-grained PAT（workflow scope）。appthrust org で既存の bot app を流用できるかは要確認。
3. **`workflow_dispatch + dry_run`**: Phase 2 の smoke で「push 無しで build だけ回す」ために必須。実装時にスコープ検討。
4. **softprops/action-gh-release vs shogo82148**: v2 では **softprops/action-gh-release@v2** を採用（2026 時点で主流・メンテ活発）。
5. **Helm chart**: kany8s に現在 `charts/` は無い。将来 plugin 用 chart を作る場合に備え、`release-helm.yml` スケルトンだけ本 plan の対象外で残す。

## 8. 参照

- capt v0.4.9: https://github.com/appthrust/capt/releases/tag/v0.4.9
- capt の Makefile `clusterctl-setup` target: `/srv/platform/refs/capt` (tag v0.4.9) `Makefile:83-105`
- capt の `config/clusterctl/infrastructure/kustomization.yaml`: 2 層 composition の実例
- clusterctl provider contract: https://cluster-api.sigs.k8s.io/developer/providers/contracts
- cluster-api-operator: https://github.com/kubernetes-sigs/cluster-api-operator
- release-please: https://github.com/googleapis/release-please
- softprops/action-gh-release: https://github.com/softprops/action-gh-release
