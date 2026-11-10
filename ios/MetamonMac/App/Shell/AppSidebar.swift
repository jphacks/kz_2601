import SwiftUI

/// サイドバーのアプリの一覧。iOS の AppSwitcherSheet に当たる(Mac では常に出す)。
/// 並べ替え・削除・サムネイルは mac-047。↑↓ で選ぶと中央が切り替わり、Return / ダブルクリックで中央へ移る(#934)。
struct AppSidebar: View {
    let apps: [SampleApp]
    @Binding var selection: SampleApp.ID?
    var onOpen: () -> Void = {}

    var body: some View {
        List(apps, selection: $selection) { app in
            VStack(alignment: .leading, spacing: 2) {
                Text(app.name)
                    .lineLimit(1)
                if let description = app.description {
                    Text(description)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            .padding(.vertical, 2)
            .tag(app.id)
        }
        .contextMenu(forSelectionType: SampleApp.ID.self, menu: { _ in }) { ids in
            guard let id = ids.first else { return }
            selection = id
            onOpen()
        }
        .overlay {
            if apps.isEmpty {
                ContentUnavailableView("アプリがありません", systemImage: "square.grid.2x2")
            }
        }
        .navigationTitle("アプリ")
    }
}
