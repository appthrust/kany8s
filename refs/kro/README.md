# kro (Kube Resource Orchestrator) 開発者向けドキュメント

このディレクトリは、Kubernetes SIG Cloud Provider のサブプロジェクト **kro** (Kube Resource Orchestrator) を、実務で使いこなすための日本語リファレンスとしてまとめたものです。

対象バージョン

- kro: `v0.7.1` (2025-12-13 リリース)
- 公式Docs: `0.7.1`
- kro API: `kro.run/v1alpha1`

注意

- kro は `v1alpha1` のため、将来破壊的変更が入り得ます。運用では「固定バージョンでのインストール」「RGD の変更管理」「検証環境での先行テスト」を強く推奨します。
- 本ドキュメントは公式ドキュメント/Helm values/リリースマニフェストを元に、開発者が迷いがちなポイントを補足しながら再構成しています。

## まず読みたい順

1. `refs/kro/01-what-is-kro.md`
2. `refs/kro/02-installation.md`
3. `refs/kro/03-authoring-rgd.md`
4. `refs/kro/06-instances-and-lifecycle.md`
5. `refs/kro/10-troubleshooting.md`
6. `refs/kro/11-static-analysis.md`

## 目次

- `refs/kro/01-what-is-kro.md`
  - kro の狙い、何が嬉しいか、全体アーキテクチャ、用語
- `refs/kro/02-installation.md`
  - Helm/マニフェストでの導入、アップグレード、RBAC、メトリクス
- `refs/kro/03-authoring-rgd.md`
  - ResourceGraphDefinition (RGD) の書き方(構造/ルール/落とし穴)
- `refs/kro/04-simpleschema-reference.md`
  - SimpleSchema の詳細仕様(型、マーカー、custom types、status)
- `refs/kro/05-cel-reference.md`
  - CEL の実践ガイド(kro での書き方、型、関数、optional `?` 等)
- `refs/kro/06-instances-and-lifecycle.md`
  - 生成された CRD のインスタンス運用、ラベル/ApplySet、状態/デバッグ
- `refs/kro/07-advanced-topics.md`
  - Access Control(RBAC)、RGD chaining、チューニング、メトリクス、Argo CD
- `refs/kro/08-cookbook-examples.md`
  - よくある設計パターンとサンプル(最小〜応用)
- `refs/kro/09-api-reference.md`
  - RGD CRD フィールドと Helm values の要点まとめ
- `refs/kro/10-troubleshooting.md`
  - RGD/インスタンスのトラブル切り分け、よくある落とし穴
- `refs/kro/11-static-analysis.md`
  - 静的解析(ステージ/型互換/AST/DAG)の理解を深める

## Examples

- `refs/kro/examples/README.md`

## 公式情報(一次ソース)

- Docs: https://kro.run/docs/overview
- API Reference: https://kro.run/api/reference
- GitHub: https://github.com/kubernetes-sigs/kro
- Releases: https://github.com/kubernetes-sigs/kro/releases
