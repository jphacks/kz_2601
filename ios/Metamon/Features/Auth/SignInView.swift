import AuthenticationServices
import SwiftUI
import MetamonServices

/// 起動時にサインインしていなければ出す画面(G5)。Sign in with Apple 優先、下に匿名フォールバック。
struct SignInView: View {
    @Environment(AuthSession.self) private var session
    @Environment(\.colorScheme) private var colorScheme
    @State private var nonce = ""

    var body: some View {
        VStack(spacing: 20) {
            Spacer()
            Image(systemName: "waveform.circle.fill")
                .font(.system(size: 84))
                .foregroundStyle(.tint)
                .symbolEffect(.pulse, options: .repeating, isActive: session.isBusy)
            Text("メタモン")
                .font(.largeTitle.bold())
            Text("話しかけるだけで、アプリが生まれ変わる")
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
            Spacer()

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
            .clipShape(RoundedRectangle(cornerRadius: 12))
            .disabled(session.isBusy)

            Button {
                Task { await session.signInAnonymously() }
            } label: {
                Text("ログインせずに使う(匿名)")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .padding(.vertical, 8)
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
        .padding(28)
        .overlay {
            if session.isBusy {
                ProgressView().controlSize(.large)
                    .padding(24)
                    .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 16))
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
