# TODO

This file tracks *current* work items. Historical checklists were removed to keep this repo navigable; use `git log` for completed work.

## Current Focus

- [ ] Decide whether to pursue the CAPD facade behind the Kany8s suite (Proposed)
  - See: `docs/adr/0010-capd-facade-behind-kany8s-suite.md`

- [ ] Evaluate kro v0.8.x adoption (Collections + breaking schema change detection)
  - Update: `docs/runbooks/kind-kro.md`, `docs/reference/kro/`, and examples as needed

- [ ] Tighten RBAC for dynamic GVK (kro.run `resources=*`) (post-MVP)
  - See tradeoffs: `docs/adr/0007-dynamic-gvk-rbac-tradeoffs.md`

## Decisions (Done)

- [x] Infra outputs policy (no generic outputs in Kany8s core; Parent RGD / Approach A)
  - See: `docs/adr/0008-infra-outputs-policy-parent-rgd-approach-a.md`
