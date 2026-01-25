# Runbook: End-to-end (kro -> Kany8s -> RGD -> Cluster)

This runbook ties together the "install -> apply -> observe" flow described in `README.md`.

## Repro environment

- kind + kro: see `docs/runbooks/kind-kro.md`
- Cluster API: already installed on the management cluster
- Kany8s: controller + CRDs installed on the management cluster

## Prereqs

- A management cluster with Cluster API installed
- kro installed
- Kany8s installed
- A provider RGD that exposes the normalized kro instance status contract:
  - `status.ready: boolean`
  - `status.endpoint: string`

## Apply

```bash
# 1) Apply the provider RGD
kubectl apply -f <provider-rgd.yaml>
kubectl get rgd -o wide
kubectl describe rgd <rgd-name>

# 2) Apply the Cluster + Kany8sControlPlane
kubectl apply -f examples/capi/cluster.yaml
```

## Observe

```bash
# Cluster API objects
kubectl get clusters -A -o wide
kubectl describe cluster -n default demo-cluster

# Kany8s ControlPlane provider object
kubectl get kany8scontrolplanes -A -o wide
kubectl describe kany8scontrolplane -n default demo-cluster

# kro objects
kubectl get rgd -o wide

# The kro instance GVK is resolved from the RGD schema.
# Expected behavior: Kany8s creates exactly one instance (name/namespace matches the Kany8sControlPlane).
kubectl get <kroInstanceKind> -n default demo-cluster -o wide
kubectl describe <kroInstanceKind> -n default demo-cluster

# When the kro instance exposes ready+endpoint, Kany8s should:
# - set `Kany8sControlPlane.spec.controlPlaneEndpoint`
# - set `Kany8sControlPlane.status.initialization.controlPlaneInitialized=true`
# - (then Cluster API mirrors the endpoint into `Cluster.spec.controlPlaneEndpoint`)
kubectl get kany8scontrolplane -n default demo-cluster -o yaml
kubectl get cluster -n default demo-cluster -o yaml
```
