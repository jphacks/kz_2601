import SwiftUI
import MetamonServices

/// 設定の「プライバシー」(#246): AI へのデータ送信の同意の確認・取り消しと、プライバシーポリシー・利用規約。
/// クラッシュ報告(#296)のオン・オフもここ(AI ではないので同意とは別の切り替え)。
struct PrivacySettingsSection: View {
    @Environment(AppRuntime.self) private var rt
    private let crashReporting = CrashReportingPreference.shared
    @State private var confirmRevoke = false
    @State private var showConsent = false

    var body: some View {
        Section {
            LabeledContent("AI へのデータ送信") {
                Text(rt.hasAIConsent ? "同意済み" : "未同意")
                    .foregroundStyle(rt.hasAIConsent ? .green : .orange)
                    .accessibilityIdentifier("settings.aiConsent.status")
            }
            if rt.hasAIConsent {
                if let date = rt.aiConsent?.grantedAt {
                    LabeledContent("同意した日時", value: date.formatted(date: .abbreviated, time: .shortened))
                        .font(.footnote)
                }
                Button("同意を取り消す", role: .destructive) { confirmRevoke = true }
                    .accessibilityIdentifier("settings.aiConsent.revoke")
            } else {
                Button("説明を読んで同意する") { showConsent = true } // 設定シートの上に重ねる(RootView のシートは出せない)
                    .accessibilityIdentifier("settings.aiConsent.grant")
                    .sheet(isPresented: $showConsent) { AIConsentView() }
            }
            LegalDocumentLink(document: .privacyPolicy)
            LegalDocumentLink(document: .terms)
        } header: {
            Text("プライバシー")
        } footer: {
            Text(L10n.key("アプリを作る・直すときは、話した内容・会話・アプリの設計を Anthropic 社の AI に送ります(記録の中身は送りません)。「AI で書く」ボタンがサーバーの AI を使う場合は、押した時だけ、そのアプリの最近の記録も送ります。同意を取り消すと、AI を使う機能が使えなくなります(閲覧・記録は使えます)。送信済みの内容は取り消せません。", review: "記録帳を用意する・直すときは、話した内容・会話・記録帳の画面の形を Anthropic 社の AI に送ります(記録の中身は送りません)。「AI で書く」ボタンがサーバーの AI を使う場合は、押した時だけ、その記録帳の最近の記録も送ります。同意を取り消すと、AI を使う機能が使えなくなります(閲覧・記録は使えます)。送信済みの内容は取り消せません。"))
        }
        .alert("AI への送信の同意を取り消しますか？", isPresented: $confirmRevoke) {
            Button("取り消す", role: .destructive) { rt.revokeAIConsent() }
            Button("キャンセル", role: .cancel) {}
        } message: {
            Text(L10n.key("アプリの生成と対話が使えなくなります。もう一度同意すると使えます。", review: "記録帳を用意する・直す機能と対話が使えなくなります。もう一度同意すると使えます。"))
        }
        Section {
            Toggle("クラッシュ報告を送る", isOn: Binding(get: { crashReporting.isEnabled }, set: { crashReporting.setEnabled($0) }))
                .accessibilityIdentifier("settings.crashReporting.toggle")
            #if !APP_STORE_REVIEW
            if CrashTestTrigger.isAvailable {
                Button("テスト用にクラッシュさせる", role: .destructive) { CrashTestTrigger.crash() }
                    .accessibilityIdentifier("settings.crashReporting.testCrash")
            }
            #endif
        } footer: {
            Text("アプリが落ちた時に、落ちた場所(プログラムの位置)・端末の機種・OS・アプリの版とユーザー ID を Google の Firebase Crashlytics に送り、不具合を直すのに使います。話した内容・記録・作ったアプリの中身は送りません。")
        }
    }
}

#if !APP_STORE_REVIEW
/// クラッシュ報告の動作確認(#296 完了の条件: わざと落としたクラッシュがシンボル付きで見える)。
/// 開発中(Xcode から入れた版)と TestFlight だけ出す(App Store の版はレシートが sandboxReceipt ではない)。審査用ビルドには入れない。
enum CrashTestTrigger {
    static var isAvailable: Bool {
        #if DEBUG
        return true
        #else
        return Bundle.main.appStoreReceiptURL?.lastPathComponent == "sandboxReceipt"
        #endif
    }

    static func crash() -> Never {
        fatalError("Crashlytics test crash (#296)")
    }
}
#endif
