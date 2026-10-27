import SwiftUI
import MetamonServices

/// 変身一覧の整理(#277)の UI 状態。名前の入力ダイアログは `AppSwitcherSheet` が出す(カードのメニューは閉じてしまうため)。
@MainActor
@Observable
final class AppListUI {
    static let shared = AppListUI()

    /// 名前を変える対象
    var renameTarget: AppSummary?
    /// 名前の入力欄
    var renameText = ""
    /// 複製中の appId(二度押しを防ぐ)
    var duplicating: Set<String> = []

    func beginRename(_ app: AppSummary) {
        renameText = app.name
        renameTarget = app
    }
}

/// カードの「…」に足す項目: 名前を変える・複製・左へ / 右へ移動。
/// 審査用ビルド(#284)でも出す(どれも自分のアプリの中で閉じ、外部への公開やシェアを伴わないため)。
struct AppManageMenuItems: View {
    let app: AppSummary
    @Environment(AppRuntime.self) private var rt
    @State private var ui = AppListUI.shared

    private var order: [String] { rt.visibleApps.map(\.id) }
    private var index: Int? { order.firstIndex(of: app.id) }

    var body: some View {
        Section {
            Button { ui.beginRename(app) } label: {
                Label("名前を変える", systemImage: "pencil")
            }
            .accessibilityIdentifier("switcher.rename")
            Button { duplicate() } label: {
                Label("複製", systemImage: "plus.square.on.square")
            }
            .disabled(ui.duplicating.contains(app.id))
            .accessibilityIdentifier("switcher.duplicate")
        }
        if order.count > 1, let index {
            Section {
                Button { move(to: index - 1) } label: {
                    Label("左へ移動", systemImage: "arrow.left")
                }
                .disabled(index == 0)
                .accessibilityIdentifier("switcher.moveLeft")
                Button { move(to: index + 1) } label: {
                    Label("右へ移動", systemImage: "arrow.right")
                }
                .disabled(index == order.count - 1)
                .accessibilityIdentifier("switcher.moveRight")
            }
        }
    }

    private func duplicate() {
        guard ui.duplicating.insert(app.id).inserted else { return }
        Task {
            await rt.duplicateApp(app)
            ui.duplicating.remove(app.id)
        }
    }

    private func move(to index: Int) {
        Task { await rt.moveApp(app.id, to: index) }
    }
}

/// 名前の入力ダイアログ。
struct AppRenameAlert: ViewModifier {
    @Environment(AppRuntime.self) private var rt
    @State private var ui = AppListUI.shared

    func body(content: Content) -> some View {
        content.alert(L10n.tr("名前を変える"), isPresented: Binding(
            get: { ui.renameTarget != nil },
            set: { if !$0 { ui.renameTarget = nil } }
        )) {
            TextField(L10n.tr("アプリの名前"), text: $ui.renameText)
                .accessibilityIdentifier("switcher.rename.field")
            Button(L10n.tr("変更")) {
                guard let app = ui.renameTarget else { return }
                let text = ui.renameText
                Task { await rt.renameApp(app, to: text) }
            }
            .disabled(AppListEdit.normalizedName(ui.renameText) == nil)
            .accessibilityIdentifier("switcher.rename.confirm")
            Button("キャンセル", role: .cancel) {}
        } message: {
            Text(L10n.tr("変身一覧とアプリの上に出る名前です(\(AppListEdit.maxNameLength)文字まで)。"))
        }
    }
}

/// カードを長押しでドラッグ → 別のカードに落とすとその位置へ動かす。
struct AppCardReorder: ViewModifier {
    let app: AppSummary
    @Environment(AppRuntime.self) private var rt

    func body(content: Content) -> some View {
        content
            .draggable(app.id)
            .dropDestination(for: String.self) { ids, _ in
                let order = rt.visibleApps.map(\.id)
                guard let id = ids.first, id != app.id, order.contains(id), let to = order.firstIndex(of: app.id) else { return false }
                Task { await rt.moveApp(id, to: to) }
                return true
            }
    }
}
