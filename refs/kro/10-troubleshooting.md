# 10. Troubleshooting / よくある落とし穴

この章は「RGD が Active にならない」「インスタンスが Ready にならない」などの実務トラブルを、最短で切り分けるためのメモです。

## まず見るべきもの

### RGD 側

```bash
kubectl get rgd
kubectl describe rgd <rgd-name>
kubectl get rgd <rgd-name> -o yaml
```

- `status.conditions` の `message` に、静的解析/検証エラーが具体的に出ます。
- `status.topologicalOrder` で kro が推論した作成順を確認できます。

### インスタンス側

```bash
kubectl get <kind>
kubectl describe <kind> <instance>
kubectl get <kind> <instance> -o yaml
```

インスタンスが作った下位リソースの列挙:

```bash
kubectl get all -l kro.run/instance-name=<instance>
```

### kro controller のログ

```bash
kubectl logs -n kro-system deploy/kro
```

## RGD が Active にならない

RGD は作成/更新時に静的解析され、失敗すると `ResourceGraphAccepted=False` になります。

### 1) 参照している apiVersion/kind がクラスタに存在しない

症状:

- `schema not found` / `failed to get schema` のようなエラー

原因:

- 利用する CRD がまだクラスタに入っていない
- apiVersion/kind の typo

対処:

- 先に 해당 CRD をインストールする
- `kubectl api-resources` で apiVersion/kind を確認する

### 2) テンプレートに存在しないフィールドを書いている

症状:

- `schema not found for field ...` / `unknownField` 相当

原因:

- Kubernetes の OpenAPI schema 上存在しないフィールド

対処:

- 正しいフィールド名に修正
- CRD の場合、実際の schema を `kubectl get crd ... -o yaml` で確認

### 3) CEL の参照先フィールドが存在しない

症状:

- `no such member: ...`

原因:

- `${deployment.spec.podReplicas}` のような typo
- `id` の typo (`deployent` など)

対処:

- `resources[].id` と参照式を突き合わせ

### 4) 型不一致

症状:

- `type mismatch ... expected integer` など

原因:

- 例: `Deployment.spec.replicas` に string を渡している

対処:

- 変換が必要なら `string()` などを使用
- schema 側の型定義も含めて整合させる

### 5) id 命名が CEL の識別子として不正

症状:

- `id` が変数名として扱えない (例: `web-server`)

対処:

- `lowerCamelCase` にする

### 6) 循環依存(DAG になっていない)

症状:

- `circular dependency detected` 相当

対処:

- 参照関係を見直し、片方を `schema.spec` から渡すなどして cycle を崩す

## インスタンスが Ready にならない

インスタンスは `status.conditions` の階層で切り分けます。

### 1) `InstanceManaged=False`

方向性:

- finalizer/labels の設定に失敗
- controller の権限不足などでメタ更新に失敗

対処:

- kro ログで `forbidden` が出ていないか確認
- RBAC(aggregation mode) の場合、必要権限を追加

### 2) `GraphResolved=False`

方向性:

- 実行時のテンプレート解決で詰まっている
  - 参照している値が resolvable にならない(ずっと null)
  - 外部参照(externalRef)が存在しない

対処:

- externalRef の参照先が存在するか確認
- `?` を使って null になり得る場合、`.orValue()` でデフォルトを設計
- `readyWhen` の設計を見直し(待ち条件が成立しない)

### 3) `ResourcesReady=False`

方向性:

- 下位リソースのどれかが ready にならない

対処:

- 下位リソースを `kubectl describe` で調査
- `readyWhen` を設定している場合、その条件式が成立するか確認

## RBAC(aggregation) でよく起きること

症状:

- インスタンスは作れるが、下位リソースの create/update が `forbidden` で失敗

対処:

- kro コントローラに必要権限を追加する ClusterRole を作成
- ClusterRole に `rbac.kro.run/aggregate-to-controller: "true"` を付ける

## Argo CD で resource tracking できない

方向性:

- kro は ownerReferences を付けないため、Argo CD の tracking 設定と噛み合わないことがある

対処:

- FAQ の tracking annotation 例を参考に、テンプレートに `argocd.argoproj.io/tracking-id` を付与
- ただし ownerReferences を付ける場合は削除順序/スコープ制約に注意

## 変更時(アップデート)の注意

- Helm upgrade では CRD が自動更新されない。CRD 変更を含むバージョンアップ時は別途 apply が必要。
- RGD の `schema.apiVersion/kind/group` は immutable。変更したい場合は新 RGD と移行手順を用意する。
