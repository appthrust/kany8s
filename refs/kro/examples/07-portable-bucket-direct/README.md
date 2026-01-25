# 07-portable-bucket-direct

この例は「抽象リソース -> 具象 Managed Resource への変換」を **中間 RGD なし** で行うパターンです。

`PortableBucket` という抽象 API を kro で提供し、裏で以下を直接作ります:

- AWS: ACK S3 `Bucket` (`s3.services.k8s.aws/v1alpha1`)
- GCP: Config Connector `StorageBucket` (`storage.cnrm.cloud.google.com/v1beta1`)
- Azure: Azure Service Operator v2 `StorageAccount` / `StorageAccountsBlobServicesContainer` など

## 重要: provider を完全には隠せない

この RGD は 1つのクラスタ内で provider を切り替えるため、`spec.provider` を持ちます。

- “クラスタごとに provider は固定” にできる場合は、`refs/kro/examples/08-bucket-cluster-specific/` の方が
  開発者から provider を完全に隠せます。

## 前提

対象 provider の CRD がクラスタにインストールされている必要があります。

- AWS: ACK S3 controller
- GCP: Config Connector
- Azure: Azure Service Operator v2

## 適用

```bash
kubectl apply -f rgd.yaml
kubectl get rgd portable-bucket-direct.platform.example.com -o wide
```

## インスタンス例

```bash
# AWS
kubectl apply -f instance-aws.yaml

# GCP
kubectl apply -f instance-gcp.yaml

# Azure
kubectl apply -f instance-azure.yaml
```

確認:

```bash
kubectl get portablebuckets
kubectl describe portablebucket <name>
```
