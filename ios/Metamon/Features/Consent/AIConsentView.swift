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
                    if rt.aiConsent?.needsReconsent == true { // 版が上がった(ポリシー 6 章)
                        Text("AI に送る内容の説明を改めました。もう一度ご確認ください。")
                            .font(.callout.bold())
                            .accessibilityIdentifier("consent.reconsent")
                    }
                    Text(L10n.key("アプリを作る・直すときは、次の内容を Anthropic 社の AI(Claude)に送ります。", review: "記録帳を用意する・直すときは、次の内容を Anthropic 社の AI(Claude)に送ります。"))
                        .font(.callout)
                    VStack(alignment: .leading, spacing: Theme.Spacing.md) {
                        row("text.bubble", "話した内容・入力した文(端末内で文字にしたもの。音声そのものは送りません)")
                        row("bubble.left.and.bubble.right", "AI との会話の履歴")
                        row("square.grid.2x2", L10n.key("今のアプリの設計(画面・項目・アプリ名など)", review: "今の記録帳の画面の形(画面・項目・名前など)"))
                    }
                    Text(L10n.key("送り先: Tailor It のサーバー(Google Cloud)を経由して Anthropic, PBC(米国)へ。送った内容は、アプリを作る・ボタンの文を作る・会話の返事を作るためだけに使います。", review: "送り先: Tailor It のサーバー(Google Cloud)を経由して Anthropic, PBC(米国)へ。送った内容は、記録帳の画面を用意するためだけに使います。"))
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                    Text(L10n.key("アプリを作る・直すときに、記録したデータ(数値・メモ・写真など)の中身は送りません。", review: "記録帳を用意する・直すときに、記録したデータ(数値・メモ・写真など)の中身は送りません。"))
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                    // 「AI で書く」の範囲(ポリシー 1.3)。審査用の版はオンラインの AI を持たない(#452 G4)ので送らない
                    Text(L10n.key("「AI で書く」ボタン(文を作る・要約・分類・書き直し・抜き出し・提案)は、押した時だけ送ります。短い答えは、端末の中の AI が使える時は送らず、使えない時だけ最近の記録と入力中の文字(最大 4KB)を送ります。長い答えのボタンは、いつも最大 16KB を送ります。共有のアプリで材料にするのは、自分が入れた記録だけです。", review: "「AI で書く」ボタン(文を作る・要約・分類・書き直し・抜き出し・提案)は、端末の中の AI だけで動き、何も送りません。"))
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                    #if !APP_STORE_REVIEW
                    // aiChat・写真のボタン(ポリシー 1.3)。審査用の版はオンラインの AI を持たない
                    Text("「AI と話す」部品は、送るたびに、そのアプリに保存した自分の会話の直近の発言(最大 20 件・16KB)を送ります。参考にする記録帳が決めてあるアプリでは、その記録帳の自分の記録の新しいもの(最大 20 件・8KB)も送ります。写真は「写真を材料にするボタン」を押した時だけ、縮めた 1 枚(JPEG・長い辺 1024 ピクセル以下)を送ります。音声・動画は送りません。")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                    #endif
                    Text(L10n.key("同意しなくても、作ったアプリの閲覧・記録はできます。AI を使う機能だけが使えません。同意は設定からいつでも取り消せます(送信済みの内容は取り消せません)。", review: "同意しなくても、記録帳の閲覧・記録はできます。AI を使う機能だけが使えません。同意は設定からいつでも取り消せます(送信済みの内容は取り消せません)。"))
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
                    Button("あとで") {
                        rt.aiConsent?.markPrompted() // 閉じる前に記録(閉じた直後のチュートリアル判定・#274)
                        dismiss()
                    }
                        .accessibilityIdentifier("consent.later")
                }
                .padding(Theme.Spacing.lg)
                .background(.bar)
            }
        }
        .interactiveDismissDisabled()
        .onDisappear { rt.aiConsent?.markPrompted() } // 閉じたあとのチュートリアル(#274)は TutorialHost
        .accessibilityIdentifier("consent.sheet")
    }

    private func row(_ icon: String, _ text: LocalizedStringKey) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.md) {
            Image(systemName: icon).foregroundStyle(Theme.Colors.brandA).frame(width: 24)
            Text(text).font(.callout)
        }
    }
}
