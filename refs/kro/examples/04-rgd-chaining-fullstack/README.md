# 04-rgd-chaining-fullstack

RGD chaining(部品化した RGD を上位 RGD で合成する)の例です。

構成:

- `DemoDatabase` RGD: Postgres(StatefulSet) を提供し、connectionString/ready を status に出す
- `DemoWebApplication` RGD: Deployment/Service/(optional Ingress) を提供し、endpoint を status に出す
- `DemoFullStackApp` RGD: 上の 2 つを resources として利用し、FullStack を 1 API にまとめる

学べること:

- RGD を building blocks として設計する(spec 入力と status 出力を安定させる)
- 親 RGD から子 RGD の status を参照して依存関係を作る
- 親側の `readyWhen` で「子インスタンスが ready になるまで」待つ

## 適用順

生成 CRD が必要なので、下位 RGD -> 上位 RGD の順で apply します。

```bash
kubectl apply -f 01-database-rgd.yaml
kubectl apply -f 02-webapp-rgd.yaml
kubectl apply -f 03-fullstack-rgd.yaml

kubectl get rgd

kubectl apply -f 04-instance.yaml

kubectl get demofullstackapps
kubectl get demodatabases
kubectl get demowebapplications
```
