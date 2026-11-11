import SwiftUI
import MetamonServices

/// サイドバーのアプリの一覧。iOS の AppSwitcherSheet に当たる(Mac では常に出す)。
/// ↑↓ で選ぶと中央が切り替わり、Return / ダブルクリックで中央へ移る(#934)。
/// 「作ったアプリ」「共有されたアプリ」(シェアから追加・参加)と「同梱の例」に分ける(#969)。並べ替えは区分の中で。作ったアプリは右クリックで名前の変更・上へ / 下へ・削除(確認あり)、
/// ドラッグで並べ替え、Delete キーで削除。同梱の例は右クリックの「複製」(作ったアプリに足す・#1285)と、会話で直した例なら「元の例に戻す」(確認あり・#1283)。
/// どの行も右クリックの「新しいウインドウで開く」・⌥クリックで別のウインドウに開ける(#1853)。
/// 検索(#1282)で絞った一覧を受け取る。行の頭にアプリの画面の写し(#1281・無ければ頭の文字)、右に新着の数(#969)。
struct AppSidebar: View {
    let created: [SampleApp]
    /// 共有されたアプリ(#969)。作ったアプリと同じ操作(名前・複製・並べ替え・削除)
    var shared: [SampleApp] = []
    let examples: [SampleApp]
    /// 新着の数(#969)。0 なら出さない
    var newItems: (String) -> Int = { _ in 0 }
    /// 行の写し(#1281)
    var thumbnail: (String) -> NSImage? = { _ in nil }
    /// 検索欄の文字(#1282)。絞り込みは呼ぶ側で済ませてある。並べ替えは絞っていない時だけ
    var searchText = ""
    @Binding var selection: SampleApp.ID?
    var onOpen: () -> Void = {}
    /// 確認の後に呼ぶ(選択の付け替えはウインドウ側)
    var onDelete: (String) -> Void = { _ in }
    /// 「複製」(#1285)。作ったアプリも同梱の例も
    var onDuplicate: (String) -> Void = { _ in }
    /// 「元の例に戻す」の確認の後に呼ぶ(#1283)
    var onResetExample: (String) -> Void = { _ in }
    /// 「新しいウインドウで開く」(#1853)
    var onOpenInNewWindow: (String) -> Void = { _ in }
    /// サーバーの一覧を初めて読んでいる(前回の一覧が無い・#1057)。作ったアプリの所にスケルトンを出す
    var loadingServerApps = false
    /// サーバーに届かず、前回の一覧を出している(#1057)
    var offline = false

    @Environment(MacAppLibrary.self) private var library
    @State private var renameTarget: SampleApp?
    @State private var renameText = ""
    @State private var deleteTarget: SampleApp?
    @State private var resetTarget: SampleApp?

    /// 作ったアプリ・共有されたアプリ(右クリックの管理・Delete キーの対象)
    private var manageable: [SampleApp] { created + shared }

    private func section(of id: String) -> [SampleApp] {
        shared.contains { $0.id == id } ? shared : created
    }

    private var isSearching: Bool { !searchText.trimmingCharacters(in: .whitespaces).isEmpty }

    var body: some View {
        List(selection: $selection) {
            if !created.isEmpty {
                Section {
                    ForEach(created) { row($0) }
                        .onMove(perform: isSearching ? nil : { move(created, fromOffsets: $0, toOffset: $1) })
                } header: {
                    Text("作ったアプリ")
                } footer: {
                    if offline { offlineNote }
                }
            } else if loadingServerApps && !isSearching {
                Section("作ったアプリ") {
                    ForEach(0..<3, id: \.self) { _ in skeletonRow }
                }
            }
            if !shared.isEmpty {
                Section("共有されたアプリ") {
                    ForEach(shared) { row($0) }
                        .onMove(perform: isSearching ? nil : { move(shared, fromOffsets: $0, toOffset: $1) })
                }
            }
            if !examples.isEmpty {
                Section("同梱の例") {
                    ForEach(examples) { row($0) }
                }
            }
        }
        .contextMenu(forSelectionType: SampleApp.ID.self) { ids in
            if ids.count == 1, let id = ids.first, let app = manageable.first(where: { $0.id == id }) {
                openInNewWindowItem(id)
                manageItems(app)
            } else if ids.count == 1, let id = ids.first, let app = examples.first(where: { $0.id == id }) {
                openInNewWindowItem(id)
                Button("複製") { onDuplicate(id) }
                if library.isEditedExample(id) {
                    Button("元の例に戻す…") { resetTarget = app }
                }
            }
        } primaryAction: { ids in
            guard let id = ids.first else { return }
            selection = id
            onOpen()
        }
        .onDeleteCommand {
            guard let id = selection, let app = manageable.first(where: { $0.id == id }) else { return }
            deleteTarget = app
        }
        .overlay {
            if manageable.isEmpty && examples.isEmpty && isSearching {
                ContentUnavailableView.search(text: searchText)
            } else if manageable.isEmpty && examples.isEmpty {
                MacEmptyState(pose: .tea, title: "アプリがありません",
                              mascotSize: MacTheme.Mascot.small, showsBackdrop: false)
            }
        }
        .navigationTitle("アプリ")
        .accessibilityIdentifier("shell.sidebar") // UI テスト(#1140)
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

    /// 読み込み中の行(#1057)。選べない・中身の形だけ
    private var skeletonRow: some View {
        HStack(spacing: 8) {
            SidebarThumbnail(name: " ", image: nil)
            VStack(alignment: .leading, spacing: MacTheme.Spacing.rowLine) {
                Text(verbatim: "アプリの名前を読み込み中")
                    .font(MacTheme.Fonts.sidebarTitle)
                    .lineLimit(1)
            }
        }
        .padding(.vertical, 2)
        .redacted(reason: .placeholder)
        .selectionDisabled()
        .accessibilityLabel(Text("アプリの一覧を読み込んでいます"))
        .help("アプリの一覧を読み込んでいます")
    }

    private var offlineNote: some View {
        Label("オフラインです。保存済みの一覧を表示しています", systemImage: "wifi.slash")
            .font(MacTheme.Fonts.sidebarDetail)
            .macSecondaryText() // コントラストを上げる時は本文の色(#1206)
            .help("サーバーに届きませんでした。つながると一覧を取り直します。")
    }

    private func row(_ app: SampleApp) -> some View {
        let count = newItems(app.id)
        return HStack(spacing: 8) {
            SidebarThumbnail(name: app.name, image: thumbnail(app.id))
            VStack(alignment: .leading, spacing: MacTheme.Spacing.rowLine) {
                Text(app.name)
                    .font(MacTheme.Fonts.sidebarTitle)
                    .fontWeight(count > 0 ? .semibold : nil) // 未読の印(#969)
                    .lineLimit(1)
                if let description = app.description {
                    Text(description)
                        .font(MacTheme.Fonts.sidebarDetail)
                        .macSecondaryText() // コントラストを上げる時は本文の色(#1206)
                        .lineLimit(1)
                }
            }
            if count > 0 {
                Spacer(minLength: 4)
                NewItemsBadge(count: count)
            }
        }
        .padding(.vertical, 2)
        .accessibilityElement(children: .combine)
        .accessibilityLabel(Text(verbatim: MacSidebarBadges.accessibilityLabel(name: app.name, count: count)))
        .tag(app.id)
    }

    /// 区分の中のドラッグ(#969)。全体の並びは区分の位置を保って入れ直す
    private func move(_ section: [SampleApp], fromOffsets source: IndexSet, toOffset destination: Int) {
        library.setOrder(MacSidebarSections.moved(library.createdIds, section: section.map(\.id),
                                                  fromOffsets: source, toOffset: destination))
    }

    /// 別のウインドウで開く(#1853)。行の ⌥クリックと同じ
    @ViewBuilder private func openInNewWindowItem(_ id: String) -> some View {
        Button("新しいウインドウで開く") { onOpenInNewWindow(id) }
        Divider()
    }

    /// 作ったアプリの右クリック(iOS のカードの「…」と同じ並び: 名前 → 移動 → 削除)
    @ViewBuilder private func manageItems(_ app: SampleApp) -> some View {
        Button("名前を変える…") {
            renameText = app.name
            renameTarget = app
        }
        Button("複製") { onDuplicate(app.id) }
        let ids = section(of: app.id).map(\.id)
        if !isSearching, ids.count > 1, let index = ids.firstIndex(of: app.id) {
            Divider()
            Button("上へ移動") { library.setOrder(MacSidebarSections.moved(library.createdIds, section: ids, id: app.id, to: index - 1)) }
                .disabled(index == 0)
            Button("下へ移動") { library.setOrder(MacSidebarSections.moved(library.createdIds, section: ids, id: app.id, to: index + 1)) }
                .disabled(index == ids.count - 1)
        }
        Divider()
        Button("削除…", role: .destructive) { deleteTarget = app }
    }
}

/// 行の右の新着の数(#969)。読み上げは行の名前に含める
private struct NewItemsBadge: View {
    let count: Int

    var body: some View {
        Text(verbatim: MacSidebarBadges.badgeText(count))
            .font(.caption2.weight(.semibold).monospacedDigit())
            .foregroundStyle(.white)
            .padding(.horizontal, 6)
            .padding(.vertical, 1)
            .background(Capsule().fill(Color.accentColor))
            .help(L10n.tr("新着 \(count) 件"))
            .accessibilityHidden(true)
    }
}
