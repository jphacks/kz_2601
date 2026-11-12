import AuthenticationServices
import SwiftUI
import MetamonServices

// #361: 1 つのアカウントを OS をまたいで使う。つながっているサインインの方法を見せ、Apple 以外のアカウントに Apple を足せる。
// その Apple ID が別のアカウントにあっても、アカウントはまとめない(切り替えもしない)。

/// 既存のアカウントに Apple をつないだ結果。
enum AppleLinkOutcome: Equatable {
    case linked
    case cancelled
    /// その Apple ID は別のアカウントで使われている(`credentialAlreadyInUse`)。何も変えない
    case inUseByOtherAccount
    case failed(String)
}

/// 設定の「サインインの方法」。匿名のユーザーには出さない(匿名 → Apple の引き継ぎはアカウントの欄・#276)。
struct LinkedAccountsSection: View {
    @Environment(AuthSession.self) private var session
    @Environment(\.colorScheme) private var colorScheme
    @State private var nonce = ""
    @State private var outcome: AppleLinkOutcome?

    var body: some View {
        if let user = session.user, !user.isAnonymous {
            Section {
                ForEach(SignInProvider.allCases) { provider in
                    row(provider, linked: user.providers.contains(provider))
                }
                if !user.providers.contains(.apple) {
                    appleButton
                        .listRowInsets(EdgeInsets(top: Theme.Spacing.sm, leading: Theme.Spacing.md, bottom: Theme.Spacing.sm, trailing: Theme.Spacing.md))
                }
                outcomeMessage
            } header: {
                Text("サインインの方法")
            } footer: {
                Text("1 つのアカウントに複数のサインインの方法をつなぐと、iPhone でも Android でも同じアカウントに入れます。")
            }
        }
    }

    private func row(_ provider: SignInProvider, linked: Bool) -> some View {
        HStack(spacing: Theme.Spacing.md) {
            Image(systemName: provider == .apple ? "apple.logo" : "g.circle")
                .frame(width: 24)
            Text(verbatim: provider == .apple ? "Apple" : "Google")
            Spacer()
            if linked {
                Label("つながっています", systemImage: "checkmark.circle.fill")
                    .labelStyle(.titleAndIcon)
                    .foregroundStyle(.green)
            } else {
                Text("つながっていません").foregroundStyle(.secondary)
            }
        }
        .font(.subheadline)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("settings.provider.\(provider == .apple ? "apple" : "google")")
    }

    private var appleButton: some View {
        SignInWithAppleButton(.continue) { request in
            let raw = AppleNonce.random()
            nonce = raw
            request.requestedScopes = [.fullName]
            request.nonce = AppleNonce.sha256(raw)
        } onCompletion: { result in
            let raw = nonce
            Task { outcome = await session.linkAppleToCurrentAccount(result, nonce: raw) }
        }
        .signInWithAppleButtonStyle(colorScheme == .dark ? .white : .black)
        .frame(height: 48)
        .clipShape(RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
        .disabled(session.isBusy)
        .accessibilityIdentifier("settings.linkApple")
    }

    @ViewBuilder private var outcomeMessage: some View {
        switch outcome {
        case .linked:
            Text("Apple をつなぎました。次からは Apple でもこのアカウントに入れます。")
                .font(.footnote).foregroundStyle(.secondary)
        case .inUseByOtherAccount:
            Text("この Apple ID は、すでに別のアカウントで使われています。アカウントはまとめず、いまのアカウントはそのままです。この Apple ID のアカウントを使うときは、ログアウトしてから Apple でサインインしてください。")
                .font(.footnote).foregroundStyle(.red)
                .accessibilityIdentifier("settings.linkApple.inUse")
        case .failed(let message):
            Text(message).font(.footnote).foregroundStyle(.red)
        case .cancelled, nil:
            EmptyView()
        }
    }
}
