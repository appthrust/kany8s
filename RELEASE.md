# Release Runbook

kany8s is shipped as a Cluster API provider that can be consumed directly via
`clusterctl` or `cluster-api-operator`. Releases are automated with
[release-please](https://github.com/googleapis/release-please) + a GitHub
Actions `release.yml` pipeline.

Always read this file before cutting a release. It encodes the few non-obvious
details that the GitHub UI will not tell you.

## Release artifacts

Every published tag (`vX.Y.Z`) produces:

- Container images on GHCR (linux/amd64 + linux/arm64 manifest list):
  - `ghcr.io/appthrust/kany8s/manager:vX.Y.Z`
  - `ghcr.io/appthrust/kany8s/eks-kubeconfig-rotator:vX.Y.Z`
  - `ghcr.io/appthrust/kany8s/eks-karpenter-bootstrapper:vX.Y.Z`
- GitHub Release assets:
  - `metadata.yaml`
  - `infrastructure-components.yaml`
  - `control-plane-components.yaml`
  - `cluster-template.yaml`
  - `clusterctl-config.yaml`

The images and release assets are public. The three GHCR packages must be
set to "Public" visibility by hand the first time they are pushed, see the
troubleshooting section below.

## Automation topology

Two workflows collaborate on every main-branch merge:

1. `release-please.yml` — runs on every push to `main`. It opens (or updates)
   a single "chore(main): release X.Y.Z" PR that bumps `VERSION`, the
   `.release-please-manifest.json`, and prepends the `CHANGELOG.md` from the
   merged conventional commits.
2. `release.yml` — runs on `push: tags: ['v*']` and `workflow_dispatch`. It
   builds the three images, composes the multi-arch manifest list, then
   attaches the five provider assets to a draft GitHub Release.

Merging the release PR creates the tag. That tag push triggers `release.yml`
**only if the token that created the tag has `workflow` permissions**.
The default `GITHUB_TOKEN` explicitly does not fire downstream workflows,
which is why we configure release-please with a `RELEASE_PLEASE_TOKEN` secret
backed by a GitHub App (preferred) or a fine-grained PAT.

## One-time setup

### Install the release GitHub App

1. Go to https://github.com/organizations/appthrust/settings/apps and create
   (or reuse) a GitHub App named e.g. `appthrust-release-bot`.
2. Grant it the following repository permissions:
   - Contents: Read & Write
   - Issues: Read & Write
   - Pull requests: Read & Write
   - Workflows: Read & Write (required — this is what lets `release.yml`
     run after a tag push)
3. Install the app into the `appthrust/kany8s` repository.
4. Generate a private key and store it in the repository (or org) secrets.

### Configure the `RELEASE_PLEASE_TOKEN` secret

The `release-please.yml` workflow reads `secrets.RELEASE_PLEASE_TOKEN`.
Create it at `Settings → Secrets and variables → Actions → New repository
secret`.

Preferred pattern — ephemeral app token per run via `tibdex/github-app-token`
or `actions/create-github-app-token`. Alternatively, paste a fine-grained
PAT that has `contents:write`, `pull_requests:write`, and `workflows:write`.

Without this secret, release-please will push the release tag using
`GITHUB_TOKEN`, and `release.yml` will never fire. The cluster-api bundle
will not be built until you either re-run `release.yml` manually via
`workflow_dispatch` or rotate the tag with a privileged token.

### Allow Actions to open pull requests

Under `Settings → Actions → General → Workflow permissions`, enable
**"Allow GitHub Actions to create and approve pull requests"**. Without
this, release-please logs:

> GitHub Actions is not permitted to create or approve pull requests.

and the release PR is never opened. This toggle is required once per
repository.

### Branch protection

Require the following status checks on `main`:

- `ci`
- `smoke-test`

This prevents release PRs from merging while the generated provider bundle
fails to install into kind.

## Cutting v0.1.0 (the bootstrap release)

The very first release needs a couple of manual steps because the
release-please manifest starts at `0.0.0` and there is no GitHub Release
yet to anchor `clusterctl-config.yaml`.

**Canonical path for v0.1.0**: skip release-please and push the tag by
hand. Why: release-please+`GITHUB_TOKEN` pushes the tag as
`github-actions[bot]`, and GitHub's workflow chaining rule swallows
downstream workflow triggers on tags pushed by that identity. Until the
`RELEASE_PLEASE_TOKEN` secret (see "Configure the `RELEASE_PLEASE_TOKEN`
secret" above) is populated, the manual path is the only way to fire
`release.yml` automatically. After v0.1.0 is out **and** the secret is
configured, subsequent releases flow through release-please without any
manual tagging.

1. Merge the scaffolding commit that introduces this runbook, the
   `release-please` configuration, and the clusterctl provider manifests.
2. (Optional) Smoke-test the pipeline without touching GHCR:
   ```bash
   gh workflow run release.yml \
     --field tag=v0.1.0-rc0 \
     --field dry_run=true
   ```
   Confirm that `build-images` succeeds and that the workflow exits without
   creating a GitHub Release (because `dry_run=true` disables pushes).
3. Tag and push `v0.1.0`:
   ```bash
   git tag -a v0.1.0 -m "v0.1.0"
   git push origin v0.1.0
   ```
   This fires `release.yml`. Watch the run at
   `https://github.com/appthrust/kany8s/actions/workflows/release.yml` until
   the `release` job reports `Creating release` and finishes green.
4. Switch the three GHCR packages to public visibility:
   ```bash
   for pkg in manager eks-kubeconfig-rotator eks-karpenter-bootstrapper; do
     gh api \
       --method PATCH \
       -H "Accept: application/vnd.github+json" \
       "/orgs/appthrust/packages/container/kany8s%2F${pkg}" \
       -f visibility=public
   done
   ```
   (The same toggle exists in the GHCR UI under `Package settings`.)
5. Promote the draft Release. Open the draft created by
   `softprops/action-gh-release`, verify the five attached assets, then
   click `Publish release`.
6. Smoke-install with the published assets:
   ```bash
   clusterctl init \
     --config clusterctl-config.yaml \
     --infrastructure kany8s \
     --control-plane kany8s
   ```

After this first release the `release-please` loop is self-sustaining as
long as the token is configured.

## Cutting subsequent releases

1. Merge conventional-commit PRs to `main` (`feat: ...` bumps minor,
   `fix: ...` bumps patch, `feat!: ...` or `BREAKING CHANGE:` in the body
   bumps major).
2. release-please opens a `chore(main): release X.Y.Z` PR. Review the
   CHANGELOG diff and merge when it looks right.
3. The merge pushes the tag, which triggers `release.yml`.
4. Wait for the draft Release and publish it manually once you are happy.

## Moving to a new contract (contract bump)

`hack/capi/metadata.yaml` tracks the controller contract for each
releaseSeries. When you cut the first release of a new minor line (e.g.
`v0.2.0`) whose controller imports a different `sigs.k8s.io/cluster-api`
contract, prepend a new entry to the `releaseSeries` list:

```yaml
releaseSeries:
- major: 0
  minor: 2
  contract: v1beta3
- major: 0
  minor: 1
  contract: v1beta2
```

release-please does not know about this file, so the update has to land in
the PR that cuts the minor bump.

## Troubleshooting

### release.yml did not run after merging the release PR

The tag was pushed with `GITHUB_TOKEN`. Either:
- Re-trigger the workflow manually: `gh workflow run release.yml
  --field tag=vX.Y.Z --field dry_run=false`.
- Configure `RELEASE_PLEASE_TOKEN` with `workflows:write` and delete+recreate
  the tag so the downstream workflow is linked to the privileged push.

### GHCR packages 404 with "name unknown"

The packages were created private by default. Switch them to public with
the `gh api` snippet in the bootstrap section.

### `clusterctl init` rejects the provider with "contract not supported"

Either `metadata.yaml` declares the wrong contract or the installed
clusterctl is too old. Confirm with `clusterctl version` that you are on
`v1.12+` (v1beta2 contract) and that the contract line matches the minor
series you released.
