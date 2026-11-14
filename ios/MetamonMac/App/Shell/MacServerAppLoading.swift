import SwiftUI

/// 中身(DSL)をまだ取っていないサーバーのアプリを開いた時の中央(#1057)。取れたら `AppCanvas` に替わる。
/// 取れなかったら(オフラインなど)「もう一度」を出す
struct MacServerAppLoading: View {
    let app: SampleApp
    let failed: Bool
    var retry: () -> Void

    var body: some View {
        VStack(spacing: MacTheme.Spacing.group) {
            if failed {
                MacEmptyState(pose: .tea, title: "アプリを開けませんでした",
                              message: "サーバーに届きませんでした。つながってからもう一度お試しください。")
                Button("もう一度", action: retry)
                    .keyboardShortcut(.defaultAction)
                    .help("サーバーからアプリをもう一度読み込みます")
            } else {
                ProgressView()
                    .controlSize(.large)
                Text(L10n.tr("「\(app.name)」を読み込んでいます"))
                    .foregroundStyle(.secondary)
                    .help("サーバーからアプリを読み込んでいます")
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}
