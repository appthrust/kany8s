---
title: EKS プラグイン Helm chart 提供プラン（cluster-api-operator から EKS を使えるようにする）
date: 2026-04-22
owner: kahirokunn
status: draft
revision: 2
changelog:
  - "v2 (2026-04-22): image フィールドを Bitnami 流の registry/repository/tag/digest 分離形に変更。global.imageRegistry / imagePullSecrets を追加。_helpers.tpl での image 合成ロジック、上書きユースケース例、受け入れ基準を追加。"
references:
  - plans/2026-04-22-kany8s-release-ci.md      # §7-5「charts/ は future work」と明記された後続タスク
  - /srv/platform/refs/capt/charts/capt         # 参考: manager 用 Helm chart の構成
  - /srv/platform/refs/capt/.github/workflows/release-helm.yml  # 参考: GHCR OCI push
  - https://github.com/kubernetes-sigs/cluster-api-operator
  - https://cluster-api.sigs.k8s.io/developer/providers/contracts
---

# EKS プラグイン Helm chart 提供プラン v1

## 1. 背景・ゴール

### 1.1 なぜ必要か

- リリース CI（`plans/2026-04-22-kany8s-release-ci.md` が完了済 or 進行中）で、
  `clusterctl` / `cluster-api-operator` から `kany8s` provider を install 可能になる。
- ただし **clusterctl は CAPI provider の manager（InfrastructureProvider / ControlPlaneProvider）しか入れない**。
  EKS 固有の付随コンポーネント（`eks-kubeconfig-rotator`, `eks-karpenter-bootstrapper`）は
  **別途デプロイが必要** — これが無いと、
  - kubeconfig Secret が短命 token で期限切れ → `Cluster Available=True` にならない（rotator 未稼働）
  - Karpenter の bootstrap（IAM Role / OIDC provider / SecurityGroup / Helm install）が走らない
  という状態に陥り、実質 EKS を動かせない。
- 現状これらは `config/eks-plugin/` / `config/eks-karpenter-bootstrapper/` の kustomize オーバーレイしかなく、
  cluster-api-operator ユーザには不親切（kustomize を別途 apply する必要、image タグ合わせ手動）。

### 1.2 ゴール

- `charts/eks-kubeconfig-rotator/` と `charts/eks-karpenter-bootstrapper/` の **Helm chart を追加**し、
  `ghcr.io/appthrust/charts/<chart-name>` に OCI push する。
- コンテナイメージは **既に release CI で GHCR に push 済み**
  (`ghcr.io/appthrust/kany8s/eks-kubeconfig-rotator`, `ghcr.io/appthrust/kany8s/eks-karpenter-bootstrapper` — `.github/workflows/release.yml:85-94,132-133`)
  なので、chart 側の `appVersion` をそのタグに同期させるのみ。
- ユーザは `helm install` 1 発で EKS プラグインを入れられる。
  `cluster-api-operator` の manifest は **触らない**（operator は CAPI provider のみを扱う責務）。
- AWS 認証は **Secret volume mount（開発/kind）** と **IRSA（本番）** の両対応を values で切り替える。

### 1.3 明示的に非ゴール

- **cluster-api-operator への統合**: EKS プラグインを `InfrastructureProvider.spec.additionalDeployments` に
  詰める案は取らない。operator の責務外。ユーザドキュメントで「別途 Helm で入れてください」と誘導する。
- **ACK（AWS Controllers for Kubernetes）の chart 化**: ACK は upstream の既存 chart
  （`oci://public.ecr.aws/aws-controllers-k8s/*-chart`）を使う。本 plan の対象外。
- **Karpenter 本体の chart 化**: upstream `oci://public.ecr.aws/karpenter/karpenter` を使う
  （bootstrapper は Flux OCIRepository + HelmRelease で上記を参照）。本 plan は bootstrapper manager 自身の chart のみ。
- **新機能の追加**: 現在 `config/eks-*/` に存在する挙動をそのまま Helm で再現するだけ。
  IRSA サポートは既存の AWS_SHARED_CREDENTIALS_FILE env を置換可能にするだけで、controller ロジックは変更しない。

## 2. 現状との差分

| 項目 | 現状 | 必要アクション |
|---|---|---|
| `charts/` dir | 未作成 | 新規 2 chart |
| コンテナイメージ | `ghcr.io/appthrust/kany8s/eks-{kubeconfig-rotator,karpenter-bootstrapper}:vX.Y.Z` が release.yml で既に push | そのまま参照 |
| `config/eks-plugin/` | kustomize で `ack-system` ns に deploy | chart が upstream 情報源。kustomize は**削除せず残す**（開発/ACK 併用向け） |
| `config/eks-karpenter-bootstrapper/` | 同上 | 同上 |
| AWS 認証 | `AWS_SHARED_CREDENTIALS_FILE=/aws/credentials` + `aws-creds` Secret volume のみ | values で `aws.mode: staticSecret \| irsa \| podIdentity` を選択可能に |
| Namespace | `ack-system` ハードコード | values `namespace` を指定可能に。default は後述の "kany8s-eks-system"（§4.4 で議論） |
| Leader election ID | kubeconfig-rotator: `f6b95f95.cluster.x-k8s.io`, bootstrapper: `8b9ae7d0.cluster.x-k8s.io`（`cmd/*/main.go` ハードコード） | そのまま（chart では変更不可） |
| リリース方式 | なし | `release-helm.yml` を新規追加、`main` push で GHCR に OCI push |

## 3. 参考にした先行例

### 3.1 capt の `charts/capt/` 構成

```
charts/capt/
├── Chart.yaml            # version, appVersion, maintainers
├── values.yaml           # image.repository/tag, replicas, RBAC 有無
└── templates/
    ├── _helpers.tpl
    ├── deployment.yaml
    ├── manager-rbac.yaml
    ├── leader-election-rbac.yaml
    ├── metrics-auth-rbac.yaml
    ├── <crd>-crd.yaml
    ├── <crd>-editor-rbac.yaml
    └── <crd>-viewer-rbac.yaml
```

本プランは **CRD を含まない**（eks プラグインは CRD を持たない controller）ため、
`templates/` は deployment + RBAC + leader-election の 4 種のみ。

### 3.2 capt の `release-helm.yml` パターン

- `main` push で `charts/*/Chart.yaml` を走査
- `yq` で name/version を抽出 → matrix
- `helm pull` で**既存バージョン存在チェック**（version 未bumpの再 push を防止）
- 無ければ `helm push` で `oci://ghcr.io/appthrust/charts/<name>:<version>`

本プランも同形式を流用。ただし後述の理由（§6.2）で **tag push トリガ** にする案も併記。

## 4. 設計

### 4.1 chart layout（最終形）

```
charts/
├── eks-kubeconfig-rotator/
│   ├── Chart.yaml
│   ├── values.yaml
│   ├── values.schema.json         # optional: 設定ミス検知
│   ├── README.md                  # 使い方（`helm show readme` で引ける）
│   └── templates/
│       ├── _helpers.tpl
│       ├── NOTES.txt
│       ├── namespace.yaml         # {{- if .Values.createNamespace }}
│       ├── serviceaccount.yaml
│       ├── clusterrole.yaml
│       ├── clusterrolebinding.yaml
│       ├── leader-election-role.yaml
│       ├── leader-election-rolebinding.yaml
│       └── deployment.yaml
└── eks-karpenter-bootstrapper/
    ├── Chart.yaml
    ├── values.yaml
    ├── values.schema.json
    ├── README.md
    └── templates/
        ├── _helpers.tpl
        ├── NOTES.txt
        ├── namespace.yaml
        ├── serviceaccount.yaml
        ├── clusterrole.yaml
        ├── clusterrolebinding.yaml
        ├── leader-election-role.yaml
        ├── leader-election-rolebinding.yaml
        └── deployment.yaml
```

### 4.2 Chart.yaml の version 方針

```yaml
# charts/eks-kubeconfig-rotator/Chart.yaml
apiVersion: v2
name: eks-kubeconfig-rotator
description: Kany8s EKS kubeconfig rotator — rotates short-lived EKS tokens into the CAPI kubeconfig Secret.
type: application
version: 0.1.0          # chart SemVer（chart 側の構造変更で bump）
appVersion: "v0.1.0"    # manager image タグ（kany8s release と同期）
home: https://github.com/appthrust/kany8s
sources:
  - https://github.com/appthrust/kany8s
maintainers:
  - name: appthrust
    url: https://github.com/appthrust
keywords:
  - kubernetes
  - cluster-api
  - eks
  - aws
  - kany8s
icon: ""  # 将来的に設定（optional）
```

**同期ポリシー**:
- `appVersion` は kany8s のリリースタグに追従（`v0.1.0` → `v0.1.1` → …）。
- `version`（chart SemVer）は **chart 構造が変わった時のみ bump**。manager image だけ変わった場合は
  patch 桁を bump（`0.1.0` → `0.1.1`）して OCI 再 push。
- release-please の `extra-files` に両 `Chart.yaml` を加え、**manager と完全同期で bump** する案も検討（§6.3）。
  決定が付かないうちは「手動 bump + CI で既存 version チェック」で十分。

### 4.3 values.yaml（eks-kubeconfig-rotator の例）

```yaml
# chart 全体
nameOverride: ""
fullnameOverride: ""
createNamespace: true         # false なら外部で作成済み前提
namespace: kany8s-eks-system  # §4.4 参照

# image（Bitnami 流 — registry / repository / tag / digest を個別指定可能）
image:
  registry: ghcr.io                              # private mirror に差し替える想定の主目的
  repository: appthrust/kany8s/eks-kubeconfig-rotator
  tag: ""                                        # 空なら Chart.appVersion にフォールバック（_helpers.tpl で解決）
  digest: ""                                     # 空でない場合 tag より優先（sha256:... を直接固定したい時用）
  pullPolicy: IfNotPresent
# registry 全体認証（registry だけ上書きする場合も同じ secret を使い回せる）
imagePullSecrets: []                             # 例: [ { name: my-registry-creds } ]
# 全 chart 共通で registry をまとめて上書きしたい場合に使う（Bitnami の global.imageRegistry 互換）
global:
  imageRegistry: ""                              # 非空なら image.registry を無視して優先
  imagePullSecrets: []                           # imagePullSecrets とマージして Deployment に渡る

# deployment
replicas: 1
podAnnotations: {}
podLabels: {}
resources:
  limits:
    cpu: 500m
    memory: 256Mi
  requests:
    cpu: 10m
    memory: 64Mi
nodeSelector: {}
tolerations: []
affinity: {}
priorityClassName: ""

podSecurityContext:
  runAsNonRoot: true
securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: [ALL]

# manager args（cmd/eks-kubeconfig-rotator/main.go の flag と対応）
args:
  leaderElect: true
  metricsBindAddress: "0"    # "0" で disabled
  metricsSecure: false
  healthProbeBindAddress: ":8081"
  refreshBefore: "5m"
  maxRefreshInterval: "10m"
  failureBackoff: "30s"
  watchNamespace: ""         # 空で all-namespaces

# AWS 認証モード（排他）
aws:
  mode: staticSecret   # staticSecret | irsa | podIdentity
  # mode=staticSecret 時のみ参照
  staticSecret:
    secretName: aws-creds
    mountPath: /aws
    envFilePath: /aws/credentials
    optional: true
  # mode=irsa 時のみ参照（ServiceAccount に annotation 付与）
  irsa:
    roleArn: ""        # arn:aws:iam::<acct>:role/<role>
  # mode=podIdentity 時のみ参照（AWS ドキュメント準拠、ラベルは不要）
  podIdentity: {}
  # 共通の AWS env（任意）
  region: ""           # 空で DefaultRegion 依存

# RBAC
rbac:
  create: true

# ServiceAccount
serviceAccount:
  create: true
  name: ""             # 空で fullname
  annotations: {}      # irsa mode では自動で eks.amazonaws.com/role-arn が入る
```

**eks-karpenter-bootstrapper の values** は上記をベースに以下を差し替え:

```yaml
image:
  registry: ghcr.io
  repository: appthrust/kany8s/eks-karpenter-bootstrapper
  tag: ""
  digest: ""
  pullPolicy: IfNotPresent
args:
  failureBackoff: "30s"
  steadyStateRequeue: "10m"
  karpenterChartVersion: ""   # 空なら controller default
```

### 4.3.1 image 解決ロジック（`_helpers.tpl`）

3 つのフィールド（registry, repository, tag/digest）を独立に上書きできるよう、
**Deployment template では 1 変数（`.Values.image.*`）を直接参照せず、helper で合成**する。

```gotemplate
{{/*
Compose full image reference from registry / repository / tag / digest.
Precedence:
  - .Values.global.imageRegistry (if non-empty) overrides .Values.image.registry
  - digest (if non-empty) overrides tag
  - tag falls back to .Chart.AppVersion when empty
*/}}
{{- define "kany8s-eks.image" -}}
{{- $registry := .Values.image.registry -}}
{{- if .Values.global -}}
  {{- if .Values.global.imageRegistry -}}
    {{- $registry = .Values.global.imageRegistry -}}
  {{- end -}}
{{- end -}}
{{- $repository := .Values.image.repository -}}
{{- if .Values.image.digest -}}
  {{- printf "%s/%s@%s" $registry $repository .Values.image.digest -}}
{{- else -}}
  {{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
  {{- printf "%s/%s:%s" $registry $repository $tag -}}
{{- end -}}
{{- end -}}

{{/* Merge global + local imagePullSecrets, dedupe by .name. */}}
{{- define "kany8s-eks.imagePullSecrets" -}}
{{- $merged := list -}}
{{- if .Values.global -}}
  {{- range .Values.global.imagePullSecrets -}}
    {{- $merged = append $merged . -}}
  {{- end -}}
{{- end -}}
{{- range .Values.imagePullSecrets -}}
  {{- $merged = append $merged . -}}
{{- end -}}
{{- if $merged -}}
imagePullSecrets:
{{ toYaml $merged | indent 2 }}
{{- end -}}
{{- end -}}
```

Deployment 側:

```yaml
spec:
  template:
    spec:
      {{- include "kany8s-eks.imagePullSecrets" . | nindent 6 }}
      containers:
        - name: manager
          image: {{ include "kany8s-eks.image" . | quote }}
          imagePullPolicy: {{ .Values.image.pullPolicy }}
```

### 4.3.2 上書きユースケース（README に掲載する例）

```bash
# (1) tag だけ上書き（デフォルト registry = ghcr.io / repository = appthrust/... を使う）
helm install rotator oci://ghcr.io/appthrust/charts/eks-kubeconfig-rotator \
  --version 0.1.0 \
  --set image.tag=v0.1.1

# (2) private mirror に丸ごと差し替え（registry だけ上書き — repository path は GHCR と同じ構造でミラー済の前提）
helm install rotator oci://ghcr.io/appthrust/charts/eks-kubeconfig-rotator \
  --version 0.1.0 \
  --set image.registry=my-registry.example.com \
  --set imagePullSecrets[0].name=my-registry-creds

# (3) エアギャップ / 完全カスタム（registry + repository + tag を独立に指定）
helm install rotator oci://ghcr.io/appthrust/charts/eks-kubeconfig-rotator \
  --version 0.1.0 \
  --set image.registry=registry.internal \
  --set image.repository=platform/kany8s-eks-rotator \
  --set image.tag=2026.04.22-internal

# (4) digest 固定（tag を無視して immutable 指定）
helm install rotator oci://ghcr.io/appthrust/charts/eks-kubeconfig-rotator \
  --version 0.1.0 \
  --set image.digest=sha256:abc123...

# (5) 複数 chart にまたがって registry をまとめて上書き（global.imageRegistry）
helm install rotator oci://ghcr.io/appthrust/charts/eks-kubeconfig-rotator \
  --version 0.1.0 \
  --set global.imageRegistry=my-registry.example.com
```

### 4.4 namespace の決定（重要議論点）

**現状**: 両 kustomize とも `ack-system` にデプロイされている（ACK controller と同居）。

**論点**: chart default を何にするか。

| 候補 | pros | cons |
|---|---|---|
| A. `ack-system` 維持 | 既存 kustomize と一致、docs 変更不要、ACK 前提なので resource lookup しやすい | 「EKS プラグインを ACK に相乗りさせる」という責務混在 |
| B. `kany8s-eks-system` | 責務が明確（kany8s の EKS plugin 専用）、operator が作る `kany8s-system` とも対称 | 既存 `config/eks-*/` kustomize と分離、aws-creds Secret を別 ns に作り直す必要 |
| C. `kany8s-system` に集約 | operator と同居で install 手数が最小 | operator namespace を共有する派生物になり、権限境界が曖昧 |

**推奨: B（`kany8s-eks-system`）**。理由:
- 「kany8s の EKS プラグインをどこに入れたか」を ns 名で自明にできる。
- `kany8s-system` との対称性 — ACK とも分離し、ACK を使わない構成でも違和感が無い。
- Helm は `createNamespace: true` で自動作成可能、Secret 再作成も `helm install --set aws.staticSecret.secretName=...` で吸収できる。

**決定事項（revision 1 時点、未確定）**: B を default にしつつ、`values.namespace` で override 可能とする。
現状の kustomize は **そのまま ack-system 維持**（破壊的変更を避ける）。ただし docs には
「新規は chart、既存の ack-system は kustomize」という二重路線を注記する。

### 4.5 AWS 認証モード詳細

#### 4.5.1 `staticSecret`（kind / dev）

現状の kustomize と同じ。`aws-creds` Secret を volume mount し `AWS_SHARED_CREDENTIALS_FILE` を設定。

```yaml
# deployment.yaml（抜粋）
{{- if eq .Values.aws.mode "staticSecret" }}
env:
  - name: AWS_SHARED_CREDENTIALS_FILE
    value: {{ .Values.aws.staticSecret.envFilePath }}
volumeMounts:
  - name: aws-creds
    mountPath: {{ .Values.aws.staticSecret.mountPath }}
    readOnly: true
volumes:
  - name: aws-creds
    secret:
      secretName: {{ .Values.aws.staticSecret.secretName }}
      optional: {{ .Values.aws.staticSecret.optional }}
{{- end }}
```

#### 4.5.2 `irsa`（本番推奨）

ServiceAccount annotation `eks.amazonaws.com/role-arn` を付与し、EKS の OIDC provider 経由で
Role assume させる。AWS SDK は `AWS_ROLE_ARN` + `AWS_WEB_IDENTITY_TOKEN_FILE` を自動で拾う。

```yaml
# serviceaccount.yaml
{{- if eq .Values.aws.mode "irsa" }}
metadata:
  annotations:
    eks.amazonaws.com/role-arn: {{ required "aws.irsa.roleArn required in irsa mode" .Values.aws.irsa.roleArn }}
{{- end }}
```

**注意**: この chart を install するクラスタが **IRSA 対応** である必要（= EKS クラスタで `oidc-identity-provider` が有効、
または kind 上で `kind-aws-irsa-example`（refs/kind-aws-irsa-example）相当のセットアップ済）。

#### 4.5.3 `podIdentity`（EKS 2023+ のみ）

Pod Identity Agent 側で ServiceAccount と Role の紐付けを管理する方式。
Deployment 側は追加設定不要（AWS SDK が自動で `AWS_CONTAINER_CREDENTIALS_FULL_URI` を拾う）。
chart としては env/volume を何も注入しないだけでよい。

**注意**: Pod Identity Agent が対象クラスタに install 済であることが前提。
未 install の場合は失敗する旨を README に明記。

#### 4.5.4 バリデーション

`values.schema.json` で `aws.mode` を enum に固定:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "properties": {
    "aws": {
      "properties": {
        "mode": { "enum": ["staticSecret", "irsa", "podIdentity"] }
      },
      "required": ["mode"]
    },
    "image": {
      "type": "object",
      "properties": {
        "registry":   { "type": "string", "minLength": 1 },
        "repository": { "type": "string", "minLength": 1 },
        "tag":        { "type": "string" },
        "digest":     { "type": "string", "pattern": "^$|^sha256:[a-f0-9]{64}$" },
        "pullPolicy": { "enum": ["Always", "IfNotPresent", "Never"] }
      },
      "required": ["registry", "repository"]
    },
    "global": {
      "type": "object",
      "properties": {
        "imageRegistry":    { "type": "string" },
        "imagePullSecrets": { "type": "array", "items": { "type": "object" } }
      }
    }
  }
}
```

### 4.6 RBAC templates

既存 `config/eks-plugin/clusterrole.yaml` と `config/eks-karpenter-bootstrapper/clusterrole.yaml` を
そのまま `templates/clusterrole.yaml` に移植（{{ .Release.Name }} prefix は付けない — ClusterRole は
グローバルスコープなので固定名 `eks-kubeconfig-rotator` / `eks-karpenter-bootstrapper` を維持）。

理由: 同 cluster に複数 release を並べるユースケースは想定外（singleton controller）。
**値の多重 install を防ぐため**、NOTES.txt で「同名 ClusterRole が既にあれば helm が conflict を出す」旨を注記。

**leader-election** の Role/RoleBinding は namespace スコープなので通常通り `{{ include "chart.fullname" . }}` prefix を使う。

### 4.7 release 同期フロー

```
   (main push)
       │
       ▼
release-please.yml ──→ Release PR 生成
       │                │ (PR merge)
       │                ▼
       │          tag vX.Y.Z push
       │                │
       │                ▼
       │          release.yml 発火
       │          ├── 3 image build+push（manager, rotator, bootstrapper）
       │          ├── clusterctl アセット生成
       │          └── GitHub Release 作成
       ▼
release-helm.yml（新規）
  - tag push で発火（= manager image と同タイミング）
  - charts/*/Chart.yaml の appVersion を tag に書き換え
  - helm package + helm push → oci://ghcr.io/appthrust/charts/
```

**tag トリガにする理由**（capt の `main` push 方式と違う）:
- manager image は `release.yml` で tag から build される → chart の `appVersion` が指す image も**その時点で GHCR に存在する必要**がある。
- `main` push で chart を先に push すると、image が未 push なので `helm install` が `ImagePullBackOff` になる。
- **順序保証**: release-helm.yml を release.yml の `needs: create-manifest` に追加する案も可（同 workflow ファイルへ統合）。

### 4.8 dev flow（Zot ローカル push）

`/srv/platform/CLAUDE.md` の `Assets Architecture` 節に従い、開発時は **local Zot** に OCI push:

```bash
helm package charts/eks-kubeconfig-rotator --destination /tmp/charts
helm push /tmp/charts/eks-kubeconfig-rotator-0.1.0.tgz oci://localhost:5001/charts
helm install rotator oci://localhost:5001/charts/eks-kubeconfig-rotator --version 0.1.0 \
  --namespace kany8s-eks-system --create-namespace \
  --set image.tag=dev-latest \
  --set aws.mode=staticSecret
```

Makefile に target 追加:

```makefile
.PHONY: helm-package
helm-package: ## Package all Helm charts to dist/charts/.
	mkdir -p dist/charts
	for chart in charts/*/; do \
	  helm package "$$chart" --destination dist/charts; \
	done

.PHONY: helm-push-local
helm-push-local: helm-package ## Push packaged charts to local Zot registry.
	for pkg in dist/charts/*.tgz; do \
	  helm push "$$pkg" oci://localhost:5001/charts; \
	done

.PHONY: helm-lint
helm-lint: ## Lint all Helm charts (blocking for PR).
	for chart in charts/*/; do \
	  helm lint "$$chart"; \
	done
```

## 5. 実装フェーズ分割

### Phase 0 — 調査 & 合意（先行）

- [ ] §4.4 namespace default を `kany8s-eks-system` にする決定を最終化（A/B/C から選択）。
- [ ] §4.7 release-helm workflow を **独立ファイル** にするか **release.yml に統合** するか決定。
  - 推奨: 独立（release.yml が既に長い / helm push は image push の `needs` で串刺せる）。
- [ ] AWS 認証モード 3 種の priority — 最初は `staticSecret` + `irsa` のみ実装し、`podIdentity` は Phase 4 の follow-up としても OK。
  - 推奨: Phase 1 では 3 種すべて実装（if/else なので増分コストは小さい）。

### Phase 1 — chart 実装（CI と独立、先行 merge 可）

- [ ] `charts/eks-kubeconfig-rotator/` を作成（§4.1 の layout）
- [ ] `charts/eks-karpenter-bootstrapper/` を作成
- [ ] 既存 `config/eks-plugin/` / `config/eks-karpenter-bootstrapper/` の YAML を templates に移植、
      values 化できるフィールド（image, args, namespace, AWS mode）を Helm template 化
- [ ] `values.schema.json` で `aws.mode` enum + `image.repository` 必須を検証
- [ ] Makefile に `helm-lint`, `helm-package`, `helm-push-local` を追加
- [ ] **検証**:
  - `helm lint charts/eks-kubeconfig-rotator` / `helm lint charts/eks-karpenter-bootstrapper` が green
  - `helm template` 出力が現行 `kustomize build config/eks-plugin` と**意味的に等価**（`dyff` で diff）
  - kind に install し、`kubectl logs` で manager が起動・`Reconciler started` が出る
  - `aws.mode=staticSecret` で既存 e2e の smoke と同等動作を確認

### Phase 2 — release-helm workflow

- [ ] `.github/workflows/release-helm.yml` を作成（tag push トリガ）
- [ ] workflow の構造:
  ```yaml
  on:
    push:
      tags: ['v*']
    workflow_dispatch:
      inputs:
        tag: { required: true, type: string }
        dry_run: { default: false, type: boolean }

  jobs:
    push-charts:
      needs: []  # release.yml の create-manifest に依存したい場合は同一 workflow に統合すべき
      # strategy matrix: charts = [eks-kubeconfig-rotator, eks-karpenter-bootstrapper]
      # steps:
      #   - checkout
      #   - yq で Chart.yaml の appVersion を tag に書き換え（git 変更は commit しない、build 時のみ）
      #   - helm pull で既存チェック（重複 push は error ではなく skip）
      #   - helm package → helm push oci://ghcr.io/${{ github.repository_owner }}/charts
  ```
- [ ] `workflow_dispatch + dry_run=true` 対応（`helm push` を skip、package までで止める）
- [ ] release.yml の `needs` ではなく独立 workflow にするか、`release.yml` 内の job として追加するか確定
- [ ] **検証**: `v0.1.0-rc1` push で `ghcr.io/appthrust/charts/eks-kubeconfig-rotator:0.1.0-rc1` が publish される（GHCR UI / `helm pull`）

### Phase 3 — ドキュメント整備

- [ ] `README.md`（root）に "Install EKS plugins via Helm" セクション追加
- [ ] `docs/eks/plugin/README.md` に chart install 手順を追記（従来の kustomize 手順は "Advanced / dev" として残す）
- [ ] `INSTALL.md` または `docs/eks/quickstart.md`（新規）に以下のフローを記載:
  1. `clusterctl init --infrastructure kany8s --control-plane kany8s`
  2. （IRSA 前提なら）IAM Role 作成
  3. `helm install rotator oci://ghcr.io/appthrust/charts/eks-kubeconfig-rotator --version v0.1.0 ...`
  4. `helm install bootstrapper oci://ghcr.io/appthrust/charts/eks-karpenter-bootstrapper --version v0.1.0 ...`
  5. Cluster apply
- [ ] cluster-api-operator 使用時の位置付けを明示（"operator は provider だけ入れる / プラグインは Helm"）

### Phase 4 — (optional) podIdentity と IRSA の e2e 検証

- [ ] `refs/kind-aws-irsa-example` を参考に kind + IRSA の smoke を整備
- [ ] 実 EKS で IRSA mode の動作確認（手動、CI には含めない — cost/認証が重いため）
- [ ] Pod Identity 対応の文書化（EKS でのみ可能、kind では無理）

### Phase 5 — (optional) release-please 連動で appVersion 自動 bump

- [ ] `release-please-config.json` の `extra-files` に両 `Chart.yaml` を追加し、
      `appVersion: "vX.Y.Z"` 行を release-please マーカーで自動書き換え:
  ```yaml
  apiVersion: v2
  name: eks-kubeconfig-rotator
  appVersion: "v0.1.0" # x-release-please-version
  ```
- [ ] chart の `version`（SemVer）も同期 bump する設定（`# x-release-please-version` コメントで検出）
- [ ] **検証**: release-please PR で `Chart.yaml` の `version` / `appVersion` が書き換わる

## 6. 受け入れ基準

- [ ] `helm lint charts/eks-kubeconfig-rotator` / `helm lint charts/eks-karpenter-bootstrapper` が ERROR 0 件
- [ ] `helm template` の出力が、既存 `kustomize build config/eks-plugin` / `config/eks-karpenter-bootstrapper`
      と意味的に等価（ClusterRole rules / Deployment args / ServiceAccount name / leader-election Role が一致）
- [ ] `aws.mode=staticSecret` で kind に install し、Pod が Ready（`kubectl logs` に panic 無し）
- [ ] `aws.mode=irsa` で serviceaccount に `eks.amazonaws.com/role-arn` annotation が入る（`kubectl get sa -o yaml` で確認）
- [ ] `aws.mode=podIdentity` で env/volume が何も注入されない（`helm template` diff）
- [ ] `values.schema.json` により `aws.mode: invalid` が `helm install` 時点で拒否される
- [ ] image 個別上書き検証（全 4 パターンで `helm template` 出力の `image:` フィールドが期待通り）:
  - [ ] default: `ghcr.io/appthrust/kany8s/eks-kubeconfig-rotator:<Chart.appVersion>`
  - [ ] `--set image.tag=v0.1.1` のみ: registry/repository はそのまま、tag のみ差し替わる
  - [ ] `--set image.registry=my-registry.example.com` のみ: repository/tag はそのまま、registry 前置が変わる
  - [ ] `--set image.registry=r --set image.repository=foo/bar --set image.tag=x` 3 者独立: `r/foo/bar:x` になる
  - [ ] `--set image.digest=sha256:<64 hex>`: tag が無視され `@sha256:...` で参照される
  - [ ] `--set global.imageRegistry=m` が `image.registry` より優先される
  - [ ] `values.schema.json` により `image.digest=sha256:INVALID` が拒否される
- [ ] tag push → `release-helm.yml` が自動発火し `oci://ghcr.io/appthrust/charts/eks-{rotator,bootstrapper}:vX.Y.Z` が publish される
- [ ] 同一 tag で workflow を **再実行しても** push 済み chart が上書きされない（`helm pull` で existence チェック）
- [ ] `workflow_dispatch + dry_run=true` で push 無しの package のみ実行できる
- [ ] README に Helm install 手順が記載、`cluster-api-operator` 併用時の注意書きあり

## 7. 未決事項

1. **Phase 0 の namespace default**: A (`ack-system`) / B (`kany8s-eks-system`) / C (`kany8s-system`)。推奨は B。
2. **release-helm.yml を独立 workflow にするか release.yml に統合するか**: 推奨は独立ファイル（release.yml が既に肥大）。
3. **chart SemVer の bump 戦略**: 手動 bump / release-please 自動 bump どちらを default とするか。Phase 1 は手動、Phase 5 で自動化検討。
4. **upstream karpenter chart との重複回避**: bootstrapper は `KarpenterChartTag` flag で upstream chart の tag を override できる
   (`cmd/eks-karpenter-bootstrapper/main.go:56`)。この値を bootstrapper chart の values に出すかどうか。推奨: 出す（`args.karpenterChartVersion` として既に記載済）。
5. **既存 `config/eks-*/` kustomize を残すか**: 残す（dev / ACK 相乗り構成で有用）。ただし README で "Helm chart 推奨" と誘導。
6. **signing**: 将来的に cosign OCI 署名を入れるか。Phase 5+ で検討（release.yml の `id-token: write` は既に予約済）。

## 8. 参照

- 先行 plan: `plans/2026-04-22-kany8s-release-ci.md` §7-5（charts/ は future work）
- 既存 release workflow: `.github/workflows/release.yml` — image は既に GHCR に push 済
- 既存 kustomize（移植元）:
  - `config/eks-plugin/` (`deployment.yaml`, `clusterrole.yaml`, ...)
  - `config/eks-karpenter-bootstrapper/` (同上)
- 参考 chart: `/srv/platform/refs/capt/charts/capt/`
- 参考 workflow: `/srv/platform/refs/capt/.github/workflows/release-helm.yml`
- IRSA 参考: `/srv/platform/refs/kind-aws-irsa-example`
- controller ソース:
  - `cmd/eks-kubeconfig-rotator/main.go` (flags)
  - `cmd/eks-karpenter-bootstrapper/main.go` (flags)
  - `internal/controller/plugin/eks/rotator_controller.go`
  - `internal/controller/plugin/eks/karpenter_bootstrapper_controller.go`
- cluster-api-operator 仕様: https://github.com/kubernetes-sigs/cluster-api-operator
