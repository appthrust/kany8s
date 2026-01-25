# 03-postgres-readywhen

Postgres(StatefulSet) とアプリ(Deployment)を 1 つの RGD で作る例です。

学べること:

- `readyWhen` で「DB が ready になるまで」待ってから次のリソースを作る
- `readyWhen` は “自分自身だけ” を参照する制約がある(= `dbStatefulSet.*` だけ)
- DB 側の status を参照することで、App 側に依存関係を作る

## 適用

```bash
kubectl apply -f rgd.yaml
kubectl apply -f instance.yaml

kubectl get rgd postgres-backed-app.kro.run -o wide
kubectl get postgresbackedapps
kubectl get sts,svc,deploy
```

## 既知の挙動(kro v0.7.1)

- bool の status field(例: `dbReady`)が instance status に出力されないケースがあるため、
  この例では `dbReadyReplicas`(integer) を status に出しています。
- それでも不確かな場合は `kubectl get sts <name>-db -o jsonpath='{.status.readyReplicas}'` 等で確認してください。
