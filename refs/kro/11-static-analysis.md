# 11. Static Analysis(静的解析)の仕組み

この章は、RGD 作成時に kro が実施する静的解析を整理します。RGD が Active にならない/想定外の型エラーが出るときの理解に役立ちます。

一次ソース: https://kro.run/docs/concepts/rgd/static-type-checking

## なぜ静的解析が重要か

kro の強みは「インスタンスを作ってから壊れる」のではなく、**RGD を作った時点で壊れている箇所を見つける** ことです。

- CEL の文法ミス
- 存在しないフィールド参照
- 型不一致
- 循環依存
- クラスタに存在しない apiVersion/kind

などを RGD 作成時に弾きます。

## 検証パイプライン(0.7.1)

公式ドキュメント上、RGD reconcile 中に概ね次のステージで検証されます。

### Stage 1: Schema Validation

- `spec.schema` の SimpleSchema を解析
- OpenAPI へ変換
- 生成 CRD の spec として妥当か検証

### Stage 2: Status Schema Inference

- `schema.status` の CEL 式を解析
- 式の戻り値型を推論して、生成 CRD の status OpenAPI schema を生成

### Stage 3: Resource Naming Validation

- `resources[].id` が CEL 識別子として妥当か検証
  - ハイフンなどを排除

### Stage 4: Resource Template Validation

各 `resources[].template` について:

- `apiVersion/kind/metadata` などの基本構造を検証
- API Server から対象リソースの OpenAPI schema を取得
  - built-in リソースだけでなく、クラスタに入っている CRD も対象
- テンプレートの静的値も含め、schema に沿っているか検証

### Stage 5: AST Analysis + Dependency Graph

- `${...}` 内の CEL を AST(Abstract Syntax Tree) にパース
- 参照している識別子/関数/フィールドを抽出
- `schema` と resources の参照関係から DAG を構築
- 循環依存を検知

### Stage 6: Expression Type Checking

- CEL type checker を用いて式を型検証
- 式の戻り値型を推論
- ターゲットフィールド(OpenAPI)の期待型と互換か判定

互換判定には:

- 単純な assignable 判定
- deep structural compatibility(後述)

が使われます。

### Stage 7: Condition Expression Validation

- `includeWhen` / `readyWhen` が boolean を返すか
- 参照制約(`includeWhen` は schema.spec のみ、`readyWhen` は self のみ)に違反していないか

### Stage 8: Activation

全て通ると:

- topological order を計算
- 生成 CRD をクラスタに登録
- インスタンスを扱うハンドラを dynamic controller に登録

## 型互換判定の深掘り(実務上の要点)

### primitives

- `int`/`string`/`bool`/`double` の一致

### lists

- 要素型を再帰的に検証

### maps

- key/value 型を再帰的に検証

### structs (subset semantics)

Kubernetes の多くのスキーマは巨大な struct です。kro は以下のように扱います。

- 式の戻り値が「期待される struct の subset(一部フィールドだけ)」なら OK
- ただし、期待される struct に存在しない “余計なフィールド” を含むと NG

### map/struct 互換

Kubernetes では labels/annotations/data など map と struct を行き来することが多く、kro はこれを考慮します。

ただし、CRD 側の `x-kubernetes-preserve-unknown-fields: true` のような箇所では、
構造が事前に分からず静的検証ができないため、検証は緩くなります。

## PreserveUnknownFields と `?`

次のような “構造がスキーマで確定しない” フィールドは静的検証が難しく、`?` の使用が推奨されます。

- ConfigMap/Secret の `data` (キーが自由)
- CRD の自由形式フィールド

例:

```yaml
value: ${config.data.?DATABASE_URL}
```

## 失敗時の見方

- RGD が reject された場合、`kubectl describe rgd <name>` の `status.conditions[].message` を読む
- メッセージには「どの resource のどのパスで、どの式が、どの型が期待されたか」が含まれる

関連: `refs/kro/10-troubleshooting.md`
