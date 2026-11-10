import SwiftUI

/// サイドバーのアプリの一覧。iOS の AppSwitcherSheet に当たる(Mac では常に出す)。
/// 並べ替え・削除・サムネイルは mac-047。
struct AppSidebar: View {
    let apps: [SampleApp]
    @Binding var selection: SampleApp.ID?

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
        .overlay {
            if apps.isEmpty {
                ContentUnavailableView("アプリがありません", systemImage: "square.grid.2x2")
            }
        }
        .navigationTitle("アプリ")
    }
}
