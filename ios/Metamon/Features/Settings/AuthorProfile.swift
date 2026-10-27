import SwiftUI

/// 共有アプリへの投稿に出す表示名(#250)。本人が決めた名前だけを使う。
/// Apple の氏名・メールアドレス・端末名は、本人が公開すると決めていないので使わない。未設定なら送らず、backend が「匿名」を付ける。
/// アカウント削除(`wipeLocalData`)で UserDefaults ごと消える。
enum AuthorProfile {
    static let nameKey = "authorProfile.name"
    static let askedKey = "authorProfile.asked"
    /// backend の上限(64 文字)より短く。一覧の 1 行に収まる長さ
    static let maxLength = 20

    /// 投稿に付ける名前。空白だけ・未設定は nil。
    static var name: String? {
        get { normalized(UserDefaults.standard.string(forKey: nameKey) ?? "") }
        set { UserDefaults.standard.set(normalized(newValue ?? "") ?? "", forKey: nameKey) }
    }

    /// 初めての共有投稿で一度だけ名前を聞いたか(設定で名前を決めた時も済みにする)。
    static var asked: Bool {
        get { UserDefaults.standard.bool(forKey: askedKey) }
        set { UserDefaults.standard.set(newValue, forKey: askedKey) }
    }

    static func normalized(_ raw: String) -> String? {
        let s = String(raw.trimmingCharacters(in: .whitespacesAndNewlines).prefix(maxLength))
        return s.isEmpty ? nil : s
    }
}

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
