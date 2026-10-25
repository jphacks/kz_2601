import AuthenticationServices
import SwiftUI
import MetamonServices

/// 起動時にサインインしていなければ出す画面(G5)。Sign in with Apple 優先、下に匿名フォールバック。
struct SignInView: View {
    @Environment(AuthSession.self) private var session
    @Environment(\.colorScheme) private var colorScheme
    @State private var nonce = ""

    var body: some View {
        ZStack {
            Theme.backdrop(colorScheme)

            VStack(spacing: Theme.Spacing.lg) {
                Spacer()
                if session.farewell {
                    // ログアウト / アカウント削除の後(#102)
                    CraneMascot(pose: .bye, size: 120, sway: 6)
                    Text("またね。いつでも戻ってきてください")
                        .font(.subheadline.weight(.medium))
                        .foregroundStyle(.secondary)
                } else {
                    BrandMark(size: 96)
                        .symbolEffect(.pulse, options: .repeating, isActive: session.isBusy)
                }
                VStack(spacing: Theme.Spacing.sm) {
                    Text("Tailor It")
                        .font(.system(size: 40, weight: .bold, design: .rounded))
                        .brandText()
                    Text("話しかけるだけで、アプリが生まれ変わる")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)
                }
                Spacer()

                VStack(spacing: Theme.Spacing.md) {
                    SignInWithAppleButton(.signIn) { request in
                        let raw = AppleNonce.random()
                        nonce = raw
                        request.requestedScopes = [.fullName, .email]
                        request.nonce = AppleNonce.sha256(raw)
                    } onCompletion: { result in
                        let raw = nonce
                        Task { await session.completeAppleSignIn(result, nonce: raw) }
                    }
                    .signInWithAppleButtonStyle(colorScheme == .dark ? .white : .black)
                    .frame(height: 52)
                    .clipShape(RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
                    .disabled(session.isBusy)

                    Button {
                        Task { await session.signInAnonymously() }
                    } label: {
                        Text("ログインせずに使う(匿名)")
                            .font(.footnote.weight(.medium))
                            .foregroundStyle(.secondary)
                            .padding(.vertical, Theme.Spacing.sm)
                    }
                    .disabled(session.isBusy)

                    if let message = session.errorMessage {
                        Text(message)
                            .font(.footnote)
                            .foregroundStyle(.red)
                            .multilineTextAlignment(.center)
                            .transition(.opacity)
                    }
                }
            }
            .padding(Theme.Spacing.lg + Theme.Spacing.xs)
        }
        .overlay {
            if session.isBusy {
                ProgressView().controlSize(.large)
                    .padding(Theme.Spacing.lg)
                    .background(.regularMaterial, in: RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
            }
        }
        #if DEBUG
        // ヘッドレス E2E 用: `METAMON_E2E_ANON=1` なら匿名サインインを自動で行う(シミュレータはタップ自動化が無い)
        .task {
            if ProcessInfo.processInfo.environment["METAMON_E2E_ANON"] == "1", session.user == nil { await session.signInAnonymously() }
        }
        #endif
        .animation(.default, value: session.errorMessage)
        .animation(.default, value: session.isBusy)
    }
}

#Preview {
    SignInView()
        .environment(AuthSession(service: MockAuthService(signedIn: false)))
}
