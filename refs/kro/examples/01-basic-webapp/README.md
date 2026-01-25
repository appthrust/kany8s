# 01-basic-webapp

Deployment + Service の最小構成です。

学べること:

- `spec.schema` で API を設計する
- `${schema.spec.*}` をテンプレートに埋め込む
- status を下位リソースから射影する

## 適用

```bash
kubectl apply -f rgd.yaml
kubectl get rgd webapp-basic.kro.run -o wide

kubectl apply -f instance.yaml
kubectl get webapps
kubectl get deploy,svc
```
