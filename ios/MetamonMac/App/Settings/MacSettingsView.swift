import AppKit
import SwiftUI
import MetamonRenderer
import MetamonServices

/// ⌘, で開く設定ウィンドウ(#973)。iOS の設定シート(SettingsSheet)の節を、Mac の作法(タブ + Form の grouped)で並べる。
/// 値は iOS と同じ保存先(UserDefaults のキー)を使う。Mac に無い仕組み(サインイン・課金・通知の登録・音声)に頼る行は
/// 「この端末では使えません」と表示だけにする(Renderer の `UnavailableComponent` と同じ出し方)。
struct MacSettingsView: View {
    var body: some View {
        TabView {
            MacGeneralSettings()
                .tabItem { Label("一般", systemImage: "gearshape") }
            MacAccountSettings()
                .tabItem { Label("アカウント", systemImage: "person.crop.circle") }
            MacNotificationSettings()
                .tabItem { Label("通知", systemImage: "bell") }
            #if !APP_STORE_REVIEW // 審査用の版は OAuth の provider を外す(決定 6)
            MacConnectedServicesSettings()
                .tabItem { Label("つないだサービス", systemImage: "link") }
            #endif
            MacPrivacySettings()
                .tabItem { Label("プライバシー", systemImage: "hand.raised") }
            MacDetailSettings()
                .tabItem { Label("詳細", systemImage: "info.circle") }
        }
        .frame(width: 560)
        .frame(minHeight: 360)
    }
}

/// 表示言語(#44)を画面に流す。iOS のルートと同じく `\.locale` を `LanguagePreference` から決め、切り替えで即時に描き直す。
struct MacLanguageRoot<Content: View>: View {
    @ViewBuilder var content: Content

    var body: some View {
        content.environment(\.locale, LanguagePreference.shared.locale)
    }
}

/// Mac で使えない行。名前と「この端末では使えません」。
struct MacUnavailableSettingRow: View {
    let title: LocalizedStringKey

    var body: some View {
        LabeledContent(title) {
            Label("この端末では使えません", systemImage: "nosign")
                .foregroundStyle(.secondary)
        }
        .accessibilityElement(children: .combine)
    }
}

// MARK: - 一般(言語・対話・読み上げ)

private struct MacGeneralSettings: View {
    // 対話(#75 / #160)。iOS と同じ保存先(`ClarifySettings`)。会話のシート(#967)が開く時に読む
    @State private var clarifyEnabled = ClarifySettings.enabled
    @State private var clarifyAutoBuild = ClarifySettings.autoBuild
    /// 作ったアプリの文字の大きさ(#937)。表示メニューの ⌘+ / ⌘- / ⌘0 と同じ保存先
    @AppStorage(MacTextZoom.storageKey) private var zoomStep = MacTextZoom.defaultStep

    var body: some View {
        @Bindable var language = LanguagePreference.shared
        @Bindable var reader = MacSpeechReader.shared
        Form {
            // 表示言語(#44)。iOS と同じ `LanguagePreference`(キー metamon.appLanguage)。
            Section {
                Picker("言語", selection: $language.setting) {
                    Text("自動").tag(AppLanguage.system)
                    Text(verbatim: "日本語").tag(AppLanguage.ja)
                    Text(verbatim: "English").tag(AppLanguage.en)
                }
                .accessibilityIdentifier("settings.language")
            } footer: {
                Text(L10n.key("新しく作るアプリと質問もこの言語になります。作成済みのアプリの文言は変わりません。", review: "新しく用意する記録帳と質問もこの言語になります。用意済みの記録帳の文言は変わりません。"))
            }
            Section {
                Toggle("チャットで相談してから作る", isOn: $clarifyEnabled)
                    .accessibilityIdentifier("settings.clarify")
                    .onChange(of: clarifyEnabled) { _, on in ClarifySettings.enabled = on }
                Toggle("答えがそろったら自動で作る", isOn: $clarifyAutoBuild)
                    .disabled(!clarifyEnabled)
                    .accessibilityIdentifier("settings.clarifyAutoBuild")
                    .onChange(of: clarifyAutoBuild) { _, on in ClarifySettings.autoBuild = on }
            } header: {
                Text("対話")
            } footer: {
                VStack(alignment: .leading, spacing: 4) {
                    Text(L10n.key("ON にすると、新しく作る時も作ったアプリを変える時も、まずチャットで AI に相談できます。質問には答え、案を出すので「これでいく」で反映します。OFF にすると、話した内容をすぐ反映します。",
                                  review: "ON にすると、新しく用意する時も記録帳を直す時も、まずチャットで AI に相談できます。質問には答え、案を出すので「これでいく」で反映します。OFF にすると、話した内容をすぐ反映します。"))
                    Text(L10n.key("新しいアプリを声だけで相談している時は、内容がそろうと少し待ってから自動で作り始めます。作ったアプリの変更は、意図しない変更を防ぐため必ず「これでいく」を押してから反映します。",
                                  review: "新しい記録帳を声だけで相談している時は、内容がそろうと少し待ってから自動で用意し始めます。用意した記録帳の変更は、意図しない変更を防ぐため必ず「これでいく」を押してから反映します。"))
                }
            }
            // 文字の大きさ(#937)。Mac に Dynamic Type は無いので、作ったアプリの文字の段をここと表示メニューで選ぶ
            Section {
                Picker("文字の大きさ", selection: Binding(get: { MacTextZoom.clamped(zoomStep) },
                                                    set: { zoomStep = MacTextZoom.clamped($0) })) {
                    ForEach(MacTextZoom.steps.indices, id: \.self) { step in
                        zoomLabel(step).tag(step)
                    }
                }
                .accessibilityIdentifier("settings.textZoom")
            } header: {
                Text("表示")
            } footer: {
                Text("作ったアプリの文字の大きさです。表示メニューの「拡大」(⌘+)・「縮小」(⌘-)・「実際のサイズ」(⌘0)でも変えられます。")
            }
            // 声で頼むのマイク(#1221)
            MacInputDeviceSection()
            // メニューバーの常駐・ログイン時に開く(#1848)
            MacMenuBarSettingsSection()
            // 読み上げ(#1210・#1216)。iOS と同じ保存先(metamon.speech.enabled)。Mac が読むのは完成の要約だけ
            Section {
                Toggle("アプリができた時の説明を読み上げる", isOn: $reader.enabled)
                    .help("アプリができた時の説明を Mac の声で読み上げます。VoiceOver を使っている時は重ねて読みません")
                    .accessibilityIdentifier("settings.speech")
            } header: {
                Text("読み上げ")
            } footer: {
                Text("ON にすると、アプリができた時の説明を声で読み上げます。新しく頼むとすぐ止まります。")
            }
        }
        .formStyle(.grouped)
    }

    private func zoomLabel(_ step: Int) -> Text {
        let percent = Text(verbatim: "\(MacTextZoom.percent(at: step))%")
        guard MacTextZoom.isActualSize(step) else { return percent }
        return Text("標準") + Text(verbatim: " (") + percent + Text(verbatim: ")")
    }
}

// MARK: - アカウント(サインイン・Pro・今日の残り・使っている量)

private struct MacAccountSettings: View {
    var body: some View {
        Form {
            // サインイン(#930)。Firebase につながっていない版(plist 無し)は今まで通り「使えない」
            Section {
                if MacAccount.shared.isLive {
                    MacAccountRow()
                } else {
                    MacUnavailableSettingRow(title: "アカウント")
                }
            }
            #if !APP_STORE_REVIEW // 審査用ビルドは App 内課金を持たない(#450 G1)
            Section {
                MacProSettingsRow()
            }
            #endif
            // 今日の残り(#630・#1219)。AI の応答のヘッダーと「使っている量」から入る(iOS と同じ表示・審査用の版も出す)
            RemainingTodaySettingsSection()
            // 使っている量(#317・#1219)。サーバーにつながっている版だけ
            if MacAccount.shared.isLive {
                MacUsageSettingsSection()
            } else {
                Section { MacUnavailableSettingRow(title: "使っている量") }
            }
        }
        .formStyle(.grouped)
    }
}

/// アカウントの状態と「Apple でサインイン」(#930)。匿名なら Apple に引き継ぐ(uid はそのまま)。
/// ログアウトとアカウント削除(#972)は iOS の設定と同じ確認・同じ文言で、この Mac のデータを消してから行う
private struct MacAccountRow: View {
    private let account = MacAccount.shared
    @State private var signOutPrompt: SignOutFlow.Prompt?
    @State private var signOutUnsent: Int?
    @State private var signOutError: String?
    @State private var confirmDelete = false
    @State private var deleteError: String?

    var body: some View {
        LabeledContent("アカウント") {
            if let user = account.session.user {
                VStack(alignment: .trailing, spacing: 6) {
                    Text(SupportInfo.accountStatus(isAnonymous: user.isAnonymous))
                    if user.isAnonymous { MacAppleSignInButton() }
                }
            } else {
                MacAppleSignInButton()
            }
        }
        if let message = account.errorMessage {
            Text(message)
                .font(.callout)
                .foregroundStyle(.secondary)
        }
        if account.session.user != nil {
            LabeledContent {
                HStack {
                    if let busy = account.busy {
                        ProgressView(LocalizedStringKey(busy == .deleting ? "削除しています…" : "ログアウトしています…"))
                            .controlSize(.small)
                    }
                    Button("ログアウト", role: .destructive) {
                        Task { signOutPrompt = await account.signOutPrompt() }
                    }
                    .help("この Mac のデータを消してからログアウトします")
                    .accessibilityIdentifier("settings.signOut")
                    Button("アカウントを削除", role: .destructive) { confirmDelete = true }
                        .help("アカウントを削除し、この Mac のデータも消します。元に戻せません")
                        .accessibilityIdentifier("settings.deleteAccount")
                }
                .disabled(account.busy != nil)
            } label: {
                Text(L10n.key("作ったアプリが使えなくなり、共有リンクと「探す」への掲載も取り下げます。この端末のデータも消えます。ほかの人と共有しているアプリへの投稿は残ります。", review: "用意した記録帳が使えなくなります。この端末のデータも消えます。"))
                    .font(.callout)
                    .foregroundStyle(.secondary)
            }
            .alert("ログアウトしますか？", isPresented: Binding(get: { signOutPrompt != nil }, set: { if !$0 { signOutPrompt = nil } }),
                   presenting: signOutPrompt) { prompt in
                switch prompt {
                case .plain:
                    Button("ログアウト", role: .destructive) { signOut(.discardAndWipe) }
                case .unsent(_, let canSend):
                    if canSend {
                        Button("送ってからログアウト") { signOut(.sendThenWipe) }
                            .keyboardShortcut(.defaultAction)
                    }
                    Button("送らずにログアウト", role: .destructive) { signOut(.discardAndWipe) }
                }
                Button("キャンセル", role: .cancel) {}
            } message: { prompt in
                Text(SignOutCopy.message(isAnonymous: account.session.user?.isAnonymous == true, prompt: prompt))
            }
            .alert("送れなかった記録があります", isPresented: Binding(get: { signOutUnsent != nil }, set: { if !$0 { signOutUnsent = nil } }),
                   presenting: signOutUnsent) { _ in
                Button("送らずにログアウト", role: .destructive) { signOut(.discardAndWipe) }
                Button("キャンセル", role: .cancel) {}
            } message: { n in
                Text(SignOutCopy.unsentAfterFlush(n))
            }
            .alert("ログアウトできませんでした", isPresented: Binding(get: { signOutError != nil }, set: { if !$0 { signOutError = nil } })) {
                Button("OK", role: .cancel) {}
            } message: {
                Text(signOutError ?? "")
            }
            .alert("アカウントを削除しますか？", isPresented: $confirmDelete) {
                Button("削除する", role: .destructive) { deleteAccount() }
                Button("キャンセル", role: .cancel) {}
            } message: {
                Text(L10n.key("アカウントとすべてのアプリが使えなくなります。元に戻せません。", review: "アカウントとすべての記録帳が使えなくなります。元に戻せません。"))
            }
            .alert("アカウントを削除できませんでした", isPresented: Binding(get: { deleteError != nil }, set: { if !$0 { deleteError = nil } })) {
                Button("OK", role: .cancel) {}
            } message: {
                Text(deleteError ?? "")
            }
        }
    }

    private func signOut(_ choice: SignOutFlow.Choice) {
        Task {
            do {
                if case .unsent(let n) = try await account.signOutAndWipe(choice: choice) { signOutUnsent = n }
            } catch {
                signOutError = error.localizedDescription
            }
        }
    }

    private func deleteAccount() {
        Task {
            do {
                _ = try await account.deleteAccount()
            } catch {
                deleteError = error.localizedDescription
            }
        }
    }
}

#if !APP_STORE_REVIEW
/// Tailor It Pro の状態と課金画面の入口(iOS の「Pro にする」と同じ商品・同じ購入)
private struct MacProSettingsRow: View {
    var body: some View {
        LabeledContent("Tailor It Pro") {
            if MacPro.shared.isPro {
                HStack {
                    Label("ご利用中", systemImage: "checkmark.seal.fill")
                        .foregroundStyle(Theme.Colors.brandText)
                    Button("サブスクリプションを管理") { MacPro.openManageSubscriptions() }
                        .help("App Store のアカウントのページを開きます")
                }
            } else {
                MacUpgradeProButton(reason: .settings)
            }
        }
        .task { MacPro.shared.start() }
    }
}
#endif

// MARK: - 通知

private struct MacNotificationSettings: View {
    var body: some View {
        Form {
            // 予約した通知の一覧はアプリごとなので、中央のアプリのツールバーのベル・「アプリ」メニューに置く(#1325)。
            // 他の人の行動で届く通知(Push・Firebase)は Mac にまだ無い
            Section {
                LabeledContent("次の通知") {
                    Text("アプリを開いて、ツールバーのベルから確かめられます")
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.trailing)
                }
                .accessibilityElement(children: .combine)
                #if !APP_STORE_REVIEW
                MacUnavailableSettingRow(title: "他の人の行動で届く通知")
                #endif
            }
        }
        .formStyle(.grouped)
    }
}

// MARK: - プライバシー(AI への同意・クラッシュ報告・ポリシー・規約)

/// iOS の `PrivacySettingsSection`(#246・#296・#446)と同じ保存先。未同意の時の「説明を読んで同意する」は
/// 同意の説明(`MacAIConsentSheet`・#971)を設定ウィンドウの上に開く。規約・ポリシーは同梱の文書をアプリ内で読む。
private struct MacPrivacySettings: View {
    private let consent = AIConsentStore.shared
    private let crashReporting = CrashReportingPreference.shared
    #if !APP_STORE_REVIEW
    private let aiButtonMetrics = AIButtonMetricsRecorder.shared
    #endif
    @State private var confirmRevoke = false
    @State private var showConsent = false
    @State private var legalDocument: LegalDocument?

    var body: some View {
        Form {
            Section {
                LabeledContent("AI へのデータ送信") {
                    Text(consent.isGranted ? "同意済み" : "未同意")
                        .foregroundStyle(consent.isGranted ? .green : .orange)
                        .accessibilityIdentifier("settings.aiConsent.status")
                }
                if consent.isGranted {
                    if let date = consent.grantedAt {
                        LabeledContent("同意した日時", value: date.formatted(date: .abbreviated, time: .shortened))
                    }
                    Button("同意を取り消す", role: .destructive) { confirmRevoke = true }
                        .accessibilityIdentifier("settings.aiConsent.revoke")
                } else {
                    Button("説明を読んで同意する") { showConsent = true }
                        .accessibilityIdentifier("settings.aiConsent.grant")
                }
                ForEach(LegalDocument.allCases) { doc in
                    Button { legalDocument = doc } label: { Label(doc.title, systemImage: doc.systemImage) }
                        .accessibilityIdentifier("legal.\(doc.rawValue)")
                }
            } header: {
                Text("プライバシー")
            } footer: {
                Text(L10n.key("アプリを作る・直すときは、話した内容・会話・アプリの設計を Anthropic 社の AI に送ります(記録の中身は送りません)。「AI で書く」ボタンは、押した時だけ送ります(短い答えは端末の中の AI が使えない時だけ最大 4KB、長い答えはいつも最大 16KB)。「AI と話す」部品は直近の会話(最大 20 件・16KB)と、決めてあれば参考の自分の記録(最大 20 件・8KB)を、写真のボタンは縮めた写真 1 枚を送ります。同意を取り消すと、AI を使う機能が使えなくなります(閲覧・記録は使えます)。送信済みの内容は取り消せません。", review: "記録帳を用意する・直すときは、話した内容・会話・記録帳の画面の形を Anthropic 社の AI に送ります(記録の中身は送りません)。「AI で書く」ボタンは端末の中の AI だけで動き、送りません。同意を取り消すと、AI を使う機能が使えなくなります(閲覧・記録は使えます)。送信済みの内容は取り消せません。"))
            }
            Section {
                Toggle("クラッシュ報告を送る", isOn: Binding(get: { crashReporting.isEnabled }, set: { crashReporting.setEnabled($0) }))
                    .accessibilityIdentifier("settings.crashReporting.toggle")
            } footer: {
                Text(L10n.key("アプリが落ちた時に、落ちた場所(プログラムの位置)・端末の機種・OS・アプリの版とユーザー ID を Google の Firebase Crashlytics に送り、不具合を直すのに使います。話した内容・記録・作ったアプリの中身は送りません。",
                              review: "アプリが落ちた時に、落ちた場所(プログラムの位置)・端末の機種・OS・アプリの版とユーザー ID を Google の Firebase Crashlytics に送り、不具合を直すのに使います。話した内容・記録・記録帳の中身は送りません。"))
            }
            #if !APP_STORE_REVIEW
            Section {
                Toggle("AI ボタンの利用状況を送る", isOn: Binding(get: { aiButtonMetrics.isEnabled }, set: { aiButtonMetrics.setEnabled($0) }))
                    .accessibilityIdentifier("settings.aiButtonMetrics.toggle")
            } footer: {
                Text("端末内の AI が使えるか(非対応の端末・オフ・準備中)と、AI ボタンが答えを出せなかった回数を、ボタンの種類(要約・分類など)ごとに 1 日 1 回まとめて送ります。ユーザー ID・入力した文・記録は送りません。")
            }
            #endif
        }
        .formStyle(.grouped)
        .sheet(isPresented: $showConsent) { MacAIConsentSheet() }
        .sheet(item: $legalDocument) { MacLegalDocumentSheet(document: $0) }
        .alert("AI への送信の同意を取り消しますか？", isPresented: $confirmRevoke) {
            Button("取り消す", role: .destructive) { consent.revoke() }
            Button("キャンセル", role: .cancel) {}
        } message: {
            Text(L10n.key("アプリを作る・直す機能と対話が使えなくなります。もう一度同意すると使えます。", review: "記録帳を用意する・直す機能と対話が使えなくなります。もう一度同意すると使えます。"))
        }
    }
}

// MARK: - 詳細(お問い合わせ・フィードバック・ライセンス・サポート用の詳細・バージョン)

private struct MacDetailSettings: View {
    @Environment(\.openWindow) private var openWindow
    @State private var showLicenses = false
    @State private var showTutorial = false // #971
    #if !APP_STORE_REVIEW
    @State private var showBetaWelcome = false // #971(β の版だけ)
    private let betaWelcome = BetaWelcomeStore.shared
    #endif
    @State private var showSupportDetails = false
    @State private var copiedSupportInfo = false

    var body: some View {
        let info = supportInfo
        Form {
            Section {
                Button {
                    showTutorial = true
                } label: {
                    Label("使い方を見る", systemImage: "questionmark.circle")
                }
                .accessibilityIdentifier("settings.tutorial")
                #if !APP_STORE_REVIEW
                if betaWelcome.isBeta {
                    Button {
                        showBetaWelcome = true
                    } label: {
                        Label("β 版のご案内", systemImage: "testtube.2")
                    }
                    .accessibilityIdentifier("settings.betaWelcome")
                }
                #endif
            }
            Section {
                Link(destination: LegalDocument.contactURL) {
                    Label("お問い合わせ", systemImage: "envelope")
                }
                .accessibilityIdentifier("settings.contact")
                #if !APP_STORE_REVIEW // 審査用ビルドはフィードバックを外す(#319)
                Button {
                    MacFeedbackCenter.shared.open(openWindow) // #983
                } label: {
                    Label("フィードバックを送る", systemImage: "bubble.left.and.text.bubble.right")
                }
                .accessibilityIdentifier("settings.feedback")
                #endif
                Button {
                    showLicenses = true
                } label: {
                    Label("ライセンス", systemImage: "doc.plaintext")
                }
                .accessibilityIdentifier("settings.licenses")
            }
            Section {
                DisclosureGroup(isExpanded: $showSupportDetails) {
                    ForEach(info.rows, id: \.label) { row in
                        LabeledContent(row.label) {
                            Text(verbatim: row.value)
                                .font(.body.monospaced())
                                .lineLimit(1)
                                .truncationMode(.middle)
                                .textSelection(.enabled)
                        }
                    }
                    Button {
                        PlatformPasteboard.setString(info.clipboardText)
                        copiedSupportInfo = true
                    } label: {
                        Label(copiedSupportInfo ? L10n.tr("コピーしました") : L10n.tr("詳細をコピー"),
                              systemImage: copiedSupportInfo ? "checkmark" : "doc.on.doc")
                    }
                    .accessibilityIdentifier("settings.supportDetails.copy")
                } label: {
                    Text("詳細(サポート用)")
                }
                .accessibilityIdentifier("settings.supportDetails")
            } footer: {
                if showSupportDetails { Text("問い合わせの時は、この内容を伝えてください。") }
            }
        }
        .formStyle(.grouped)
        .sheet(isPresented: $showLicenses) { MacLicensesView() }
        .sheet(isPresented: $showTutorial) {
            MacTutorialView {
                FirstRunTutorialStore.shared.markTutorialSeen()
                showTutorial = false
            }
        }
        #if !APP_STORE_REVIEW
        .sheet(isPresented: $showBetaWelcome) {
            MacBetaWelcomeView {
                betaWelcome.markSeen()
                showBetaWelcome = false
            }
        }
        #endif
    }

    /// Mac はサインイン・音声モデル・接続先・開いているアプリの版を持たないので、バージョンだけ(#279 と同じ並び)。
    private var supportInfo: SupportInfo {
        let bundle = Bundle.main
        return SupportInfo(
            uid: nil,
            voiceModel: nil,
            aiEndpoint: nil,
            appConfigEndpoint: nil,
            appVersion: bundle.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "-",
            build: bundle.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "-",
            appRevision: nil)
    }
}
