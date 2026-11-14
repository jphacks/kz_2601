import SwiftUI
import MetamonDSL
import MetamonRenderer

/// 画面の並びを手で直す編集(#2323・iOS の `AppRuntime+GridEdit` の Mac 版)。
/// 入口は表示メニュー「画面の並びを編集」とツールバーのボタン。部品はクリック・ドラッグで動かし、右クリックのメニューでも動かせる。
/// 「完了」で版の一覧(`MacAppLibrary`)に新しい版として保存し(「変更を元に戻す」で戻せる)、Esc・「キャンセル」で捨てる。
extension MacAppRuntime {
    /// 編集に入れるか: 根の画面(シート・ダイアログが無い)で、area 付きの部品がある普通の画面だけ
    var canStartGridEdit: Bool {
        guard onSaveGridEdit != nil, gridEdit == nil, !dsl.isVoiceOnly,
              sheet == nil, fullScreen == nil, !isShowingDialog,
              let screen = dsl.screen(currentScreen) else { return false }
        return screen.isGridEditable
    }

    func startGridEdit() {
        guard canStartGridEdit, let index = dsl.screens.firstIndex(where: { $0.id == currentScreen }) else {
            toast = L10n.tr("この画面には動かせる部品がありません")
            return
        }
        withMacAnimation(.easeOut(duration: 0.2)) {
            gridEdit = GridEditSession(draft: GridEditDraft(screen: dsl.screens[index], screenIndex: index))
        }
    }

    func cancelGridEdit() {
        guard !gridEditSaving else { return }
        withMacAnimation(.easeOut(duration: 0.2)) { gridEdit = nil }
    }

    /// 「完了」: 変えた所があれば保存する。保存できたら版が進み、中央はランタイムごと作り直される
    func finishGridEdit() async {
        guard let session = gridEdit, !gridEditSaving else { return }
        let draft = session.draft
        guard draft.hasChanges, let save = onSaveGridEdit else { return cancelGridEdit() }
        gridEditSaving = true
        defer { gridEditSaving = false }
        switch await save(draft) {
        case .saved:
            gridEdit = nil
        case .conflict:
            gridEdit = nil
            toast = L10n.tr("ほかの変更が先に保存されました。もう一度やり直してください")
        case .failed:
            toast = L10n.tr("配置を保存できませんでした") // 下書きは残す(もう一度「完了」を押せる)
        }
    }
}
