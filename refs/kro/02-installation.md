# 02. インストール/アップグレード

## インストール方式

kro は以下のいずれかでインストールできます。

- Helm (推奨)
- GitHub Releases の raw manifests (kro が配布するインストール用 YAML)

本ドキュメントでは **kro v0.7.1** を前提に例を書きます。

## 前提

- Kubernetes クラスタ
- `kubectl`
- Helm を使う場合: Helm 3.x

## Helm でインストール

### 最新版(その時点の latest)

```bash
helm install kro oci://registry.k8s.io/kro/charts/kro \
  --namespace kro-system \
  --create-namespace
```

### バージョン固定(再現性のため推奨)

```bash
export KRO_VERSION=0.7.1

helm install kro oci://registry.k8s.io/kro/charts/kro \
  --namespace kro-system \
  --create-namespace \
  --version=${KRO_VERSION}
```

### GitHub の latest release を自動取得してインストール

```bash
export KRO_VERSION=$(curl -sL \
  https://api.github.com/repos/kubernetes-sigs/kro/releases/latest | \
  jq -r '.tag_name | ltrimstr("v")')

echo "KRO_VERSION=${KRO_VERSION}"

helm install kro oci://registry.k8s.io/kro/charts/kro \
  --namespace kro-system \
  --create-namespace \
  --version=${KRO_VERSION}
```

## Raw manifests でインストール

Helm を使わない運用(マニフェスト管理に寄せたい)場合は、リリースアセットの YAML を apply します。

### 変数の準備

```bash
export KRO_VERSION=0.7.1
export KRO_VARIANT=kro-core-install-manifests
```

variant の例:

- `kro-core-install-manifests`
- `kro-core-install-manifests-with-prometheus` (metrics 用の Service/ServiceMonitor を含む)

### apply

```bash
kubectl create namespace kro-system
kubectl apply -f https://github.com/kubernetes-sigs/kro/releases/download/v${KRO_VERSION}/${KRO_VARIANT}.yaml
```

## インストール確認

### Pod

```bash
kubectl get pods -n kro-system
```

### RGD の CRD

```bash
kubectl get crd resourcegraphdefinitions.kro.run
```

### RGD 一覧

```bash
kubectl get resourcegraphdefinitions
# 短縮名
kubectl get rgd
```

## Upgrade

### Helm

```bash
helm upgrade kro oci://registry.k8s.io/kro/charts/kro \
  --namespace kro-system
```

特定バージョンへ:

```bash
export KRO_VERSION=0.7.1

helm upgrade kro oci://registry.k8s.io/kro/charts/kro \
  --namespace kro-system \
  --version=${KRO_VERSION}
```

注意(重要)

- Helm は CRD を自動更新しません。リリースに CRD 変更が含まれる場合、手動で CRD を apply する必要があります。
- 具体的な手順は、リリースノートと、必要なら `kro-core-install-manifests*.yaml` に含まれる CRD を確認してください。

### Raw manifests

```bash
export KRO_VERSION=0.7.1
export KRO_VARIANT=kro-core-install-manifests

kubectl apply -f https://github.com/kubernetes-sigs/kro/releases/download/v${KRO_VERSION}/${KRO_VARIANT}.yaml
```

注意

- `kubectl apply` によるインストールは「古いオブジェクトの削除」を自動ではしません。新バージョンで不要になったオブジェクトが残る場合があります。

## Uninstall

### Helm

```bash
helm uninstall kro -n kro-system
```

### Raw manifests

```bash
kubectl delete -f https://github.com/kubernetes-sigs/kro/releases/download/v${KRO_VERSION}/${KRO_VARIANT}.yaml
```

注意

- これは kro コントローラを消すだけで、既存の RGD やインスタンスは残ります。
- 完全に消す場合は、インスタンスと RGD を先に削除してください。

## Helm values で押さえるべき設定

### RBAC (最重要)

Helm chart には `rbac.mode` があり、2 モードあります。

- `unrestricted` (デフォルト)
  - kro にクラスタ内の全リソースに対するフルアクセスを付与
  - 検証用途には楽だが、本番には非推奨
  - この状態で RGD を作れる人は、実質 cluster-admin に近い影響力を持つ
- `aggregation` (本番推奨)
  - kro 自身が動作するための最小権限 + 「ラベル付き ClusterRole を集約する」方式
  - `rbac.kro.run/aggregate-to-controller: "true"` を付けた ClusterRole のルールが kro に追加される
  - RGD で新しい kind を扱うたびに、kro が必要とする権限を明示的に追加できる

values 例:

```yaml
rbac:
  mode: aggregation
```

`aggregation` の場合、RGD で使うリソース種別の権限を ClusterRole として追加する必要があります(例は `refs/kro/07-advanced-topics.md` を参照)。

### Metrics

Prometheus でメトリクスを scrape したい場合は values で有効化します。

```yaml
metrics:
  service:
    create: true
    port: 8080
  serviceMonitor:
    enabled: true   # Prometheus Operator を使う場合
    interval: 1m
```

### Controller のチューニング

代表的な設定(Helm values の `config.*`)。

- `config.resourceGraphDefinitionConcurrentReconciles`
- `config.dynamicControllerConcurrentReconciles`
- `config.dynamicControllerDefaultResyncPeriod` (秒)
- `config.dynamicControllerDefaultQueueMaxRetries`
- `config.clientQps` / `config.clientBurst`

詳細は `refs/kro/07-advanced-topics.md` にまとめています。
