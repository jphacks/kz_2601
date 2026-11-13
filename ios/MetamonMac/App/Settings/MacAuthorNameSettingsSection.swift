import SwiftUI

#if !APP_STORE_REVIEW // 審査用ビルドは共有を外しているので、この欄も出さない(#247)
/// 設定の「投稿に表示する名前」(#3077)。iOS と同じ保存先・同じ正規化(`AuthorProfile`)。Apple の氏名・端末名は使わない。
struct MacAuthorNameSettingsSection: View {
    @AppStorage(AuthorProfile.nameKey) private var stored = ""
    @State private var draft = ""

    var body: some View {
        Section {
            TextField("匿名", text: $draft)
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
