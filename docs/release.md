# Release

This document describes how we version and publish Kany8s releases.

## Versioning

- We use semantic versioning: `vMAJOR.MINOR.PATCH` (for example: `v0.3.0`).
- The git tag is the source of truth for the release version.
- The controller image tag SHOULD match the git tag (for example: `ghcr.io/<org>/kany8s-controller:v0.3.0`).

## Release Process

### 1. Pick the version

Decide the next tag (for example: `v0.3.0`).

### 2. Verify the repo

```bash
make test
make lint
```

### 3. Build and push the controller image

```bash
export IMG=ghcr.io/<org>/kany8s-controller:v0.3.0
make docker-build IMG=$IMG
make docker-push IMG=$IMG
```

### 4. Generate the install bundle

Kany8s is distributed as a single YAML bundle generated from the kustomize manifests.

```bash
make build-installer IMG=$IMG
ls -lh dist/install.yaml
```

### 5. Tag the release

Create the tag on the commit you want to release:

```bash
git tag -a v0.3.0 -m "Kany8s v0.3.0"
git push origin v0.3.0
```

### 6. Create a GitHub Release

Attach the bundle so users can download it directly.

```bash
gh release create v0.3.0 \
  --title "Kany8s v0.3.0" \
  --notes "<release notes>" \
  dist/install.yaml
```
