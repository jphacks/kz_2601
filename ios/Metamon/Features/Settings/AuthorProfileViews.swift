import SwiftUI

#if !APP_STORE_REVIEW // 審査用ビルドは共有を外しているので、この欄も出さない(#247)
/// 設定シートの「投稿に表示する名前」。
struct AuthorNameSettingsSection: View {
    @AppStorage(AuthorProfile.nameKey) private var stored = ""
    @State private var draft = ""

    var body: some View {
        Section {
            TextField("匿名", text: $draft)
                .textInputAutocapitalization(.never)
                .submitLabel(.done)
                .onAppear { draft = stored }
                .onChange(of: draft) { _, new in
                    guard new != stored else { return } // onAppear で読み込んだだけ
                    if new.count > AuthorProfile.maxLength { draft = String(new.prefix(AuthorProfile.maxLength)) }
                    AuthorProfile.name = draft
                    AuthorProfile.asked = true
                }
                .accessibilityIdentifier("settings.authorName")
        } header: {
            Text("投稿に表示する名前")
        } footer: {
            Text("みんなで使うアプリに投稿すると、この名前がほかの人にも表示されます。空なら「匿名」です。Apple ID の名前やメールアドレスは表示しません。変えても、これまでの投稿の名前は変わりません。")
        }
    }
}

#endif

/// 共有アプリへの初めての投稿で表示名を聞くアラート(#250)。`AppRuntime.authorNamePrompt` が立っている間だけ出る。
struct AuthorNamePromptModifier: ViewModifier {
    let active: Bool
    @Environment(AppRuntime.self) private var rt
    @State private var draft = ""

    func body(content: Content) -> some View {
        content.alert(
            Text("投稿に表示する名前"),
            isPresented: Binding(get: { active && rt.authorNamePrompt != nil }, set: { if !$0 { rt.authorNamePrompt = nil } })
        ) {
            TextField("ニックネーム", text: $draft)
            Button("この名前で投稿") { finish(name: draft) }
            Button("匿名で投稿", role: .cancel) { finish(name: nil) }
        } message: {
            Text("ほかの人の一覧にこの名前が表示されます。Apple ID の名前やメールアドレスは表示しません。あとから設定で変えられます。")
        }
    }

    private func finish(name: String?) {
        guard let prompt = rt.authorNamePrompt else { return }
        if let name { AuthorProfile.name = name }
        AuthorProfile.asked = true
        rt.authorNamePrompt = nil
        draft = ""
        Task { await prompt.resume() }
    }
}
