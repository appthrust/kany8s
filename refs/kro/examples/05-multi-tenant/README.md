# 05-multi-tenant

テナント(チーム)単位で Namespace を分離し、Quota/NetworkPolicy を自動付与し、その上にアプリを載せる例です。

学べること:

- cluster-scoped(Namespace) と namespaced(ResourceQuota/NetworkPolicy) を 1 インスタンスで扱う
- Namespace の `status.phase` を `readyWhen` に使って「Active になるまで待つ」
- RGD chaining で Tenant API にまとめる

## 注意

- Namespace/ClusterRole など cluster-scoped リソースを作るので、RBAC(aggregation)運用では kro に権限を追加してください。

## 重要: kro v0.7.1(kind) では Ready にならない

kro v0.7.1 + kind の検証では、`NetworkPolicy` を含むグラフが `Ready` 判定を通らず、
`DemoTenantEnvironment` / `DemoTenant` が `IN_PROGRESS` から進まない現象を確認しています。

症状(例):

- `kubectl get demotenantenvironments` が `STATE=IN_PROGRESS`
- `status.conditions` の `ResourcesReady` が `Unknown(ResourcesInProgress)` のまま

現状は「設計例」として参照し、実行検証には向きません。
詳細は `kro.md` の「`NetworkPolicy` を含む graph は v0.7.1 で Ready にならない」を参照してください。

## 適用順

```bash
kubectl apply -f 01-tenant-environment-rgd.yaml
kubectl apply -f 02-tenant-application-rgd.yaml
kubectl apply -f 03-tenant-rgd.yaml

kubectl get rgd

kubectl apply -f 04-tenant-instance.yaml

kubectl get demotenants
kubectl get demotenantenvironments
kubectl get demotenantapplications
```
