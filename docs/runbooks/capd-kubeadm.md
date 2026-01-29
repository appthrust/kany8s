# CAPD + kubeadm (self-managed)

This runbook provisions a self-managed Kubernetes control plane on local Docker using:

- Cluster API core
- CABPK (`kubeadm` bootstrap)
- CAPD (Docker infrastructure)
- Kany8s `Kany8sKubeadmControlPlane` (control plane provider)

Goal: reach `RemoteConnectionProbe=True` and `Cluster Available=True`.

## Prerequisites

- `docker`
- `kind`
- `kubectl`
- `clusterctl`
- `make` + Go (to build the controller image)

## 0. From the repo root

This runbook assumes you are running commands from the Kany8s repo root.

## 1. Create a kind management cluster

```bash
kind create cluster --name kany8s --wait 60s
kubectl config use-context kind-kany8s
kubectl get nodes
```

## 2. Build and load the Kany8s controller image into kind

```bash
IMG=controller:dev make docker-build
kind load docker-image controller:dev --name kany8s
```

## 3. Build the clusterctl components bundle for Kany8s

```bash
IMG=controller:dev make build-installer
ls -lh dist/install.yaml
```

## 4. Point clusterctl at the local Kany8s bundle

Create `~/.cluster-api/clusterctl.yaml` (note: the `url` must be an absolute `file://` URL):

```bash
mkdir -p ~/.cluster-api

KANY8S_REPO_ROOT="$(pwd)"
cat > ~/.cluster-api/clusterctl.yaml <<EOF
providers:
  - name: kany8s
    type: ControlPlaneProvider
    url: file://${KANY8S_REPO_ROOT}/dist/install.yaml
EOF
```

## 5. Install Cluster API providers (CAPD + CABPK + Kany8s)

`clusterctl` may install cert-manager automatically if it's not present; this can take a few minutes.

```bash
kubectl create namespace kany8s-system

clusterctl init --infrastructure docker --bootstrap kubeadm --control-plane kany8s
```

Verify:

```bash
kubectl get deployments -n capi-system
kubectl get deployments -n capd-system
kubectl get deployments -n cabpk-system
kubectl get deployments -n kany8s-system
```

## 6. Create the self-managed workload cluster

```bash
kubectl apply -f examples/self-managed-docker/cluster.yaml
```

Watch progress:

```bash
kubectl get clusters -A -w
clusterctl describe cluster -n default demo-self-managed-docker
```

Wait for `Cluster Available=True`:

```bash
kubectl wait --for=condition=Available cluster/demo-self-managed-docker -n default --timeout=30m
```

## 7. Fetch the workload kubeconfig and verify

```bash
clusterctl get kubeconfig -n default demo-self-managed-docker > /tmp/demo-self-managed-docker.kubeconfig

kubectl --kubeconfig /tmp/demo-self-managed-docker.kubeconfig get nodes
```

## 8. Cleanup

```bash
kubectl delete -f examples/self-managed-docker/cluster.yaml
kind delete cluster --name kany8s
```
