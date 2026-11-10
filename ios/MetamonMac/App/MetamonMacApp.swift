import SwiftUI
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// Mac の殻(#929)。今は同梱の DSL 例の一覧を出すだけ。
/// 画面の骨格(サイドバー・メニュー・設定)は #933、Firebase は #930、権限の説明は #931 で足す。
@main
struct MetamonMacApp: App {
    var body: some Scene {
        WindowGroup {
            AppListView(apps: SampleAppCatalog.load())
                .frame(minWidth: 480, minHeight: 360)
        }
    }
}

struct AppListView: View {
    let apps: [SampleApp]

    var body: some View {
        NavigationStack {
            List(apps) { app in
                VStack(alignment: .leading, spacing: 4) {
                    Text(app.name).font(.headline)
                    if let description = app.description {
                        Text(description).font(.subheadline).foregroundStyle(.secondary)
                    }
                }
                .padding(.vertical, 4)
            }
            .overlay {
                if apps.isEmpty {
                    ContentUnavailableView("アプリがありません", systemImage: "square.grid.2x2")
                }
            }
            .navigationTitle("アプリ")
        }
    }
}
