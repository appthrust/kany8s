# 07. Advanced Topics

## Access Control(RBAC)

Helm install 時の `rbac.mode` で、kro コントローラに付与する権限設計が変わります。

### 1) unrestricted

- kro にクラスタ内の全リソースへのフルアクセスを付与
- 検証用途向け

重要

- `ResourceGraphDefinition` を作れる権限は強力です。unrestricted では「RGD を作れる = 実質 admin 的な操作が可能」と考え、運用設計してください。

### 2) aggregation (本番推奨)

- kro が動作するための最小権限 + aggregated ClusterRole
- `rbac.kro.run/aggregate-to-controller: "true"` ラベル付き ClusterRole を足すことで、kro の権限を段階的に拡張

#### 例: 生成 CRD(kind: Foo) + Deployment + ConfigMap を扱う

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kro:controller:foos
  labels:
    rbac.kro.run/aggregate-to-controller: "true"
rules:
  - apiGroups: ["kro.run"]
    resources: ["foos"]
    verbs: ["*"]
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["*"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["*"]
```

ポイント

- 「kro が reconcile する対象」ごとに権限を明示できる
- 逆に言うと、権限が足りないとインスタンスは正常に reconcile できない

## RGD Chaining (RGD を部品として合成)

RGD chaining は「ある RGD によって生成された CRD のインスタンス」を、別の RGD の `resources.template` として利用する設計です。

### 何ができるか

- 小さく再利用可能な building blocks を作り、それらを上位 API で合成する
- 下位 RGD の status(接続文字列/endpoint 等)を上位 RGD の入力に渡す

### 例(概念)

- `Database` RGD: StatefulSet/Service/Secret を作り `status.connectionString` を出す
- `WebApplication` RGD: Deployment/Service/Ingress を作る
- `FullStackApp` RGD: `Database` と `WebApplication` のインスタンスを resources として持ち、
  `webapp` に `${database.status.connectionString}` を渡す

運用のコツ

- 下位 RGD は “入出力(spec/status)が安定した部品” として設計する
- `readyWhen` を下位の重要な状態に設定し、上位が安全に参照できるようにする
- ネストが深すぎるとデバッグが難しくなるため、適度な粒度で止める

## Controller Tuning

大規模クラスタ/大量インスタンスでの調整ポイント。

### 重要な 2 つの reconcile ループ

1) RGD reconciler

- `ResourceGraphDefinition` の静的解析と CRD 生成

2) Dynamic Controller

- 生成 CRD のインスタンスを reconcile
- 親(インスタンス)と子(管理下リソース)のイベントをまとめて処理

### Helm values で調整できる代表値

```yaml
config:
  resourceGraphDefinitionConcurrentReconciles: 3
  dynamicControllerConcurrentReconciles: 10
  dynamicControllerDefaultResyncPeriod: 36000
  dynamicControllerDefaultQueueMaxRetries: 20
  clientQps: 200
  clientBurst: 300
```

意味

- `*ConcurrentReconciles`: ワーカー数
- `clientQps/clientBurst`: API server へのリクエスト上限
- resync/retry: ドリフト検知や失敗時の再試行挙動に影響

### Rate limiter のフラグ

dynamic controller の queue rate limiter はフラグで調整できます。

- `--dynamic-controller-rate-limiter-min-delay` (default 200ms)
- `--dynamic-controller-rate-limiter-max-delay` (default 1000s)
- `--dynamic-controller-rate-limiter-rate-limit` (default 10)
- `--dynamic-controller-rate-limiter-burst-limit` (default 100)

注意

- v0.7.1 の Helm chart はこれらフラグを values として露出していません。
- 調整する場合は Deployment へのパッチ(例: Kustomize)や chart の拡張が必要になります。

## Controller Metrics

### 取得方法

- デフォルト: `:8078/metrics`
- Helm で `metrics.service.create=true` を指定すると Service を作成可能

Prometheus Operator を使う場合は `ServiceMonitor` の作成も可能です。

```yaml
metrics:
  service:
    create: true
    port: 8080
  serviceMonitor:
    enabled: true
    interval: 1m
```

### 代表的なメトリクス(0.7.1)

Dynamic Controller (ALPHA):

- `dynamic_controller_reconcile_total`
- `dynamic_controller_reconcile_duration_seconds`
- `dynamic_controller_queue_length`
- `dynamic_controller_gvr_count`

Schema Resolver (ALPHA):

- `schema_resolver_cache_hits_total`
- `schema_resolver_cache_misses_total`
- `schema_resolver_api_call_duration_seconds`

controller-runtime / workqueue の標準メトリクスも出ます。

### 監視の観点

- queue length が増え続ける: reconcile が追いついていない(ワーカー不足/外部依存/権限エラーなど)
- schema resolver の miss や API call time が大きい: CRD/スキーマ解決負荷が高い

## 本番運用の勘所(まとめ)

- `rbac.mode=aggregation` を検討し、「RGD 作成権限」と「kro の reconcile 権限」を分離する
- RGD/インスタンスは GitOps で管理し、変更は段階的に rollout する
- RGD の status.conditions を CI でチェックし、Active にならない変更を mainline に入れない
- `readyWhen` を適切に入れて、非同期な status 値を参照する race を潰す
- メトリクスを scrape して、キューや reconcile 時間を監視する
