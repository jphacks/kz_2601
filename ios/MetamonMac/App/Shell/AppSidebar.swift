import SwiftUI
import MetamonServices

/// サイドバーのアプリの一覧。iOS の AppSwitcherSheet に当たる(Mac では常に出す)。
/// ↑↓ で選ぶと中央が切り替わり、Return / ダブルクリックで中央へ移る(#934)。
/// 「作ったアプリ」と「同梱の例」に分ける(#969)。作ったアプリは右クリックで名前の変更・上へ / 下へ・削除(確認あり)、
/// ドラッグで並べ替え、Delete キーで削除。同梱の例は整理できない。検索(#1282)で絞った一覧を受け取る。サムネイル・新着の数は後の Issue。
struct AppSidebar: View {
    let created: [SampleApp]
    let examples: [SampleApp]
    /// 検索欄の文字(#1282)。絞り込みは呼ぶ側で済ませてある。並べ替えは絞っていない時だけ
    var searchText = ""
    @Binding var selection: SampleApp.ID?
    var onOpen: () -> Void = {}
    /// 確認の後に呼ぶ(選択の付け替えはウインドウ側)
    var onDelete: (String) -> Void = { _ in }

    @Environment(MacAppLibrary.self) private var library
    @State private var renameTarget: SampleApp?
    @State private var renameText = ""
    @State private var deleteTarget: SampleApp?

    private var createdIds: [String] { created.map(\.id) }

    private var isSearching: Bool { !searchText.trimmingCharacters(in: .whitespaces).isEmpty }

    var body: some View {
        List(selection: $selection) {
            if !created.isEmpty {
                Section("作ったアプリ") {
                    ForEach(created) { row($0) }
                        .onMove(perform: isSearching ? nil : { library.move(fromOffsets: $0, toOffset: $1) })
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
            if created.isEmpty && examples.isEmpty && isSearching {
                ContentUnavailableView.search(text: searchText)
            } else if created.isEmpty && examples.isEmpty {
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
        if !isSearching, created.count > 1, let index = createdIds.firstIndex(of: app.id) {
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
