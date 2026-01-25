# 06. Instance 運用とライフサイクル

RGD が Active になると、クラスタに新しい API(生成 CRD)が登録されます。
利用者はその API の **インスタンス** を作成し、kro が下位リソース群を管理します。

## インスタンスとは

- 生成 CRD の Custom Resource
- インスタンス 1 つが「リソース群の desired state」の単位

例:

```yaml
apiVersion: kro.run/v1alpha1
kind: Application
metadata:
  name: my-app-instance
spec:
  name: my-app
  replicas: 1
  ingress:
    enabled: true
```

## kro の reconcile 動作(概念)

kro は通常の Kubernetes controller パターンで動きます。

1. Observe: インスタンス/子リソースの変更を watch
2. Compare: 現在状態と desired state を比較
3. Act: create/update/delete を実行
4. Report: status を更新

特徴

- 子リソースが手で変更/削除されても検知して修復する(ドリフト検知)
- 依存グラフ(DAG)に従って順序を守って作成/削除する

## ラベル/アノテーションと所有権

kro はリソース所有関係を「ownerReferences」ではなく **labels + ApplySet** で追跡します。

### インスタンス自体に付くメタデータ(例)

Labels:

- `kro.run/owned: "true"`
- `kro.run/kro-version: <version>`
- `kro.run/resource-graph-definition-id: <RGD UID>`
- `kro.run/resource-graph-definition-name: <RGD name>`
- `app.kubernetes.io/managed-by: kro`
- `applyset.kubernetes.io/id: <hash>`

Annotations:

- `applyset.kubernetes.io/tooling: kro/<version>`
- `applyset.kubernetes.io/contains-group-kinds: ...`
- `applyset.kubernetes.io/additional-namespaces: ...`

### kro が作る下位リソースに付くメタデータ(追加分)

インスタンス labels に加えて、以下が付与されます。

- `kro.run/instance-id: <instance UID>`
- `kro.run/instance-name: <instance name>`
- `kro.run/instance-namespace: <instance namespace>`
- `applyset.kubernetes.io/part-of: <applyset id>`

これにより「どのインスタンスがどのリソースを管理しているか」をラベル検索で追えます。

### ApplySet とは

kro は Kubernetes ApplySet 仕様(KEP-3659: kubectl apply-prune)を利用し、

- インスタンスが管理するリソース集合
- グラフ変更時に不要になったリソースの prune

を実現します。

## ownerReferences をデフォルトで付けない理由

kro はデフォルトで ownerReferences を付けません。

理由:

1) **順序付き削除**

- kro は依存関係に従い「逆トポロジカル順」で削除したい
- ownerReferences の GC は順序保証がない

2) **スコープの制約**

- namespaced resource は cluster-scoped resource を owner にできない
- kro のインスタンスは通常 namespaced だが、RGD は cluster-scoped リソースも作れる(例: Namespace, ClusterRole)

### どうしても ownerReferences が必要な場合

Argo CD など「ownerReferences があると扱いやすい」ツールもあります。その場合はテンプレートで明示できます。

```yaml
resources:
  - id: configmap
    template:
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: ${schema.spec.name}-config
        ownerReferences:
          - apiVersion: ${schema.apiVersion}
            kind: ${schema.kind}
            name: ${schema.metadata.name}
            uid: ${schema.metadata.uid}
            controller: true
            blockOwnerDeletion: true
```

注意

- cross-namespace / cluster-scoped を含むグラフで破綻する可能性があります。
- GC により削除順序が崩れ、依存関係が壊れる可能性があります。

## Argo CD 連携(FAQ 抜粋)

Argo CD の resource tracking のため、RGD の各テンプレートに tracking annotation を付与する例が提示されています。

```yaml
metadata:
  ownerReferences:
    - apiVersion: kro.run/v1alpha1
      kind: ${schema.kind}
      name: ${schema.metadata.name}
      uid: ${schema.metadata.uid}
      blockOwnerDeletion: true
      controller: false
  annotations:
    argocd.argoproj.io/tracking-id: ${schema.metadata.?annotations["argocd.argoproj.io/tracking-id"]}
```

`controller: false` としている点に注意してください(Argo CD 側の運用方針に合わせて調整してください)。

## インスタンス status の読み方

### `kubectl get` での概観

```bash
kubectl get <kind>
```

例:

```
NAME     STATE   READY   AGE
my-app   ACTIVE  True    30s
```

### status の内訳

インスタンス status には以下が混在します。

1) **state** (大まかな状態)

- `ACTIVE`
- `IN_PROGRESS`
- `FAILED`
- `DELETING`
- `ERROR`

2) **conditions** (詳細)

0.7.1 時点では、トップレベル `Ready` とサブ条件が用意されています。

- `InstanceManaged`
- `GraphResolved`
- `ResourcesReady`
- `Ready` (総合)

運用上は `Ready` を主に見て、問題があるときにサブ条件で切り分けます。

3) **RGD で定義した status fields**

例: `${deployment.status.availableReplicas}` を `availableReplicas` に射影する、など。

### observedGeneration

condition の `observedGeneration` が `metadata.generation` に追従しているかを確認すると、
「最新 spec を controller が見ているか」を判断できます。

## デバッグの定番手順

1) インスタンスの Ready condition を確認

```bash
kubectl get <kind> <name> -o jsonpath='{.status.conditions[?(@.type=="Ready")]}'
```

2) `Ready=False` ならサブ条件を見る

- `InstanceManaged=False`: finalizer/labels 周り
- `GraphResolved=False`: テンプレート解決/参照/式の解決
- `ResourcesReady=False`: どれかのリソースが ready にならない

3) describe でイベント/メッセージを見る

```bash
kubectl describe <kind> <name>
```

4) 下位リソースをラベルで列挙

```bash
kubectl get all -l kro.run/instance-name=<name>
kubectl get all -l kro.run/instance-id=<uid>
```

5) kro controller のログを見る

```bash
kubectl logs -n kro-system deploy/kro
```

## 削除時の挙動

- インスタンス削除で、kro は下位リソースを **逆トポロジカル順** で削除します。
- `includeWhen` で skip されたリソース、およびそれに依存して skip されたリソースは作成されないため、削除対象にもなりません。
