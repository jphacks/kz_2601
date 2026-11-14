import SwiftUI
import AuthenticationServices
import MetamonServices

/// 設定の「アカウント」の「サインインの方法」(#361・iOS の `LinkedAccountsSection` の Mac 版)。
/// つながっている方法を見せ、Apple 以外(Android の Google など)で作ったアカウントに Apple を足せる。
/// その Apple ID が別のアカウントにあっても、アカウントはまとめない(切り替えもしない)。匿名の人には出さない
struct MacLinkedAccountsSection: View {
    private let account = MacAccount.shared
    @State private var outcome: AppleLinkOutcome?

    var body: some View {
        if LinkedAccountsRules.showsSection(for: account.session.user), let user = account.session.user {
            Section {
                ForEach(LinkedAccountsRules.rows(for: user)) { provider in
                    row(provider)
                }
                if LinkedAccountsRules.offersApple(to: user) {
                    MacLinkAppleButton { outcome = $0 }
                }
                outcomeMessage
            } header: {
                Text("サインインの方法")
            } footer: {
                Text("1 つのアカウントに複数のサインインの方法をつなぐと、iPhone でも Android でも同じアカウントに入れます。")
            }
            .task { await account.session.refreshUser() } // 別の端末でつないだ方法も出す(providerData は手元のキャッシュ)
        }
    }

    private func row(_ provider: SignInProvider) -> some View {
        LabeledContent {
            Label("つながっています", systemImage: "checkmark.circle.fill")
                .labelStyle(.titleAndIcon)
                .foregroundStyle(.green)
        } label: {
            Label {
                Text(verbatim: provider == .apple ? "Apple" : "Google")
            } icon: {
                Image(systemName: provider == .apple ? "apple.logo" : "g.circle")
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("settings.provider.\(provider == .apple ? "apple" : "google")")
    }

    @ViewBuilder private var outcomeMessage: some View {
        switch outcome {
        case .linked:
            Text("Apple をつなぎました。次からは Apple でもこのアカウントに入れます。")
                .font(.callout).foregroundStyle(.secondary)
        case .inUseByOtherAccount:
            Text("この Apple ID は、すでに別のアカウントで使われています。アカウントはまとめず、いまのアカウントはそのままです。この Apple ID のアカウントを使うときは、ログアウトしてから Apple でサインインしてください。")
                .font(.callout).foregroundStyle(.red)
                .accessibilityIdentifier("settings.linkApple.inUse")
        case .emailInUseByOtherAccount:
            Text("この Apple ID のメールアドレスは、すでに別のアカウントで使われています。アカウントはまとめず、いまのアカウントはそのままです。そのアカウントを使うときは、ログアウトしてから、そのアカウントを作った方法でサインインしてください。")
                .font(.callout).foregroundStyle(.red)
                .accessibilityIdentifier("settings.linkApple.emailInUse")
        case .failed(let message):
            Text(message).font(.callout).foregroundStyle(.red)
        case .cancelled, nil:
            EmptyView()
        }
    }
}

/// 既にあるアカウントに Apple をつなぐボタン(「Apple で続ける」)。結果は呼び出し側が文にする
private struct MacLinkAppleButton: View {
    var onOutcome: (AppleLinkOutcome) -> Void
    @Environment(\.colorScheme) private var colorScheme
    @State private var nonce = ""

    var body: some View {
        SignInWithAppleButton(.continue) { request in
            let raw = AppleNonce.random()
            nonce = raw
            request.requestedScopes = [.fullName]
            request.nonce = AppleNonce.sha256(raw)
        } onCompletion: { result in
            let raw = nonce
            Task { onOutcome(await MacAccount.shared.linkApple(result, nonce: raw)) }
        }
        .signInWithAppleButtonStyle(colorScheme == .dark ? .white : .black)
        .frame(width: 200, height: 28)
        .disabled(MacAccount.shared.session.isBusy)
        .accessibilityIdentifier("settings.linkApple")
    }
}
