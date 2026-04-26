#!/usr/bin/env python3
"""Idempotently append a v1beta2 served alias to Cluster API CRD spec.versions.

For each CRD YAML passed on the command line, locate the existing
``v1alpha1`` entry inside ``spec.versions``, copy its block verbatim, and
append a v1beta2 alias with ``served: true`` / ``storage: false``. The
original v1alpha1 block (including its schema, served, storage, and
subresources fields) is preserved byte-for-byte so the only diff is the
appended v1beta2 entry.

Why we need this:

- Sveltos addon-controller v1.8 and CAPI v1.13 perform API discovery on
  ``infrastructure.cluster.x-k8s.io/v1beta2`` directly, bypassing the
  CRD label hint that the topology controller follows.
- We cannot bump the storage version without a migration, so the alias is
  served alongside v1alpha1 with ``storage: false`` and the default
  conversion strategy (None). Because the schemas are identical, the API
  server can return v1alpha1-stored bytes through either version.
- This script runs after controller-gen so it always sees freshly
  generated CRDs and produces deterministic output.

The script is invoked from the Makefile ``manifests`` and
``clusterapi-manifests`` targets. It is text-based on purpose: it never
re-emits the controller-gen YAML, only inserts new lines next to the
matched v1alpha1 block.
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

STORAGE_VERSION = "v1alpha1"
ALIAS_VERSION = "v1beta2"

# controller-gen (kubebuilder default) emits the spec.versions list with two
# leading spaces for each list item. Some CRDs put `name:` on the same line
# as the `- ` dash; others start the item with a different key (e.g.
# `additionalPrinterColumns:`) and indent `name:` four spaces. Handle both.
_RE_LIST_ITEM_START = re.compile(r"^  - ")
_RE_SIBLING_KEY = re.compile(r"^  [A-Za-z]")
_RE_NAME_ALPHA = re.compile(r"^    name: v1alpha1\s*$")
_RE_NAME_ALPHA_INLINE = re.compile(r"^  - name: v1alpha1\s*$")
_RE_NAME_BETA = re.compile(r"^(    | {2}- )name: v1beta2\s*$")


def _find_alpha_block(lines: list[str]) -> tuple[int, int] | None:
    """Return (start, end_exclusive) line indices spanning the v1alpha1 entry.

    Detects v1alpha1 either as the first key of a list item ("  - name:
    v1alpha1") or as a non-first key ("    name: v1alpha1") and walks
    backwards to the enclosing list item header.
    """
    name_idx = None
    for idx, line in enumerate(lines):
        if _RE_NAME_ALPHA_INLINE.match(line) or _RE_NAME_ALPHA.match(line):
            name_idx = idx
            break
    if name_idx is None:
        return None

    start = name_idx
    if not _RE_LIST_ITEM_START.match(lines[start]):
        for idx in range(name_idx - 1, -1, -1):
            if _RE_LIST_ITEM_START.match(lines[idx]):
                start = idx
                break
        else:
            return None

    end = len(lines)
    for idx in range(start + 1, len(lines)):
        line = lines[idx]
        if _RE_LIST_ITEM_START.match(line) or _RE_SIBLING_KEY.match(line):
            end = idx
            break
    return start, end


def _transform_to_beta(block: list[str]) -> list[str]:
    out: list[str] = []
    for line in block:
        if _RE_NAME_ALPHA_INLINE.match(line):
            out.append("  - name: v1beta2")
        elif _RE_NAME_ALPHA.match(line):
            out.append("    name: v1beta2")
        elif line == "    storage: true":
            out.append("    storage: false")
        else:
            out.append(line)
    return out


def process(path: Path) -> bool:
    text = path.read_text()

    if any(_RE_NAME_BETA.match(line) for line in text.splitlines()):
        return False

    lines = text.split("\n")
    span = _find_alpha_block(lines)
    if span is None:
        print(f"[skip] {path}: no {STORAGE_VERSION} entry", file=sys.stderr)
        return False
    start, end = span

    alias_block = _transform_to_beta(lines[start:end])

    new_lines = lines[:end] + alias_block + lines[end:]
    new_text = "\n".join(new_lines)
    if not new_text.endswith("\n") and text.endswith("\n"):
        new_text += "\n"
    path.write_text(new_text)
    print(f"[ok] {path}: {ALIAS_VERSION} alias added", file=sys.stderr)
    return True


def main(argv: list[str]) -> int:
    if not argv:
        print("usage: add-v1beta2-alias.py <crd-yaml> [...]", file=sys.stderr)
        return 2

    for arg in argv:
        path = Path(arg)
        if not path.is_file():
            print(f"[skip] {arg}: not a file", file=sys.stderr)
            continue
        process(path)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
