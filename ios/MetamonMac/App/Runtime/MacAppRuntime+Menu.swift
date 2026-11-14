import Foundation
import MetamonDSL
import MetamonRenderer

/// メニューから今の画面に向けて行う操作(`MacCommands`)。
/// - ファイル > 保存 ⌘S・⌘Return(#1138): いちばん上の画面のフォームを、保存バーと同じ `saveRecord` で保存する
/// - ファイル > 書き出し(CSV / PDF)・プリント ⌘P(#1139): いちばん上の画面の一覧(list / table)の記録を `export` と同じ流れで出す
extension MacAppRuntime {
    // MARK: - 保存(#1138)

    /// 保存できるフォーム: いちばん上の画面のフォームと、その画面の saveRecord(保存バーと同じもの)
    var menuSaveTarget: (form: FormDraft, action: Action, route: Route)? {
        let top = NavigationResolver.top(navigationState, dsl: dsl)
        guard let screen = dsl.screen(top.screen), screen.entity != nil,
              let action = Self.saveAction(in: screen),
              let form = openForms.form(at: top), !form.isReadOnly else { return nil }
        return (form, action, top)
    }

    /// メニューの「保存」を押せるか。フォームが無い・何も変えていない・確認ダイアログが出ている間は押せない
    var canSaveFromMenu: Bool {
        guard !isShowingDialog, let target = menuSaveTarget else { return false }
        return target.form.hasUnsavedChanges
    }

    /// ⌘S・⌘Return。保存バーを押したのと同じ(必須が空なら弾いて欄に印を付ける・保存したら閉じる / 進む)
    func saveFromMenu() async {
        guard canSaveFromMenu, let target = menuSaveTarget else { return }
        await KeyboardState.finishEditing() // 変換中の文字を確定させてから保存する(#627)
        await run(target.action, ActionContext(route: target.route, form: target.form))
    }

    /// 画面の saveRecord: 保存バーに出す toolbar のものを先に、無ければ画面の中のボタン
    static func saveAction(in screen: Screen) -> Action? {
        if let pinned = screen.toolbar.first(where: { $0.action.type == .saveRecord }) { return pinned.action }
        return screen.allComponents.lazy.compactMap(\.action).first { $0.type == .saveRecord }
    }

    // MARK: - 書き出し・プリント(#1139)

    /// いちばん上の画面の一覧(list / table)の書き出し。一覧が無ければ nil
    func menuExportAction(format: String) -> Action? {
        let top = NavigationResolver.top(navigationState, dsl: dsl)
        guard let screen = dsl.screen(top.screen),
              let list = screen.allComponents.first(where: { ($0.type == .list || $0.type == .table) && $0.entity != nil }),
              let entity = list.entity, dsl.entity(entity) != nil else { return nil }
        var action = Action(type: .export, entity: entity)
        action.format = format
        action.filters = list.filters // 画面に出ている絞り込みと同じ記録を出す
        return action
    }

    /// メニューの「書き出し」「プリント」を押せるか
    var canExportFromMenu: Bool { !isShowingDialog && menuExportAction(format: "csv") != nil }

    /// 書き出し(CSV / PDF): export アクションと同じく、保存・共有・プリントを選んでから出す
    func exportFromMenu(format: String) async {
        guard !isShowingDialog, let action = menuExportAction(format: format) else { return }
        await run(action, ActionContext(route: NavigationResolver.top(navigationState, dsl: dsl)))
    }

    /// ⌘P: 一覧をそのままプリントする(export の「プリント…」と同じ PDF の表)
    func printFromMenu() async {
        guard !isShowingDialog, let action = menuExportAction(format: "pdf"), let entity = action.entity,
              let entityDef = dsl.entity(entity) else { return }
        await load(entities: Set([entity] + entityDef.fields.compactMap(\.refEntity)))
        printExport(action, screen: NavigationResolver.top(navigationState, dsl: dsl).screen)
    }

    /// 中央の列に確認(削除・破棄・カレンダー・AI の同意)が出ている
    var isShowingDialog: Bool {
        confirm != nil || discardRequest != nil || calendarDraft != nil || needsAIConsent
    }
}
