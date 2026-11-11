import SwiftUI
import MetamonServices

/// サイドバーのアプリの一覧。iOS の AppSwitcherSheet に当たる(Mac では常に出す)。
/// ↑↓ で選ぶと中央が切り替わり、Return / ダブルクリックで中央へ移る(#934)。
/// 「作ったアプリ」と「同梱の例」に分ける(#969)。作ったアプリは右クリックで名前の変更・上へ / 下へ・削除(確認あり)、
/// ドラッグで並べ替え、Delete キーで削除。同梱の例は整理できないが、会話で直した例は右クリックで「元の例に戻す」(確認あり・#1283)。
/// サムネイル・検索・新着の数は後の Issue。
struct AppSidebar: View {
    let created: [SampleApp]
    let examples: [SampleApp]
    @Binding var selection: SampleApp.ID?
    var onOpen: () -> Void = {}
    /// 確認の後に呼ぶ(選択の付け替えはウインドウ側)
    var onDelete: (String) -> Void = { _ in }
    /// 「元の例に戻す」の確認の後に呼ぶ(#1283)
    var onResetExample: (String) -> Void = { _ in }

    @Environment(MacAppLibrary.self) private var library
    @State private var renameTarget: SampleApp?
    @State private var renameText = ""
    @State private var deleteTarget: SampleApp?
    @State private var resetTarget: SampleApp?

    private var createdIds: [String] { created.map(\.id) }

    var body: some View {
        List(selection: $selection) {
            if !created.isEmpty {
                Section("作ったアプリ") {
                    ForEach(created) { row($0) }
                        .onMove { library.move(fromOffsets: $0, toOffset: $1) }
                }
            }
            if !examples.isEmpty {
                Section("同梱の例") {
                    ForEach(examples) { row($0) }
                }
            }
        }
        .contextMenu(forSelectionType: SampleApp.ID.self) { ids in
            if ids.count == 1, let id = ids.first, let app = created.first(where: { $0.id == id }) {
                manageItems(app)
            } else if ids.count == 1, let id = ids.first, library.isEditedExample(id),
                      let app = examples.first(where: { $0.id == id }) {
                Button("元の例に戻す…") { resetTarget = app }
            }
        } primaryAction: { ids in
            guard let id = ids.first else { return }
            selection = id
            onOpen()
        }
        .onDeleteCommand {
            guard let id = selection, let app = created.first(where: { $0.id == id }) else { return }
            deleteTarget = app
        }
        .overlay {
            if created.isEmpty && examples.isEmpty {
                MacEmptyState(pose: .tea, title: "アプリがありません",
                              mascotSize: MacTheme.Mascot.small, showsBackdrop: false)
            }
        }
        .navigationTitle("アプリ")
        .alert("名前を変える", isPresented: Binding(get: { renameTarget != nil }, set: { if !$0 { renameTarget = nil } })) {
            TextField("アプリの名前", text: $renameText)
            Button("変更") {
                guard let app = renameTarget else { return }
                library.rename(app.id, to: renameText)
            }
            .disabled(AppListEdit.normalizedName(renameText) == nil)
            Button("キャンセル", role: .cancel) {}
        } message: {
            Text("アプリの一覧と画面の上に出る名前です(\(AppListEdit.maxNameLength)文字まで)。")
        }
        .alert(L10n.tr("「\(deleteTarget?.name ?? "")」を削除しますか?"),
               isPresented: Binding(get: { deleteTarget != nil }, set: { if !$0 { deleteTarget = nil } })) {
            Button("削除", role: .destructive) {
                guard let app = deleteTarget else { return }
                onDelete(app.id)
            }
            Button("キャンセル", role: .cancel) {}
        } message: {
            Text("元に戻せません。")
        }
        .alert(L10n.tr("「\(resetTarget?.name ?? "")」を元の例に戻しますか?"),
               isPresented: Binding(get: { resetTarget != nil }, set: { if !$0 { resetTarget = nil } })) {
            Button("元の例に戻す", role: .destructive) {
                guard let app = resetTarget else { return }
                onResetExample(app.id)
            }
            Button("キャンセル", role: .cancel) {}
        } message: {
            Text("会話で直した分は消えます。")
        }
    }

    private func row(_ app: SampleApp) -> some View {
        VStack(alignment: .leading, spacing: MacTheme.Spacing.rowLine) {
            Text(app.name)
                .font(MacTheme.Fonts.sidebarTitle)
                .lineLimit(1)
            if let description = app.description {
                Text(description)
                    .font(MacTheme.Fonts.sidebarDetail)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
        }
        .padding(.vertical, 2)
        .tag(app.id)
    }

    /// 作ったアプリの右クリック(iOS のカードの「…」と同じ並び: 名前 → 移動 → 削除)
    @ViewBuilder private func manageItems(_ app: SampleApp) -> some View {
        Button("名前を変える…") {
            renameText = app.name
            renameTarget = app
        }
        if created.count > 1, let index = createdIds.firstIndex(of: app.id) {
            Divider()
            Button("上へ移動") { library.move(app.id, to: index - 1) }
                .disabled(index == 0)
            Button("下へ移動") { library.move(app.id, to: index + 1) }
                .disabled(index == created.count - 1)
        }
        Divider()
        Button("削除…", role: .destructive) { deleteTarget = app }
    }
}
