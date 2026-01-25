# kro examples

`refs/kro/examples/` は、kro(v0.7.1) の機能を「シンプル -> 実務っぽい -> かなり複雑」へ段階的に理解できるようにした例集です。

注意

- 例の中には、ACK / Config Connector / Azure Service Operator など外部コントローラ(= CRD)に依存するものがあります。該当するコントローラがクラスタに入っていない場合、その例の RGD は `ResourceGraphAccepted=False` になります。
- クラウドリソースの作成例は、各コントローラ側の認証設定(IAM/Workload Identity/Managed Identity 等)が完了している前提です。

## 例一覧

1) `refs/kro/examples/01-basic-webapp/`

- Deployment + Service の最小構成
- schema(spec/status) と基本的な CEL 参照

2) `refs/kro/examples/02-webapp-ingress-and-config/`

- `includeWhen` で Ingress をオン/オフ
- `externalRef` + `?` / `.orValue()` で platform 管理 ConfigMap を参照

3) `refs/kro/examples/03-postgres-readywhen/`

- StatefulSet(Postgres) + App を 1 つの RGD で構成
- `readyWhen` で DB の ready を待ってから App を作る

4) `refs/kro/examples/04-rgd-chaining-fullstack/`

- RGD chaining(Database RGD + WebApplication RGD + FullStackApp RGD)
- 部品化した API を上位 API で合成するパターン

5) `refs/kro/examples/05-multi-tenant/`

- Namespace/Quota/NetworkPolicy をテナント単位で作る
- さらに Tenant API で環境+アプリを合成(チーム/テナント分離の雛形)

注意:

- kro v0.7.1(kind) では `NetworkPolicy` を含むグラフが `Ready` にならず、
  この例は `IN_PROGRESS` のまま止まることがあります(詳細は各 README と `kro.md` を参照)

6) `refs/kro/examples/06-multi-cloud-bucket/`

- (難) Bucket という抽象 API を kro で実装
- AWS: ACK S3 Bucket
- GCP: Config Connector StorageBucket
- Azure: Azure Service Operator(StorageAccount + Container)
- `includeWhen` + RGD chaining で provider を切り替える

7) `refs/kro/examples/07-portable-bucket-direct/`

- (中) 1つの RGD で `PortableBucket` を定義し、直接 Managed Resource(ACK/CC/ASO) を作る
- 中間の `AWSBucket`/`GCPBucket` のような RGD を挟まずに「抽象 -> 具象」を 1段で行う

8) `refs/kro/examples/08-bucket-cluster-specific/`

- (難) “クラウドプロバイダを意識させない” Bucket API
- クラスタごとに `Bucket` RGD の実装(裏の Managed Resource)を差し替える
- 開発者のインスタンス YAML は常に同じ(= provider フィールド不要)

## 実行の共通手順(概要)

各ディレクトリの `README.md` に手順を書いています。基本は:

```bash
# RGD を apply
kubectl apply -f rgd.yaml

# RGD が Active になるのを確認
kubectl get rgd

# インスタンスを apply
kubectl apply -f instance.yaml

# インスタンス状態
kubectl get <kind>
kubectl describe <kind> <name>
```
