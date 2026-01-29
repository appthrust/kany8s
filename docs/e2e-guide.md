# End-to-End Verification Guide

This document captures the exact commands and checks used to verify the README demo flow on a fresh **kind** cluster, with **kro v0.7.1**.

Scope:

- kro: `ResourceGraphDefinition` (RGD) is accepted (`ResourceGraphAccepted=True`) and generates the instance CRD.
- Kany8s: consumes the kro instance `status.ready/status.endpoint` and reflects it into `Kany8sControlPlane` (endpoint + initialized + conditions).
- (Optional) Cluster API: verifies that `Cluster.spec.controlPlaneEndpoint` is mirrored from `Kany8sControlPlane` (requires CAPI v1beta2 contract).

Notes:

- The demo RGD (`examples/kro/ready-endpoint/`) creates an `nginx` Deployment/Service. This is **not** a real Kubernetes API server, so Cluster API's `RemoteConnectionProbe` will fail and `Cluster Available=False` is expected.
- kro v0.7.1 may require relaxed RBAC to watch generated CRDs; this guide applies the unrestricted aggregation ClusterRole from `docs/kro.md`.

## Prerequisites

- `docker`
- `kind`
- `kubectl`
- `make` + Go toolchain (for `make install`, `make docker-build`, `make deploy`)

Optional (only if you want to apply `Cluster` objects):

- `clusterctl` (v1.12.2 recommended for v1beta2 contract)

All commands below assume you run them from the repository root.

## Variables

```bash
export KIND_CLUSTER_NAME=kany8s
export KUBECTL_CONTEXT=kind-${KIND_CLUSTER_NAME}
export KRO_VERSION=0.7.1

export NAMESPACE=default
export CLUSTER_NAME=demo-cluster

# Controller image tag used for kind load + deploy
export IMG=example.com/kany8s:demo-f146805
```

## 1) Create a fresh kind cluster

```bash
kind create cluster --name "${KIND_CLUSTER_NAME}" --wait 60s

kubectl --context "${KUBECTL_CONTEXT}" get nodes
```

## 2) Install kro v0.7.1

```bash
kubectl --context "${KUBECTL_CONTEXT}" create namespace kro-system

kubectl --context "${KUBECTL_CONTEXT}" apply -f \
  "https://github.com/kubernetes-sigs/kro/releases/download/v${KRO_VERSION}/kro-core-install-manifests.yaml"

kubectl --context "${KUBECTL_CONTEXT}" rollout status -n kro-system deploy/kro --timeout=180s
```

Verification:

```bash
kubectl --context "${KUBECTL_CONTEXT}" -n kro-system get deploy kro -o wide
```

## 3) Relax kro controller RBAC (v0.7.1 workaround)

```bash
kubectl --context "${KUBECTL_CONTEXT}" apply -f - <<'EOF'
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

Verification:

```bash
kubectl --context "${KUBECTL_CONTEXT}" get clusterrole kro:controller:unrestricted -o name
```

## 4) Apply the demo RGD and verify acceptance (ResourceGraphAccepted=True)

```bash
kubectl --context "${KUBECTL_CONTEXT}" apply -f examples/kro/ready-endpoint/rgd.yaml

kubectl --context "${KUBECTL_CONTEXT}" wait \
  --for=condition=ResourceGraphAccepted \
  --timeout=120s \
  rgd/demo-control-plane.kro.run
```

Verification (print the acceptance condition):

```bash
kubectl --context "${KUBECTL_CONTEXT}" get rgd demo-control-plane.kro.run -o jsonpath='{
  .status.conditions[?(@.type=="ResourceGraphAccepted")].status
}{"\n"}{
  .status.conditions[?(@.type=="ResourceGraphAccepted")].reason
}{"\n"}{
  .status.conditions[?(@.type=="ResourceGraphAccepted")].message
}{"\n"}'
```

Verification (the generated instance CRD must exist):

```bash
kubectl --context "${KUBECTL_CONTEXT}" get crd democontrolplanes.kro.run -o name
```

## 5) Install + deploy Kany8s (in-cluster)

These are the exact steps used during verification.

1. Ensure `kubectl` uses the kind cluster as the current context.

```bash
kubectl config use-context "${KUBECTL_CONTEXT}"
```

2. Install CRDs.

```bash
make install
```

3. Build the controller image, load it into kind, and deploy.

```bash
make docker-build IMG="${IMG}"
kind load docker-image "${IMG}" --name "${KIND_CLUSTER_NAME}"

make deploy IMG="${IMG}"

kubectl -n kany8s-system rollout status deployment/kany8s-controller-manager --timeout=180s
```

Note: `make deploy` updates `config/manager/kustomization.yaml` to set the image; to keep the repo clean:

```bash
git restore config/manager/kustomization.yaml
```

## 6) Apply a Kany8sControlPlane and verify the full kro -> Kany8s flow

If you do not have Cluster API installed, apply only the `Kany8sControlPlane` object.

```bash
kubectl --context "${KUBECTL_CONTEXT}" apply -f - <<EOF
apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
kind: Kany8sControlPlane
metadata:
  name: ${CLUSTER_NAME}
  namespace: ${NAMESPACE}
spec:
  version: "1.34"
  resourceGraphDefinitionRef:
    name: demo-control-plane.kro.run
  kroSpec:
    name: ${CLUSTER_NAME}
EOF

kubectl --context "${KUBECTL_CONTEXT}" -n "${NAMESPACE}" wait \
  --for=condition=Ready \
  --timeout=240s \
  "kany8scontrolplane/${CLUSTER_NAME}"
```

Verification: kro instance is created 1:1 (same name/namespace as the control plane).

```bash
# Wait for the instance to exist first (kubectl wait fails with NotFound if it doesn't exist yet).
until kubectl --context "${KUBECTL_CONTEXT}" -n "${NAMESPACE}" get democontrolplanes.kro.run "${CLUSTER_NAME}" >/dev/null 2>&1; do
  sleep 1
done

kubectl --context "${KUBECTL_CONTEXT}" -n "${NAMESPACE}" wait \
  --for=jsonpath='{.status.ready}'=true \
  --timeout=180s \
  "democontrolplanes.kro.run/${CLUSTER_NAME}"
```

Verification: print normalized kro instance status.

```bash
kubectl --context "${KUBECTL_CONTEXT}" -n "${NAMESPACE}" get democontrolplanes.kro.run "${CLUSTER_NAME}" \
  -o jsonpath='{.status.ready}{"\n"}{.status.endpoint}{"\n"}{.status.state}{"\n"}'
```

Verification: print what Kany8s reflected.

```bash
kubectl --context "${KUBECTL_CONTEXT}" -n "${NAMESPACE}" get kany8scontrolplane "${CLUSTER_NAME}" \
  -o jsonpath='{.spec.controlPlaneEndpoint.host}{"\n"}{.spec.controlPlaneEndpoint.port}{"\n"}{.status.initialization.controlPlaneInitialized}{"\n"}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{.status.failureReason}{"\n"}{.status.failureMessage}{"\n"}'
```

Optional debugging:

```bash
kubectl --context "${KUBECTL_CONTEXT}" -n kany8s-system logs deploy/kany8s-controller-manager -c manager --tail=200
kubectl --context "${KUBECTL_CONTEXT}" -n kro-system logs deploy/kro --tail=200
```

## 7) (Optional) Install Cluster API and apply `examples/capi/cluster.yaml`

This section is only required if you want to apply the CAPI `Cluster` object.

### 7.1 Ensure the management cluster supports the v1beta2 contract

Check what versions the Cluster CRD serves:

```bash
kubectl --context "${KUBECTL_CONTEXT}" get crd clusters.cluster.x-k8s.io \
  -o jsonpath='{range .spec.versions[*]}{.name}{":"}{.served}{","}{.storage}{"\n"}{end}'
```

Expected: `v1beta2:true,true`.

If you only have up to `v1beta1`, use `clusterctl` v1.12.2 to install/upgrade providers to the v1beta2 contract.

Download `clusterctl` v1.12.2 (exact command used):

```bash
curl -fsSL -o bin/clusterctl-v1.12.2 \
  https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.12.2/clusterctl-linux-amd64
chmod +x bin/clusterctl-v1.12.2
bin/clusterctl-v1.12.2 version
```

Install providers (fresh cluster):

```bash
bin/clusterctl-v1.12.2 init --infrastructure docker --wait-providers --kubeconfig-context "${KUBECTL_CONTEXT}"
```

If you already installed older providers (v1beta1 contract), upgrade them:

```bash
bin/clusterctl-v1.12.2 upgrade apply --contract v1beta2 --wait-providers --kubeconfig-context "${KUBECTL_CONTEXT}"
```

Re-check that `v1beta2:true,true` is present after this.

### 7.2 Create the InfrastructureRef (required for `Cluster.spec.infrastructureRef`)

`examples/capi/cluster.yaml` references `Kany8sCluster` as the InfrastructureRef. Create it:

```bash
kubectl --context "${KUBECTL_CONTEXT}" apply -f - <<EOF
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: Kany8sCluster
metadata:
  name: ${CLUSTER_NAME}
  namespace: ${NAMESPACE}
spec: {}
EOF
```

### 7.3 Apply the sample and verify endpoint mirroring

```bash
kubectl --context "${KUBECTL_CONTEXT}" apply -f examples/capi/cluster.yaml

kubectl --context "${KUBECTL_CONTEXT}" -n "${NAMESPACE}" wait \
  --for=jsonpath='{.spec.controlPlaneEndpoint.host}'=demo-cluster-svc.default.svc.cluster.local \
  --timeout=240s \
  "cluster/${CLUSTER_NAME}"
```

Verification: print mirrored endpoint and the key readiness flags.

```bash
kubectl --context "${KUBECTL_CONTEXT}" -n "${NAMESPACE}" get cluster "${CLUSTER_NAME}" \
  -o jsonpath='{.spec.controlPlaneEndpoint.host}{"\n"}{.spec.controlPlaneEndpoint.port}{"\n"}{.status.infrastructureReady}{"\n"}{.status.controlPlaneReady}{"\n"}'
```

Expected:

- endpoint host/port are set
- `infrastructureReady=true`
- `controlPlaneReady=true`

Note: `Cluster Available=False` is expected for this demo. To see why:

```bash
kubectl --context "${KUBECTL_CONTEXT}" -n "${NAMESPACE}" get cluster "${CLUSTER_NAME}" \
  -o jsonpath='{range .status.conditions[*]}{.type}{":"}{.status}{" "}{.reason}{"\n"}{end}'
```

## 8) Cleanup

Fast cleanup (delete the whole kind cluster):

```bash
kind delete cluster --name "${KIND_CLUSTER_NAME}"
```

If you want to keep the cluster but remove Kany8s:

```bash
kubectl config use-context "${KUBECTL_CONTEXT}"
make undeploy
make uninstall
```
