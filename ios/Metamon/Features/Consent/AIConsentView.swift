import SwiftUI

/// 第三者 AI(Anthropic の Claude)へ送る前の説明と同意(#246・App Store 5.1.2(i))。
/// 初回起動時と、同意前に生成・対話しようとした時に出す。「あとで」でも閲覧・記録などは使える。
struct AIConsentView: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: Theme.Spacing.lg) {
                    Image(systemName: "sparkles")
                        .font(.system(size: 44))
                        .foregroundStyle(Theme.brandGradient)
                        .frame(maxWidth: .infinity)
                        .padding(.top, Theme.Spacing.md)
                    Text("AI へのデータ送信について")
                        .font(.title2.bold())
                        .frame(maxWidth: .infinity)
                        .multilineTextAlignment(.center)
                    Text("アプリを作る・直すときは、次の内容を Anthropic 社の AI(Claude)に送ります。")
                        .font(.callout)
                    VStack(alignment: .leading, spacing: Theme.Spacing.md) {
                        row("text.bubble", "話した内容・入力した文(端末内で文字にしたもの。音声そのものは送りません)")
                        row("bubble.left.and.bubble.right", "AI との会話の履歴")
                        row("square.grid.2x2", "今のアプリの設計(画面・項目・アプリ名など)")
                    }
                    Text("送り先: Tailor It のサーバー(Google Cloud)を経由して Anthropic, PBC(米国)へ。送った内容は、アプリを作るためだけに使います。")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                    Text("記録したデータ(数値・メモ・写真など)の中身は送りません。同意しなくても、作ったアプリの閲覧・記録はできます。生成と対話だけが使えません。同意は設定からいつでも取り消せます(送信済みの内容は取り消せません)。")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                    VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
                        LegalDocumentLink(document: .privacyPolicy)
                        LegalDocumentLink(document: .terms)
                    }
                    .font(.callout)
                }
                .padding(Theme.Spacing.lg)
            }
            .safeAreaInset(edge: .bottom) {
                VStack(spacing: Theme.Spacing.sm) {
                    Button {
                        rt.grantAIConsent()
                        dismiss()
                    } label: {
                        Text("同意して使う").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .controlSize(.large)
                    .tint(Theme.Colors.brandA)
                    .accessibilityIdentifier("consent.agree")
                    Button("あとで") { dismiss() }
                        .accessibilityIdentifier("consent.later")
                }
                .padding(Theme.Spacing.lg)
                .background(.bar)
            }
        }
        .interactiveDismissDisabled()
        .onDisappear { rt.aiConsent?.markPrompted() }
        .accessibilityIdentifier("consent.sheet")
    }

    private func row(_ icon: String, _ text: LocalizedStringKey) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.md) {
            Image(systemName: icon).foregroundStyle(Theme.Colors.brandA).frame(width: 24)
            Text(text).font(.callout)
        }
    }
}
