# Runbook: kind + kro

This runbook creates a local Kubernetes cluster with kind and installs kro.

## Repro environment

- kind: v0.31.0
- Kubernetes: v1.35.0 (kindest/node:v1.35.0)
- kro: v0.7.1

## Prereqs

- docker
- kind
- kubectl

## Steps

```bash
KIND_CLUSTER_NAME=kany8s-mgmt
KIND_NODE_IMAGE=kindest/node:v1.35.0
KRO_VERSION=0.7.1

kind create cluster --name ${KIND_CLUSTER_NAME} --image ${KIND_NODE_IMAGE} --wait 60s
kubectl --context kind-${KIND_CLUSTER_NAME} get nodes

kubectl --context kind-${KIND_CLUSTER_NAME} create namespace kro-system
kubectl --context kind-${KIND_CLUSTER_NAME} apply -f \
  https://github.com/kubernetes-sigs/kro/releases/download/v${KRO_VERSION}/kro-core-install-manifests.yaml
kubectl --context kind-${KIND_CLUSTER_NAME} rollout status -n kro-system deploy/kro
```

Optional (for experiments): relax kro RBAC.

NOTE: The upstream install manifests use aggregated RBAC and may be too restrictive for some RGDs.

```bash
kubectl --context kind-${KIND_CLUSTER_NAME} apply -f - <<'EOF'
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

## Observe

```bash
kubectl --context kind-${KIND_CLUSTER_NAME} -n kro-system get deploy,pods
kubectl --context kind-${KIND_CLUSTER_NAME} get rgd -o wide

# when you have an RGD + instance
kubectl --context kind-${KIND_CLUSTER_NAME} describe rgd <name>
kubectl --context kind-${KIND_CLUSTER_NAME} get <instanceKind> <name> -o wide
kubectl --context kind-${KIND_CLUSTER_NAME} describe <instanceKind> <name>
```

## Cleanup

```bash
kind delete cluster --name ${KIND_CLUSTER_NAME}
```
