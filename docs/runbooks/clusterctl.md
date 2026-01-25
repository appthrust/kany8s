# clusterctl

This runbook documents the current (MVP) packaging approach for using Kany8s with `clusterctl`.

## Decision (MVP)

- Use `make build-installer` (kustomize bundle) to produce a single provider manifest.
- Treat `dist/install.yaml` as Kany8s' clusterctl `ControlPlaneProvider` components.

## 1. Build the components YAML

```bash
make build-installer IMG=example.com/kany8s/controller:dev
ls -lh dist/install.yaml
```

## 2. Create the provider namespace

Until we embed a Namespace manifest into the generated bundle, create it manually:

```bash
kubectl create namespace kany8s-system
```

## 3. Point clusterctl at the local bundle

Create `~/.cluster-api/clusterctl.yaml` (note: `url` must be an absolute `file://` URL):

```yaml
providers:
  - name: kany8s
    type: ControlPlaneProvider
    url: file:///ABSOLUTE/PATH/TO/kany8s/dist/install.yaml
```

## 4. Install providers

Example (kind management cluster + CAPD for infra):

```bash
clusterctl init --infrastructure docker --control-plane kany8s
```

## 5. Verify

```bash
kubectl get deployments -n kany8s-system
kubectl get crds | grep kany8s
```
