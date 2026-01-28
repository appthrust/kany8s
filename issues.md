# Kany8s 実装レビュー: 問題点一覧

作成日: 2026-01-28

このドキュメントは、Kany8s の実装をレビューした結果発見された問題点をまとめたものです。

---

## 概要

| 重要度 | 件数 |
|--------|------|
| P0 (高) | 5件 |
| P1 (中) | 5件 |
| P2 (低) | 3件 |

---

## 更新 (2026-01-28)

この issues.md は 2026-01-28 時点の実装に合わせて、誤検知/誤提案の注記を追記しました。

- Issue #6/#7/#9 は対応不要（いずれも誤検知/誤提案）
- Issue #1/#5 の「`RequeueAfter` + error を返す」提案は controller-runtime の仕様上無効（error が non-nil の場合、Result の RequeueAfter は無視される）

---

## P0: 高重要度（早急な修正が必要）

### Issue #1: Secret取得エラーの無限リトライ

**ファイル**: `internal/controller/kany8scontrolplane_controller.go:213-221`

**問題**:
```go
if err := r.Get(ctx, client.ObjectKey{Name: sourceName, Namespace: sourceNamespace}, sourceSecret); err != nil {
    return err  // ← RequeueAfter なしで即座にリトライ
}
```

- `NotFound`エラーと権限エラーを区別していない
- 権限不足の場合、何度Reconcileしても失敗するが即座にリトライされる
- APIサーバーへの負荷増加、Thundering Herd問題を引き起こす

**推奨修正**:
```go
if err := r.Get(ctx, client.ObjectKey{Name: sourceName, Namespace: sourceNamespace}, sourceSecret); err != nil {
    if apierrors.IsNotFound(err) {
        log.Info("source kubeconfig secret not found, waiting", "name", sourceName)
        return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
    }
    return ctrl.Result{RequeueAfter: time.Minute}, err
}
```

**更新注記 (2026-01-28)**:

- controller-runtime では error を返すと RequeueAfter は無視されるため、上記の `RequeueAfter` + error は無効です。

---

### Issue #2: Watcherチャネル満杯時のイベント喪失

**ファイル**: `internal/dynamicwatch/watcher.go:108-111`

**問題**:
```go
select {
case w.events <- event.GenericEvent{Object: u}:
default:  // ← イベントが捨てられる
}
```

- チャネルが満杯（1024バッファ）の場合、イベントが破棄される
- kro instanceの状態変化（ready=true等）が検知されない可能性
- ControlPlaneがready状態に達してもコントローラが気づかない

**推奨修正**:
- チャネルサイズの増加
- または、ブロッキング送信 + タイムアウト付きコンテキスト
- または、イベント喪失時のメトリクス/ログ出力

---

### Issue #3: Kany8sClusterコントローラのテストが空

**ファイル**: `internal/controller/infrastructure/kany8scluster_controller_test.go`

**問題**:
```go
It("should successfully reconcile the resource", func() {
    // TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
    // ← テストボディが空!
})
```

- `status.initialization.provisioned = true` の設定がテストされていない
- Conditionsの設定がテストされていない
- リグレッション検知が不可能

**推奨修正**:
- Kany8sCluster作成時に`provisioned=true`が設定されることをテスト
- Ready Conditionが正しく設定されることをテスト

---

### Issue #4: 過度なRBAC権限（kro.run/*）

**ファイル**: `config/rbac/role.yaml:78-88`

**問題**:
```yaml
- apiGroups:
  - kro.run
  resources:
  - '*'
  verbs:
  - create
  - get
  - list
  - patch
  - update
  - watch
```

- `kro.run`グループの全リソースに対するCRUD権限
- ResourceGraphDefinition自体への変更権限も含まれる
- 最小権限の原則に反する

**推奨修正**:
- 動的GVKの制約上、MVPでは許容だがドキュメント化が必要
- 将来的には生成されるinstance GVKのみに制限（例: `ekscontrolplanes`, `gkecontrolplanes`）
- `docs/security.md` にRBACトレードオフを記載（既存）

---

### Issue #5: KubeconfigSecretエラー時のRequeueAfter欠落

**ファイル**: `internal/controller/kany8scontrolplane_controller.go:143-145`

**問題**:
```go
if err := r.reconcileKubeconfigSecret(ctx, cp, instance); err != nil {
    log.Error(err, "reconcile kubeconfig secret")
    return ctrl.Result{}, err  // ← RequeueAfter がない
}
```

- 一時的なネットワークエラーでも即座にリトライ
- 指数バックオフなしでAPIサーバーに負荷

**推奨修正**:
```go
if err := r.reconcileKubeconfigSecret(ctx, cp, instance); err != nil {
    log.Error(err, "reconcile kubeconfig secret")
    return ctrl.Result{RequeueAfter: 30 * time.Second}, err
}
```

**更新注記 (2026-01-28)**:

- controller-runtime では error を返すと RequeueAfter は無視されるため、上記の `RequeueAfter` + error は無効です。

---

## P1: 中重要度

### Issue #6: Patch後の状態不整合

**更新注記 (2026-01-28)**:

- `client.MergeFrom` による Patch は OptimisticLock を使っていないため、指摘のような ResourceVersion 起因の conflict は前提になりません。
- spec/status は別 subresource として Patch しており、現状の形で問題ありません（対応不要）。

**ファイル**: `internal/controller/kany8scontrolplane_controller.go:158-181`

**問題**:
```go
if cp.Spec.ControlPlaneEndpoint != cpEndpoint {
    before := cp.DeepCopy()
    cp.Spec.ControlPlaneEndpoint = cpEndpoint
    if err := r.Patch(ctx, cp, client.MergeFrom(before)); err != nil {
        return ctrl.Result{}, err
    }
}
// 次のPatchでもbefore := cp.DeepCopy()を使用
// cpのResourceVersionが古いままで conflict の可能性
```

- 複数のPatch操作が連続実行される
- 各Patchで`before := cp.DeepCopy()`を使用するため、中間状態での不整合が発生可能
- 最初のPatch成功後、`cp`のResourceVersionが更新されないまま次のPatchが実行される

**推奨修正**:
- Patch後に`cp`を再取得
- または、spec/status更新を単一のPatchにまとめる

---

### Issue #7: ObservedGenerationが一部Conditionにのみ設定

**更新注記 (2026-01-28)**:

- `sigs.k8s.io/cluster-api/util/conditions.Set` は `ObservedGeneration` を自動設定するため、ここでの「未設定」指摘は誤検知です（対応不要）。

**ファイル**: `internal/controller/kany8scontrolplane_controller.go:264-297`

**問題**:
```go
conditions.Set(cp, metav1.Condition{
    Type:    conditionTypeReady,
    Status:  metav1.ConditionTrue,
    Reason:  reason,
    Message: message,
    // ← ObservedGeneration がない
})
```

- `ResourceGraphDefinitionResolved` Conditionには`ObservedGeneration`が設定されている
- `Ready`と`Creating` Conditionには設定されていない
- CAPI contract要件: 全Conditionに`ObservedGeneration`が必須

**推奨修正**:
```go
conditions.Set(cp, metav1.Condition{
    Type:               conditionTypeReady,
    Status:             metav1.ConditionTrue,
    Reason:             reason,
    Message:            message,
    ObservedGeneration: cp.Generation,
})
```

---

### Issue #8: Kubeconfig内容の検証なし

**ファイル**: `internal/controller/kany8scontrolplane_controller.go:218-221`

**問題**:
```go
kc, ok := sourceSecret.Data[kubeconfig.DataKey]
if !ok {
    return fmt.Errorf("source secret %s/%s missing data[%q]", sourceNamespace, sourceName, kubeconfig.DataKey)
}
// ← kc の内容が有効な kubeconfig かどうか検証なし
```

- 破損/不正なデータがそのままターゲットSecretにコピーされる
- Clusterが不正なKubeconfigで起動される可能性

**推奨修正**:
```go
if _, err := clientcmd.Load(kc); err != nil {
    return fmt.Errorf("source secret contains invalid kubeconfig: %w", err)
}
```

---

### Issue #9: factory.Start()の重複呼び出し

**更新注記 (2026-01-28)**:

- `dynamicinformer.DynamicSharedInformerFactory.Start` は複数回呼んでも idempotent（未開始の informer のみ start）です。
- `EnsureWatch` 側で Start しているのは「controller 起動後に追加された informer を start する」ためであり、重複呼び出し自体は問題になりません（対応不要）。

**ファイル**: `internal/dynamicwatch/watcher.go:48, 92`

**問題**:
```go
func (w *Watcher) Start(ctx context.Context) error {
    w.factory.Start(stopCh)  // Line 48
}

func (w *Watcher) EnsureWatch(ctx context.Context, gvk schema.GroupVersionKind) error {
    if stopCh != nil {
        w.factory.Start(stopCh)  // Line 92 ← 重複!
    }
}
```

- `factory.Start()`が複数回呼ばれる可能性
- ゴルーチンリークのリスク

**推奨修正**:
```go
var startOnce sync.Once

func (w *Watcher) Start(ctx context.Context) error {
    startOnce.Do(func() {
        w.factory.Start(stopCh)
    })
}
```

---

### Issue #10: Watcherセットアップ失敗時のエラー無視

**ファイル**: `internal/controller/kany8scontrolplane_controller.go:104-108`

**問題**:
```go
if r.InstanceWatcher != nil {
    if err := r.InstanceWatcher.EnsureWatch(ctx, instanceGVK); err != nil {
        log.Error(err, "ensure dynamic watch for kro instance", "gvk", instanceGVK.String())
        // ← エラーを無視して続行
    }
}
```

- Watcherのセットアップが失敗してもログして続行
- kro instance更新があってもイベントが送信されない
- 時間ベースのRequeue（15秒）に依存し、リアルタイム性が失われる

**推奨修正**:
- メトリクスで監視可能にする
- または、Watcher失敗時はエラーを返してretryする

---

## P2: 低重要度

### Issue #11: 全Secretへのアクセス権限

**ファイル**: `config/rbac/role.yaml:7-17`

**問題**:
```yaml
- apiGroups:
  - ""
  resources:
  - secrets
  verbs:
  - create
  - get
  - list
  - patch
  - update
  - watch
```

- Kubeconfig Secretのみに制限すべきところ、全Secretへのアクセス権限
- 他のアプリケーションのSecretも読み取り可能

**推奨修正**:
- resourceNamesで制限（ただし動的な名前のため難しい）
- またはOPA/Kyvernoでポリシー制御

---

### Issue #12: TODOコメントの残存

**ファイル**:
- `internal/controller/kany8scontrolplane_controller.go:84-87`
- `internal/controller/infrastructure/kany8scluster_controller.go:43-46`

**問題**:
```go
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Kany8sControlPlane object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
```

Kubebuilderのscaffoldingで生成されたTODOコメントがそのまま残っています。

**推奨修正**:
- 実装が完了していればTODOコメントを削除
- または、実際のTODO項目に置き換え

---

### Issue #13: エラーメッセージへのエンドポイント情報出力

**ファイル**: `internal/controller/kany8scontrolplane_controller.go:152`

**問題**:
```go
log.Error(err, "parse kro instance status endpoint", "endpoint", instanceStatus.Endpoint)
```

- エンドポイントがまれに認証情報を含む可能性（例: Basic認証URLなど）
- 外部ログシステムへの流出リスク

**推奨修正**:
- エンドポイントをマスクしてログ出力
- または、パース失敗時はエンドポイント自体をログに含めない

---

## 今後の対応

### 短期（MVP前）
- [x] Issue #1: Secret取得エラーの修正
- [x] Issue #2: イベント喪失対策
- [x] Issue #3: Kany8sClusterテスト追加
- [x] Issue #5: KubeconfigSecret RequeueAfter追加

### 中期（GA前）
- [ ] Issue #4: RBAC権限の最小化検討
- [x] Issue #6: Patch不整合の修正（対応不要）
- [x] Issue #7: ObservedGeneration設定（対応不要）
- [x] Issue #8: Kubeconfig検証追加

### 長期（運用改善）
- [x] Issue #9: factory重複呼び出し修正（対応不要）
- [x] Issue #10: Watcher失敗時のメトリクス
- [x] Issue #11: Secret権限の制限検討
- [x] Issue #12: TODOコメント整理
- [x] Issue #13: ログマスキング
