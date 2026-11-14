import SwiftUI
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// 手で動かす編集モード(#1442)。今見えている画面の部品を指で動かし・大きさを変え・消して、「完了」で 1 つの版として保存する。
/// 保存は声の編集と同じ appconfig の JSON Patch(`PATCH /dsl`)なので、履歴に残り「元に戻す」で戻せる。
extension AppRuntime {
    /// 編集できる画面か(入口のボタンを出すか)。メインの層の、area 付きの部品がある普通の画面だけ。
    var canStartGridEdit: Bool {
        guard !dsl.isVoiceOnly, morphJob == nil, buildStage == nil, gridEdit == nil else { return false }
        let visible = visibleScreen
        guard visible.layer == .main, let screen = dsl.screen(visible.route.screen) else { return false }
        return screen.isGridEditable
    }

    func startGridEdit() {
        let visible = visibleScreen
        guard canStartGridEdit, let index = dsl.screens.firstIndex(where: { $0.id == visible.route.screen }) else {
            toast = L10n.tr("この画面には動かせる部品がありません")
            return
        }
        showVoiceSheet = false
        gridEditBaseVersion = document.version
        withAnimation(.easeOut(duration: 0.2)) {
            gridEdit = GridEditSession(draft: GridEditDraft(screen: dsl.screens[index], screenIndex: index))
        }
    }

    func cancelGridEdit() {
        withAnimation(.easeOut(duration: 0.2)) { gridEdit = nil }
    }

    /// 「完了」: 変えた所があれば JSON Patch を保存して新しい版にする。サーバーに版が無い時(モック)は端末だけで当てる。
    func finishGridEdit() async {
        guard let session = gridEdit, !gridEditSaving else { return }
        let draft = session.draft
        guard draft.hasChanges else { cancelGridEdit(); return }
        let before = document
        let keep = (tab: selectedTab, paths: paths)
        let summary = L10n.tr("部品の配置を変えました")
        gridEditSaving = true
        defer { gridEditSaving = false }

        var next: AppDocument
        if services.isLive {
            do {
                next = try await services.appConfig.patchDSL(appId: before.appId, baseVersion: gridEditBaseVersion,
                                                             patch: draft.patch, summary: summary)
            } catch {
                print("[Metamon] grid edit save failed: \(error)")
                if (error as? APIError)?.status == 409 {
                    // ほかの端末・声の編集で先に版が進んだ。今の版で開き直してもらう
                    toast = L10n.tr("ほかの変更が先に保存されました。もう一度やり直してください")
                    cancelGridEdit()
                    if let latest = try? await services.appConfig.fetchDocument(appId: before.appId) { hotReload(latest) }
                } else {
                    toast = L10n.tr("配置を保存できませんでした")
                }
                return
            }
        } else {
            next = before
            next.dsl = draft.applied(to: before.dsl)
            next.version = before.version + 1
        }
        guard document.appId == before.appId else { return }
        remember(before)
        gridEdit = nil
        let moved = draft.items.filter { !$0.removed && $0.area != $0.original }
            .map { ChangeHighlight.key(screen: draft.screenId, component: $0.componentId) }
        apply(next, highlight: ChangeHighlight(components: Set(moved), screens: moved.isEmpty ? [] : [draft.screenId]))
        // 編集していた画面に留まる(apply は最初のタブに戻す)
        if next.dsl.app.tabs.contains(where: { $0.screen == keep.tab }) { selectedTab = keep.tab }
        paths = keep.paths.filter { _, stack in stack.allSatisfy { next.dsl.screen($0.screen) != nil } }
        startSync()
        summaryBanner = SummaryBanner(text: summary, highlight: nil, action: canUndo ? .undo : nil)
    }
}
