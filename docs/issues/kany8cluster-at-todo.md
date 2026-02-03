# Kany8sCluster Acceptance (kro infra reflection) TODO

参照: `docs/issues/kany8scluster-at.md`

かならず、 prd.mdや、codebase.mdなどのドキュメントを参照してKany8sのコンセプトを理解してから実装作業を行うこと。

# 0) Naming / Scope

## 0.1 Naming

 - [x] Make target 名を `test-acceptance-kro-infra-reflection` に確定する
- [x] Make target 名を `test-acceptance-kro-infra-reflection-keep` に確定する
 - [x] Script 名を `hack/acceptance-test-kro-infra-reflection.sh` に確定する
- [x] Wrapper runner 名を `test/acceptance_test/run-acceptance-kro-infra-reflection.sh` に確定する

## 0.2 テストで固定する値

- [x] RGD 名を `demo-infra.kro.run` に確定する
- [x] kro instance CRD 名を `demoinfrastructures.kro.run` に確定する
- [x] Namespace のデフォルトを `default` に確定する
- [x] Cluster 名のデフォルトを `demo-cluster` に確定する
- [x] `KRO_VERSION` のデフォルトを `0.7.1` に確定する
- [x] `IMG` のデフォルトを `example.com/kany8s:acceptance-kro-infra` に確定する

## 0.3 script/wrapper が受け取る env var

- [x] hack script が `KIND_CLUSTER_NAME` を受け取れるようにする
- [x] hack script が `KUBECTL_CONTEXT` を受け取れるようにする
- [x] hack script が `KRO_VERSION` を受け取れるようにする
- [x] hack script が `IMG` を受け取れるようにする
- [x] hack script が `NAMESPACE` を受け取れるようにする
- [x] hack script が `CLUSTER_NAME` を受け取れるようにする
- [x] hack script が `CLEANUP` を受け取れるようにする
- [x] hack script が `ARTIFACTS_DIR` を受け取れるようにする
- [x] hack script が `KUBECONFIG_FILE` を受け取れるようにする
- [x] hack script が `KRO_RBAC_WORKAROUND_MANIFEST` を override できるようにする

- [x] hack script が `KRO_RBAC_WORKAROUND_MANIFEST` を override できるようにする
- [x] hack script が `KRO_RGD_MANIFEST` を override できるようにする
- [x] hack script が `KANY8S_CLUSTER_TEMPLATE` を override できるようにする

# 1) Manifests (acceptance source-of-truth)

## 1.1 infra RGD

- [x] ディレクトリ `test/acceptance_test/manifests/kro/infra/` を作成する
- [x] `examples/kro/infra/rgd.yaml` を `test/acceptance_test/manifests/kro/infra/rgd.yaml` にコピーする
- [x] `test/acceptance_test/manifests/kro/infra/rgd.yaml` の `metadata.name` が `demo-infra.kro.run` であることを確認する
- [x] `kubectl apply --dry-run=client -f test/acceptance_test/manifests/kro/infra/rgd.yaml` を実行して exit 0 を確認する

## 1.2 `Kany8sCluster` manifest template

- [x] `test/acceptance_test/manifests/kro/kany8scluster.yaml.tpl` を新規作成する
- [x] テンプレに `__CLUSTER_NAME__` を含める
- [x] テンプレに `__NAMESPACE__` を含める
- [x] テンプレに `__RGD_NAME__` を含める

## 1.3 テンプレ render の静的検証

- [x] `sed -e 's/__CLUSTER_NAME__/demo-cluster/g' -e 's/__NAMESPACE__/default/g' -e 's/__RGD_NAME__/demo-infra.kro.run/g' test/acceptance_test/manifests/kro/kany8scluster.yaml.tpl > /tmp/kany8scluster.yaml` を実行する
- [x] `kubectl apply --dry-run=client -f /tmp/kany8scluster.yaml` を実行して exit 0 を確認する

# 2) Acceptance script (hack)

## 2.1 ファイル作成と基本設定

- [ ] `hack/acceptance-test-kro-infra-reflection.sh` を新規作成する
- [ ] 先頭に `#!/usr/bin/env bash` を追加する
- [ ] `set -euo pipefail` を追加する

## 2.2 変数（デフォルト値）

- [ ] `timestamp="$(date +%Y%m%d%H%M%S)"` を追加する
- [ ] `KIND_CLUSTER_NAME` のデフォルトを `kany8s-acceptance-infra-${timestamp}` に設定する
- [ ] `KUBECTL_CONTEXT` のデフォルトを `kind-${KIND_CLUSTER_NAME}` に設定する
- [ ] `KRO_VERSION` のデフォルトを `0.7.1` に設定する
- [ ] `IMG` のデフォルトを `example.com/kany8s:acceptance-kro-infra` に設定する
- [ ] `NAMESPACE` のデフォルトを `default` に設定する
- [ ] `CLUSTER_NAME` のデフォルトを `demo-cluster` に設定する
- [ ] `CLEANUP` のデフォルトを `true` に設定する
- [ ] `ARTIFACTS_DIR` のデフォルトを `/tmp/kany8s-acceptance-kro-infra-${timestamp}` に設定する
- [ ] `KUBECONFIG_FILE` のデフォルトを `${ARTIFACTS_DIR}/kubeconfig` に設定する

## 2.3 固定値（infra RGD）

- [ ] `RGD_NAME="demo-infra.kro.run"` を追加する
- [ ] `RGD_INSTANCE_CRD="demoinfrastructures.kro.run"` を追加する

## 2.4 repo_root と manifest パス

- [x] `repo_root` を `hack/acceptance-test-kro-reflection.sh` と同じ方式で解決する
- [x] `cd "${repo_root}"` を追加する
- [ ] `KRO_RBAC_WORKAROUND_MANIFEST` のデフォルトを `test/acceptance_test/manifests/kro/rbac-unrestricted.yaml` に設定する
- [ ] `KRO_RGD_MANIFEST` のデフォルトを `test/acceptance_test/manifests/kro/infra/rgd.yaml` に設定する
- [ ] `KANY8S_CLUSTER_TEMPLATE` のデフォルトを `test/acceptance_test/manifests/kro/kany8scluster.yaml.tpl` に設定する

## 2.5 artifacts/log 設定

- [x] `mkdir -p "${ARTIFACTS_DIR}"` を追加する
- [x] log ファイル（例: `${ARTIFACTS_DIR}/acceptance-infra.log`）を作って `tee` を設定する
- [x] `export KUBECONFIG="${KUBECONFIG_FILE}"` を追加する

## 2.6 kustomization の退避/復元

- [x] `kustomization_path="${repo_root}/config/manager/kustomization.yaml"` を追加する
- [x] `backup_kustomization()` を追加する
- [x] `restore_kustomization()` を追加する

## 2.7 helper 関数

- [x] `need_cmd()` を追加する
- [x] `k()` を追加する（`kubectl --context "${KUBECTL_CONTEXT}"` wrapper）

## 2.8 diagnostics（失敗時収集）

- [x] `collect_diagnostics()` を追加する
- [x] diagnostics に `kind get clusters` を含める
- [x] diagnostics に kubeconfig context dump を含める
- [x] diagnostics に `k get nodes -o wide` を含める
- [x] diagnostics に `k get events -A --sort-by=.metadata.creationTimestamp` を含める
- [x] diagnostics に kro-system の `get all` を含める
- [x] diagnostics に kro-system の `logs deploy/kro` を含める
- [x] diagnostics に `k get rgd "${RGD_NAME}" -o yaml` を含める
- [x] diagnostics に `k get crd "${RGD_INSTANCE_CRD}" -o yaml` を含める
- [x] diagnostics に kany8s-system の `get all` を含める
- [x] diagnostics に kany8s-system の `logs deploy/kany8s-controller-manager -c manager` を含める
- [x] diagnostics に `k -n "${NAMESPACE}" get kany8scluster "${CLUSTER_NAME}" -o yaml` を含める
- [x] diagnostics に `k -n "${NAMESPACE}" get "${RGD_INSTANCE_CRD}" "${CLUSTER_NAME}" -o yaml` を含める

## 2.9 cleanup / trap

- [x] `cleanup()` を追加する（先頭で `restore_kustomization` を呼ぶ）
- [x] `cleanup()` に `CLEANUP=true` の場合の `kind delete cluster --name ... --kubeconfig ...` を追加する
- [x] `cleanup()` に `CLEANUP=false` の場合の “keep” メッセージを追加する
- [x] `on_exit()` を追加する（rc != 0 なら `collect_diagnostics`）
- [x] `on_exit()` で `cleanup` を呼ぶ
- [x] `trap on_exit EXIT` を追加する

## 2.10 必須コマンドチェック

- [x] `need_cmd docker` を追加する

- [x] `need_cmd kind` を追加する
- [x] `need_cmd kubectl` を追加する
- [x] `need_cmd make` を追加する
- [x] `need_cmd go` を追加する
- [x] `need_cmd curl` を追加する

## 2.11 kind クラスタ作成

- [x] `kind create cluster --name "${KIND_CLUSTER_NAME}" --wait 60s --kubeconfig "${KUBECONFIG_FILE}"` を追加する
- [x] `k get nodes -o wide` を追加する

## 2.12 kro install（vendor キャッシュ込み）

- [x] kro-system namespace が無ければ作成する
- [x] `KRO_CORE_INSTALL_MANIFEST` のデフォルトを `test/acceptance_test/vendor/kro/v${KRO_VERSION}/kro-core-install-manifests.yaml` に設定する
- [x] `mkdir -p "$(dirname "${KRO_CORE_INSTALL_MANIFEST}")"` を追加する
- [x] install manifest が無ければ GitHub releases から `curl -fsSL -o ...` で取得する
- [x] `k apply -f "${KRO_CORE_INSTALL_MANIFEST}"` を追加する
- [x] `k -n kro-system rollout status deploy/kro --timeout=180s` を追加する

## 2.13 kro RBAC workaround

- [x] `k apply -f "${KRO_RBAC_WORKAROUND_MANIFEST}"` を追加する

## 2.14 infra RGD apply + wait

- [x] `k apply -f "${KRO_RGD_MANIFEST}"` を追加する
- [x] `k wait --for=condition=ResourceGraphAccepted --timeout=120s "rgd/${RGD_NAME}"` を追加する
- [x] `k get crd "${RGD_INSTANCE_CRD}" -o name` を追加する

## 2.15 Kany8s の install/build/deploy

- [x] `make install` を追加する
- [x] `make docker-build IMG="${IMG}"` を追加する
- [x] `kind load docker-image "${IMG}" --name "${KIND_CLUSTER_NAME}"` を追加する
- [x] `backup_kustomization` を呼ぶ
- [x] `make deploy IMG="${IMG}"` を追加する
- [x] `k -n kany8s-system rollout status deployment/kany8s-controller-manager --timeout=180s` を追加する

## 2.16 `Kany8sCluster` apply（テンプレ render）

- [x] `rendered_cluster_manifest="${ARTIFACTS_DIR}/kany8scluster.yaml"` を追加する
- [x] `sed` で `__CLUSTER_NAME__` を置換して render する
- [x] `sed` で `__NAMESPACE__` を置換して render する
- [x] `sed` で `__RGD_NAME__` を置換して render する
- [x] `k apply -f "${rendered_cluster_manifest}"` を追加する

## 2.17 `Kany8sCluster` contract wait

- [x] `k -n "${NAMESPACE}" wait --for=condition=Ready --timeout=240s "kany8scluster/${CLUSTER_NAME}"` を追加する
- [x] `k -n "${NAMESPACE}" wait --for=jsonpath='{.status.initialization.provisioned}'=true --timeout=240s "kany8scluster/${CLUSTER_NAME}"` を追加する

## 2.18 failure fields が空であることを確認

- [x] `failureReason` を jsonpath で取得し、空（または `<no value>`）以外なら fail する
- [x] `failureMessage` を jsonpath で取得し、空（または `<no value>`）以外なら fail する

## 2.19 kro instance の存在/ready の確認

- [x] kro instance が取得できるまでリトライする（最大 240 秒）
- [x] `k -n "${NAMESPACE}" wait --for=jsonpath='{.status.ready}'=true --timeout=180s "${RGD_INSTANCE_CRD}/${CLUSTER_NAME}"` を追加する

## 2.20 kro instance spec 注入の確認

- [x] `spec.clusterName` が `CLUSTER_NAME` と一致することを確認する
- [x] `spec.clusterNamespace` が `NAMESPACE` と一致することを確認する

## 2.21 スクリプトの静的検証

- [x] `chmod +x hack/acceptance-test-kro-infra-reflection.sh` を実行する
- [x] `bash -n hack/acceptance-test-kro-infra-reflection.sh` を実行して exit 0 を確認する

# 3) Wrapper runner (test/acceptance_test)

## 3.1 wrapper 作成

- [ ] `test/acceptance_test/run-acceptance-kro-infra-reflection.sh` を新規作成する
- [ ] 先頭に `#!/usr/bin/env bash` を追加する
- [ ] `set -euo pipefail` を追加する

## 3.2 wrapper の変数/表示

- [ ] `timestamp` を設定する（`TIMESTAMP` があれば優先）
- [ ] `repo_root` を `../..` から解決する
- [ ] `KIND_CLUSTER_NAME` のデフォルトを `kany8s-acc-infra-${timestamp}` に設定する
- [ ] `ARTIFACTS_DIR` のデフォルトを `/tmp/kany8s-acceptance-kro-infra-reflection-${timestamp}` に設定する
- [ ] `CLEANUP` のデフォルトを `true` に設定する

## 3.3 pre-clean kind

- [ ] `kind` の存在チェックを追加する
- [ ] `kind delete cluster --name "${KIND_CLUSTER_NAME}"` を追加する（失敗しても OK）

## 3.4 hack script に委譲

- [ ] wrapper から `hack/acceptance-test-kro-infra-reflection.sh` を `bash` 実行する
- [ ] 実行時に `KIND_CLUSTER_NAME` を渡す
- [ ] 実行時に `ARTIFACTS_DIR` を渡す
- [ ] 実行時に `CLEANUP` を渡す

## 3.5 wrapper の静的検証

- [ ] `chmod +x test/acceptance_test/run-acceptance-kro-infra-reflection.sh` を実行する
- [ ] `bash -n test/acceptance_test/run-acceptance-kro-infra-reflection.sh` を実行して exit 0 を確認する

# 4) Makefile targets

## 4.1 target 追加

- [ ] `.PHONY: test-acceptance-kro-infra-reflection` を追加する
- [ ] `test-acceptance-kro-infra-reflection` の recipe を `bash hack/acceptance-test-kro-infra-reflection.sh` にする
- [ ] `.PHONY: test-acceptance-kro-infra-reflection-keep` を追加する
- [ ] `test-acceptance-kro-infra-reflection-keep` の recipe を `CLEANUP=false bash hack/acceptance-test-kro-infra-reflection.sh` にする

# 5) run-all integration (optional)

## 5.1 run-all に追加

- [x] `test/acceptance_test/run-all.sh` に “kro infra reflection” の実行ブロックを追加する
- [x] run-all の新ブロックで `ARTIFACTS_DIR` を `.../acceptance-kro-infra-reflection` に分ける
- [x] run-all の新ブロックで `KIND_CLUSTER_NAME` を `kany8s-acc-infra-${timestamp}` に分ける
- [x] run-all の新ブロックから `test/acceptance_test/run-acceptance-kro-infra-reflection.sh` を呼ぶ

# 6) repo-policy tests (internal/devtools)

## 6.1 script exists

- [ ] `internal/devtools/acceptance_test_script_test.go` に infra 用の "script exists" テストを追加する
- [ ] 新テストで `hack/acceptance-test-kro-infra-reflection.sh` を読み込む
- [ ] `wantSubstrings` に `kind create cluster` を含める
- [ ] `wantSubstrings` に `kro-core-install-manifests.yaml` を含める
- [ ] `wantSubstrings` に `ResourceGraphAccepted` を含める
- [ ] `wantSubstrings` に `test/acceptance_test/manifests/kro/infra/rgd.yaml` を含める
- [ ] `wantSubstrings` に `make deploy` を含める
- [ ] `wantSubstrings` に `kany8scluster` を含める
- [ ] `wantSubstrings` に `status.initialization.provisioned` を含める

## 6.2 Makefile target exists

- [ ] `internal/devtools/acceptance_test_script_test.go` に infra 用の "Makefile target exists" テストを追加する
- [ ] `wantSubstrings` に `test-acceptance-kro-infra-reflection:` を含める
- [ ] `wantSubstrings` に `bash hack/acceptance-test-kro-infra-reflection.sh` を含める
- [ ] `wantSubstrings` に `test-acceptance-kro-infra-reflection-keep:` を含める
- [ ] `wantSubstrings` に `CLEANUP=false bash hack/acceptance-test-kro-infra-reflection.sh` を含める

# 7) Docs updates

## 7.1 `test/acceptance_test/README.md`

- [x] `test/acceptance_test/README.md` に `run-acceptance-kro-infra-reflection.sh` を追記する
- [x] `test/acceptance_test/README.md` に Purpose（1行）を追記する

## 7.2 `docs/README.md`

- [x] `docs/README.md` の acceptance targets/scripts 一覧に `test-acceptance-kro-infra-reflection` を追記する

## 7.3 `docs/e2e-and-acceptance-test.md`

- [x] `docs/e2e-and-acceptance-test.md` の acceptance targets 一覧に `test-acceptance-kro-infra-reflection` を追記する

## 7.4 `docs/codebase.md`

- [x] `docs/codebase.md` の Acceptance Scripts/Entry Points に infra reflection を追記する

## 7.5 `acceptance.md`（任意）

- [ ] `acceptance.md` に “kro infra reflection” の受け入れ条件を1項目追記する

# 8) Verification

## 8.1 repo-policy / unit tests

- [ ] `go test ./internal/devtools` を実行する
- [ ] `go test ./...` を実行する

## 8.2 shell syntax

- [ ] `bash -n hack/acceptance-test-kro-infra-reflection.sh` を実行する
- [ ] `bash -n test/acceptance_test/run-acceptance-kro-infra-reflection.sh` を実行する

# 9) End-to-end (optional, manual)

## 9.1 kind 上で実行

- [ ] `make test-acceptance-kro-infra-reflection-keep` を実行する
- [ ] `kubectl --context kind-<KIND_CLUSTER_NAME> -n <NAMESPACE> get kany8scluster <CLUSTER_NAME> -o yaml` で `provisioned=true` を確認する
- [ ] `kubectl --context kind-<KIND_CLUSTER_NAME> -n <NAMESPACE> get demoinfrastructures.kro.run <CLUSTER_NAME> -o yaml` で `.spec.clusterName/.spec.clusterNamespace` を確認する
