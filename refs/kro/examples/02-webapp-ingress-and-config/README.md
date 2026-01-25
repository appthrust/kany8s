# 02-webapp-ingress-and-config

少し実務っぽい Web アプリ例です。

学べること:

- `includeWhen` で Ingress を任意にする
- `externalRef` で platform 管理の ConfigMap を参照する
- ConfigMap `data` のような「キーが不定」なフィールドに `?` / `.orValue()` を使う

## 前提

- Ingress controller はクラスタ側で用意されている前提です(例では `ingressClassName: nginx`)。

## 適用

```bash
kubectl apply -f 00-prereqs.yaml
kubectl apply -f rgd.yaml

# Ingress なし
kubectl apply -f instance-no-ingress.yaml

# Ingress あり
kubectl apply -f instance-with-ingress.yaml

kubectl get rgd webapp-ingress-config.kro.run -o wide
kubectl get webappconfigs
kubectl get deploy,svc,ingress
```
