# Release

This document describes the MVP release flow for Kany8s: versioning, image publishing, and producing the install bundle used by `kubectl`/`clusterctl`.

## Versioning

- We use SemVer tags: `vMAJOR.MINOR.PATCH`.
- Until `v1.0.0`, breaking changes may happen in minor releases, but we should still document them clearly in release notes.
- Release tags are git tags and should correspond to a repository state that can be installed via `dist/install.yaml`.

## Release process

### 1. Choose the version

Pick a new version tag (example: `v0.1.0`).

### 2. Verify locally

```bash
make test
make lint
```

### 3. Build and push the controller image

Set the release image tag (example uses GHCR):

```bash
export IMG=ghcr.io/<org>/kany8s/controller:v0.1.0
```

Build + push:

```bash
make docker-build IMG=$IMG
make docker-push IMG=$IMG
```

Optional (multi-arch) build:

```bash
make docker-buildx IMG=$IMG
```

### 4. Generate the install bundle

Generate the single YAML bundle with the release image wired in:

```bash
make build-installer IMG=$IMG
ls -lh dist/install.yaml
```

Commit `dist/install.yaml` in the same release commit so users can install directly from the tag.

### 5. Tag and publish

Create an annotated tag and push it:

```bash
git tag -a v0.1.0 -m "kany8s v0.1.0"
git push origin v0.1.0
```

Create a GitHub Release and attach `dist/install.yaml` (and release notes).

### 6. Post-release verification

Install from the tag and verify the controller comes up:

```bash
kubectl create namespace kany8s-system
kubectl apply -f https://raw.githubusercontent.com/<org>/<repo>/v0.1.0/dist/install.yaml
kubectl get deployments -n kany8s-system
```
