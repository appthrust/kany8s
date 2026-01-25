# kind + kro

This runbook sets up a local **kind** management cluster and installs **kro**.

## Prerequisites

- `docker`
- `kind`
- `kubectl`

## 1. Create a kind cluster

```bash
kind create cluster --name kany8s --wait 60s
kubectl config use-context kind-kany8s
kubectl get nodes
```

## 2. Install kro (v0.7.1 tested)

```bash
KRO_VERSION=0.7.1

kubectl create namespace kro-system
kubectl apply -f \
  https://github.com/kubernetes-sigs/kro/releases/download/v${KRO_VERSION}/kro-core-install-manifests.yaml

kubectl rollout status -n kro-system deploy/kro
```

Note: kro v0.7.1 may require relaxed RBAC for its dynamic controller to watch generated CRDs.
See `docs/kro.md` for details.

## 3. (Optional) Relax kro controller RBAC

```bash
kubectl apply -f - <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kro:controller:unrestricted
  labels:
    rbac.kro.run/aggregate-to-controller: "true"
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["*"]
EOF
```

## 4. Cleanup

```bash
kind delete cluster --name kany8s
```
