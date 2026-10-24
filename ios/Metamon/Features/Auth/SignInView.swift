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
                BrandMark(size: 96)
                    .symbolEffect(.pulse, options: .repeating, isActive: session.isBusy)
                VStack(spacing: Theme.Spacing.sm) {
                    Text("メタモン")
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
        .animation(.default, value: session.errorMessage)
        .animation(.default, value: session.isBusy)
    }
}

#Preview {
    SignInView()
        .environment(AuthSession(service: MockAuthService(signedIn: false)))
}
