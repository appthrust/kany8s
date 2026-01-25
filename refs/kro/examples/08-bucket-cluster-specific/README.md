# 08-bucket-cluster-specific

この例は「クラウドプロバイダを意識させない Bucket API」を kro で実現する、いちばん現実的なパターンです。

ポイント:

- 開発者は常に同じ `apiVersion/kind: Bucket` のインスタンスを作る
- ただし **クラスタごとに** kro 側の `Bucket` 実装(RGD)を差し替える
  - AWS クラスタ: 裏では ACK S3 `Bucket` を作る
  - GCP クラスタ: 裏では Config Connector `StorageBucket` を作る
  - Azure クラスタ: 裏では Azure Service Operator の `StorageAccount` / `Container` を作る

この方式だと、インスタンス spec から `provider` フィールドを消せます。

## 重要な制約

このディレクトリの `bucket-rgd-aws.yaml` / `bucket-rgd-gcp.yaml` / `bucket-rgd-azure.yaml` は、
**すべて同じ GVK** (`platform.example.com/v1alpha1`, `kind: Bucket`) を生成します。

そのため:

- 1 つのクラスタに同時に複数は入れられません
- “クラスタ単位で provider が固定” のときに使う設計です

## 前提

クラスタに該当 provider のコントローラ(= CRD)が入っていること。

- AWS: ACK S3 controller
- GCP: Config Connector
- Azure: Azure Service Operator v2

## 適用

### 0) 共通 prereqs

```bash
kubectl apply -f 00-common-prereqs.yaml
```

### 1) クラスタの provider に合わせて RGD を 1つだけ適用

AWS クラスタ:

```bash
kubectl apply -f bucket-rgd-aws.yaml
```

GCP クラスタ:

```bash
kubectl apply -f bucket-rgd-gcp.yaml
```

Azure クラスタ:

```bash
kubectl apply -f bucket-rgd-azure.yaml
```

### 2) 開発者は常に同じ Bucket インスタンスを apply

```bash
kubectl apply -f bucket.yaml

kubectl get buckets
kubectl describe bucket demo-bucket
```

## 実務メモ

- GitOps では「クラスタ別 overlay」で `Bucket` RGD の実装を差し替えるのが定番です。
- これにより、アプリ側の manifest は provider 非依存になります。
- 逆に「1 クラスタで複数 provider を同時に扱う」場合は、`refs/kro/examples/07-portable-bucket-direct/` のように
  spec で provider を選ぶ/または別の仕組み(Admission で注入など)が必要です。
