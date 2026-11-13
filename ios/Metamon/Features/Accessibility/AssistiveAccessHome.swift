import SwiftUI
import MetamonServices

/// アシスティブアクセス(#1997)の最初の画面: 生成アプリの一覧だけ(大きな行・1 行に 1 つ)。
/// 作る・設定・共有の入口は置かない。タップで切り替えて今の画面をそのまま出す。
struct AssistiveAccessAppList: View {
    @Environment(AppRuntime.self) private var rt
    /// 開くアプリが決まった(切り替えが済んだ)時。
    let onOpen: (String) -> Void
    @State private var opening: String?

    private var apps: [AppSummary] { AssistiveAccessPolicy.listedApps(rt.visibleApps) }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Theme.Spacing.md) {
                Text("アプリの一覧")
                    .font(.largeTitle.weight(.bold))
                    .accessibilityAddTraits(.isHeader)
                if rt.appsLoading && apps.isEmpty {
                    ProgressView()
                        .controlSize(.large)
                        .frame(maxWidth: .infinity, minHeight: 120)
                } else if apps.isEmpty {
                    Text("アプリがまだありません")
                        .font(.title3)
                        .foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity, minHeight: 120)
                } else {
                    ForEach(apps) { app in row(app) }
                }
            }
            .padding(Theme.Spacing.lg)
        }
        .background(Theme.Colors.canvas.ignoresSafeArea())
        .task { await rt.appsList() }
    }

    private func row(_ app: AppSummary) -> some View {
        Button { open(app) } label: {
            HStack(spacing: Theme.Spacing.md) {
                Text(app.name)
                    .font(.title2.weight(.semibold))
                    .multilineTextAlignment(.leading)
                    .frame(maxWidth: .infinity, alignment: .leading)
                if opening == app.id {
                    ProgressView()
                } else {
                    Image(systemName: "chevron.right")
                        .font(.title3.weight(.bold))
                        .foregroundStyle(.secondary)
                        .accessibilityHidden(true)
                }
            }
            .padding(.horizontal, Theme.Spacing.lg)
            .frame(maxWidth: .infinity, minHeight: 80)
            .background(Theme.Colors.surface, in: RoundedRectangle(cornerRadius: 20, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 20, style: .continuous).strokeBorder(Theme.Colors.hairline))
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .disabled(opening != nil)
        .accessibilityIdentifier("assistive.app.\(app.id)")
    }

    private func open(_ app: AppSummary) {
        opening = app.id
        Task {
            if app.id != rt.document.appId { await rt.switchApp(to: app.id) }
            opening = nil
            if rt.document.appId == app.id { onOpen(app.id) } // 切り替えに失敗したら一覧のまま(トーストで知らせる)
        }
    }
}

/// 一覧から開いた生成アプリの上に置く「一覧に戻る」(大きなボタン)。
struct AssistiveAccessBackBar: View {
    let onBack: () -> Void

    var body: some View {
        Button(action: onBack) {
            Label("一覧に戻る", systemImage: "chevron.left")
                .font(.title3.weight(.semibold))
                .frame(maxWidth: .infinity, minHeight: 56)
        }
        .buttonStyle(.borderedProminent)
        .padding(.horizontal, Theme.Spacing.md)
        .padding(.vertical, Theme.Spacing.sm)
        .background(Theme.Colors.canvas)
        .accessibilityIdentifier("assistive.back")
    }
}
