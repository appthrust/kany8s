# 06-multi-cloud-bucket

Bucket という「抽象 API」を kro で実装する例です。

目的:

- 開発者は `kind: Bucket` だけを作ればよい
- 裏側では provider ごとに別のコントローラ/CRD を使って実リソースを作る
  - AWS: ACK S3 `Bucket` (CRD: `buckets.s3.services.k8s.aws`)
  - GCP: Config Connector `StorageBucket` (CRD: `storagebuckets.storage.cnrm.cloud.google.com`)
  - Azure: Azure Service Operator v2 `StorageAccount` + `StorageAccountsBlobServicesContainer`

kro でやっていること:

- provider ごとの実装を `AWSBucket`/`GCPBucket`/`AzureBucket` という RGD として部品化
- `Bucket` RGD が `includeWhen` で provider を切り替えつつ、RGD chaining で部品を呼び出す

## 前提

この例は CRD が必要です。

- AWS: ACK S3 controller がインストールされている
- GCP: Config Connector がインストールされている
- Azure: Azure Service Operator v2 がインストールされている

いずれもクラウド認証設定(IAM/Workload Identity/Managed Identity 等)が済んでいる前提です。

## 適用順

```bash
# provider-specific RGDs
kubectl apply -f 01-rgd-awsbucket.yaml
kubectl apply -f 02-rgd-gcpbucket.yaml
kubectl apply -f 03-rgd-azurebucket.yaml

# abstraction RGD
kubectl apply -f 04-rgd-bucket.yaml

kubectl get rgd
```

## インスタンス例

- AWS: `05-instance-aws.yaml`
- GCP: `06-instance-gcp.yaml`
- Azure: `07-instance-azure.yaml`

```bash
kubectl apply -f 05-instance-aws.yaml
kubectl get buckets
```

## 実務でのポイント

- `Bucket` のような抽象 API を作ると「アプリ側のマニフェスト」を provider 非依存にできる
- ただし “完全に共通化できない属性” (命名制約、暗号化、public access など)は、spec の設計で折り合いを付ける必要があります
- conditional required(例: provider=gcp のとき projectId 必須)は、CRD の標準バリデーションだけでは表現しづらいです
  - この例では provider-specific RGD 側の schema で `minLength` や `pattern` を使い、足りない値はエラーになりやすいようにしています
