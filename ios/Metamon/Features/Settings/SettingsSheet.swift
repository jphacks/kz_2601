import SwiftUI
import MetamonServices
import MetamonRenderer

/// 歯車から開く設定シート。アカウント・音声モデル・アプリのリセット。uid・接続先などは「詳細(サポート用)」(#279)。
struct SettingsSheet: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(AuthSession.self) private var session
    @Environment(\.services) private var services
    @Environment(\.dismiss) private var dismiss
    @State private var confirmReset = false
    @State private var confirmDelete = false
    @State private var isDeleting = false
    @State private var deleteError: String?
    /// ログアウトの確認(端末のデータを消す・未送信の扱い)
    @State private var signOutPrompt: SignOutFlow.Prompt?
    /// 送ってからログアウトしようとして送り切れなかった件数
    @State private var signOutUnsent: Int?
    @State private var isSigningOut = false
    @State private var signOutError: String?
    @State private var showSupportDetails = false
    @State private var copiedSupportInfo = false
    #if !APP_STORE_REVIEW
    @State private var feedback: FeedbackContext? // #319
    #endif

    var body: some View {
        NavigationStack {
            List {
                if Self.e2eDeleteAccount { deleteAccountSection } // E2E: 削除ボタンを画面内に出す(シミュレータはスクロール操作できない)
                accountSection
                LinkedAccountsSection() // #361 つながっているサインインの方法・Apple をつなぐ
                #if !APP_STORE_REVIEW // 審査用ビルドは共有を外しているので、投稿に出す名前も出さない(#247)
                AuthorNameSettingsSection() // 共有アプリの投稿に出す名前(#250)
                #endif
                #if !APP_STORE_REVIEW // 審査用ビルドはシェアを外す(#245)
                BlockedUsersSettingsSection() // ブロック中のユーザー(#321)
                #endif
                #if !APP_STORE_REVIEW // 審査用ビルドは App 内課金を持たない(#450 G1)
                ProSettingsSection() // 課金(#77)
                CoinSettingsSection() // #630 コイン(サーバーが有効にした時だけ)
                #endif
                #if !APP_STORE_REVIEW // 審査用の版は OAuth の provider を外す(決定 6)
                if ConnectedServicesPolicy.isAvailable { ConnectedServicesSection() } // #1493 つないだサービス
                #endif
                RemainingTodaySettingsSection() // #630 今日の残り(審査用の版も出す)
                UsageSettingsSection() // #317 使っている量(作ったアプリの数・今のアプリの記録と写真)
                languageSection
                Section { AppIconSettingsRow() } // #232 ホーム画面のアイコン(全員・審査用の版も出す)
                voiceModelSection
                clarifySection
                speechSection
                appSection
                if !rt.dsl.app.notifications.isEmpty { notificationSection }
                #if !APP_STORE_REVIEW
                PushConsentSection() // #269
                #endif
                RetentionSettingsSection() // #482 記録の自動削除
                syncSection
                #if !APP_STORE_REVIEW // 審査用ビルドはフィードバックを外す(プライバシーの表示に無い種類を集めるため・#319)
                if rt.canOpenBetaWelcome { betaWelcomeSection } // #313: β の版だけ
                feedbackSection
                #endif
                PrivacySettingsSection() // #246: AI への送信の同意・プライバシーポリシー・利用規約
                supportSection
                if !Self.e2eDeleteAccount { deleteAccountSection }
            }
            .navigationTitle("設定")
            .task { await rt.refreshPlanUsage() } // #317 使っている量(5 分キャッシュ)
            .screenMapSheet { dismiss() } // 設定は音声シートから開くので、閉じればその画面の文脈で話せる
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                SheetCloseToolbarItem { dismiss() }
            }
            .confirmationDialog(L10n.key("アプリを初期状態に戻しますか？", review: "今の記録帳を初期状態に戻しますか？"), isPresented: $confirmReset, titleVisibility: .visible) {
                Button("初期状態に戻す", role: .destructive) {
                    rt.resetToInitial()
                    dismiss()
                }
                Button("キャンセル", role: .cancel) {}
            } message: {
                Text(L10n.key("作ったアプリは音声画面だけの状態に戻ります。サーバー上のデータは削除されません。", review: "今の記録帳は音声画面だけの状態に戻ります。サーバー上のデータは削除されません。"))
            }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
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
            Text(SignOutCopy.message(isAnonymous: session.user?.isAnonymous == true, prompt: prompt))
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
        .overlay {
            if isDeleting || isSigningOut {
                ProgressView(LocalizedStringKey(isDeleting ? "削除しています…" : "ログアウトしています…"))
                    .padding(Theme.Spacing.lg)
                    .background(.regularMaterial, in: RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
            }
        }
        .interactiveDismissDisabled(isDeleting || isSigningOut)
        #if !APP_STORE_REVIEW
        .coinSheets() // #630 設定の上にお店・プランの比較を重ねる
        .sheet(item: $feedback) { ctx in
            FeedbackComposerView(context: ctx, client: services.feedback, notifyWhenFixed: services.loopNotices.map { FeedbackFixedOptIn.live(client: $0) }) { feedback = nil }
        }
        #endif
        #if DEBUG
        // ヘッドレス E2E 用(#119): `METAMON_E2E_DELETE_ACCOUNT=<秒>` で、その秒数後に確認ダイアログ → さらに 4 秒後に「削除する」
        .task {
            guard let s = ProcessInfo.processInfo.environment["METAMON_E2E_DELETE_ACCOUNT"].flatMap(Double.init) else { return }
            try? await Task.sleep(for: .seconds(s))
            confirmDelete = true
            try? await Task.sleep(for: .seconds(4))
            confirmDelete = false
            print("[Metamon] E2E: deleting account")
            deleteAccount()
        }
        #endif
    }

    /// アカウント削除(#119・App Store 5.1.1(v))。サーバーで論理削除 → 端末のデータ → サインアウト。
    @ViewBuilder
    private var deleteAccountSection: some View {
        if session.user != nil {
            Section {
                Button(role: .destructive) { confirmDelete = true } label: {
                    Label("アカウントを削除", systemImage: "person.crop.circle.badge.xmark")
                        .foregroundStyle(rt.canDelete ? Color.red : Color.secondary) // 生成中は押せない見た目に
                }
                .disabled(isDeleting || isSigningOut || !rt.canDelete)
                .accessibilityIdentifier("settings.deleteAccount")
            } footer: {
                VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                    if !rt.canDelete {
                        Text(AppRuntime.deleteBlockedReason)
                            .accessibilityIdentifier("settings.deleteAccount.blocked")
                    }
                    Text(L10n.key("作ったアプリが使えなくなり、共有リンクと「探す」への掲載も取り下げます。この端末のデータも消えます。ほかの人と共有しているアプリへの投稿は残ります。", review: "用意した記録帳が使えなくなります。この端末のデータも消えます。"))
                }
            }
        }
    }

    private static var e2eDeleteAccount: Bool {
        #if DEBUG
        ProcessInfo.processInfo.environment["METAMON_E2E_DELETE_ACCOUNT"] != nil
        #else
        false
        #endif
    }

    private func deleteAccount() {
        guard rt.canDelete else { return } // 確認を出した後に生成が始まった
        isDeleting = true
        Task {
            defer { isDeleting = false }
            do {
                if try await session.deleteAccount(account: services.account, runtime: rt) { dismiss() }
            } catch {
                deleteError = ErrorCopy.accountFailureBody(error, context: "account delete")
            }
        }
    }

    /// ログアウト: 端末のデータをアカウント削除と同じに消してからサインアウトする。
    private func signOut(_ choice: SignOutFlow.Choice) {
        isSigningOut = true
        Task {
            defer { isSigningOut = false }
            do {
                switch try await session.signOutAndWipe(runtime: rt, choice: choice) {
                case .signedOut: dismiss()
                case .unsent(let n): signOutUnsent = n
                }
            } catch {
                signOutError = ErrorCopy.accountFailureBody(error, context: "sign out")
            }
        }
    }

    private var accountSection: some View {
        Section {
            if let user = session.user {
                HStack(spacing: Theme.Spacing.md) {
                    Image(systemName: user.isAnonymous ? "person.crop.circle.dashed" : "apple.logo")
                        .font(.title2)
                        .foregroundStyle(Theme.brandGradient)
                        .frame(width: 36)
                    Text(SupportInfo.accountStatus(isAnonymous: user.isAnonymous))
                        .font(.body.weight(.medium))
                        .accessibilityIdentifier("settings.accountStatus")
                }
                if user.isAnonymous {
                    // #276: uid を保ったまま Apple に昇格(作ったアプリとデータはそのまま)
                    AppleLinkButton()
                        .listRowInsets(EdgeInsets(top: Theme.Spacing.sm, leading: Theme.Spacing.md, bottom: Theme.Spacing.sm, trailing: Theme.Spacing.md))
                    if let message = session.linkError {
                        Text(message).font(.footnote).foregroundStyle(.red)
                    }
                }
                Button("ログアウト", role: .destructive) {
                    Task {
                        await rt.settlePendingRecordDeletes() // 取り消し待ちの削除(M11)も未送信に数える
                        signOutPrompt = SignOutFlow.prompt(pending: rt.recordSync.pendingCount, isOnline: rt.recordSync.isOnline)
                    }
                }
                .disabled(isDeleting || isSigningOut)
                .accessibilityIdentifier("settings.signOut")
            } else {
                Text("サインインしていません").foregroundStyle(.secondary)
            }
        } header: {
            Text("アカウント")
        } footer: {
            if session.user?.isAnonymous == true {
                Text(L10n.key("Apple でサインインすると、作ったアプリとデータをこのまま引き継げます。匿名のままアプリを消したり機種変更したりすると、作ったアプリに戻れません。",
                               review: "Apple でサインインすると、用意した記録帳とデータをこのまま引き継げます。匿名のままアプリを消したり機種変更したりすると、用意した記録帳に戻れません。"))
            }
        }
    }

    /// 表示言語(#44)。自動 = 端末の言語。切り替えは即時(ルートの `\.locale` と `LanguagePreference` の再描画)。
    /// 新しく作るアプリ・対話・音声認識の言語もこれに合わせる(作成済みアプリの文言はそのまま)。
    private var languageSection: some View {
        @Bindable var language = LanguagePreference.shared
        return Section {
            Picker("言語", selection: $language.setting) {
                Text("自動").tag(AppLanguage.system)
                Text(verbatim: "日本語").tag(AppLanguage.ja)
                Text(verbatim: "English").tag(AppLanguage.en)
            }
            .accessibilityIdentifier("settings.language")
        } footer: {
            Text(L10n.key("新しく作るアプリと質問もこの言語になります。作成済みのアプリの文言は変わりません。", review: "新しく用意する記録帳と質問もこの言語になります。用意済みの記録帳の文言は変わりません。"))
        }
    }

    private var voiceModelSection: some View {
        Section("音声モデル(オンデバイス)") {
            LabeledContent("状態") {
                HStack(spacing: Theme.Spacing.sm) {
                    Circle().fill(statusColor).frame(width: 8, height: 8)
                    Text(statusText)
                }
            }
            if case .failed = services.transcriber.status {
                Button("再試行") { Task { try? await services.transcriber.prepare() } }
            }
            if case .notDownloaded = services.transcriber.status, services.transcriber.needsDownload { // #246: 確認で「あとで」を選んだ人
                Button("ダウンロード(約\(AppRuntime.voiceModelMegabytes)MB)") { rt.approveModelDownload() }
            }
        }
    }

    /// 生成・改変の前の対話(#75 / #186)。create も edit も。OFF は一発で反映。
    private var clarifySection: some View {
        @Bindable var rt = rt
        return Section {
            Toggle(ClarifyCopy.settingTitle, isOn: $rt.clarifyEnabled)
                .tint(Theme.Colors.brandA)
                .accessibilityIdentifier("settings.clarify")
            Toggle(ClarifyCopy.autoBuildSettingTitle, isOn: $rt.clarifyAutoBuild)
                .tint(Theme.Colors.brandA)
                .disabled(!rt.clarifyEnabled)
                .accessibilityIdentifier("settings.clarifyAutoBuild")
        } header: {
            Text("対話")
        } footer: {
            Text(ClarifyCopy.settingFooter + "\n" + ClarifyCopy.autoBuildSettingFooter)
        }
    }

    /// 読み上げ(#43)。既定 OFF(理由は `SpeechReader.enabled`)。
    private var speechSection: some View {
        @Bindable var reader = SpeechReader.shared
        return Section {
            Toggle(SpeechCopy.settingTitle, isOn: $reader.enabled)
                .tint(Theme.Colors.brandA)
                .accessibilityIdentifier("settings.speech")
        } header: {
            Text(SpeechCopy.sectionTitle)
        } footer: {
            Text(SpeechCopy.settingFooter)
        }
    }

    private var appSection: some View {
        Section {
            Button {
                dismiss()
                Task { await rt.undo() }
            } label: {
                Label("元に戻す", systemImage: "arrow.uturn.backward")
            }
            .disabled(rt.isMorphing)
            ScreenMapSettingsRow() // #116
            #if !APP_STORE_REVIEW // 審査用ビルドは serverRules を作らない(#614)
            AutomationsSettingsRow() // #347 自動の動き(serverRules があるアプリだけ)
            #endif
            Button {
                dismiss()
                rt.replayTutorial() // #274
            } label: {
                Label("使い方を見る", systemImage: "questionmark.circle")
            }
            .accessibilityIdentifier("settings.tutorial")
            Button {
                dismiss()
                rt.replayCoachMarks() // #1441
            } label: {
                Label("操作のヒントをもう一度見る", systemImage: "hand.tap")
            }
            .accessibilityHint("長押しやドラッグなど、見た目から分からない操作の案内をもう一度出します")
            .accessibilityIdentifier("settings.coachMarks")
            NavigationLink {
                // #415: 選んだ例は音声画面(か対話)の入力欄に入る。送らない
                PromptTipsView { text in
                    rt.composerDraft = text
                    dismiss()
                }
            } label: {
                Label("頼み方のコツ", systemImage: "lightbulb")
            }
            .accessibilityIdentifier("settings.tips")
            Button(role: .destructive) { confirmReset = true } label: {
                Label(L10n.key("アプリをリセット(初期状態に戻す)", review: "記録帳をリセット(初期状態に戻す)"), systemImage: "arrow.counterclockwise")
            }
            .disabled(rt.isMorphing)
        } header: {
            Text(L10n.key("アプリ", review: "今の記録帳"))
        } footer: {
            Text(L10n.tr("現在: \(rt.dsl.app.name)") + (rt.isOffline ? L10n.tr("・オフライン(キャッシュ表示)") : "") + "\n" + L10n.pick(L10n.tr("「元に戻して」と話しても同じ。「別のアプリ。…」と話すと新しいアプリとして作ります。"), review: L10n.tr("「元に戻して」と話しても同じです。")))
        }
    }

    #if !APP_STORE_REVIEW
    /// β の案内(#313)。β(TestFlight)の版でだけ出す。
    private var betaWelcomeSection: some View {
        Section {
            Button {
                dismiss()
                rt.openBetaWelcome()
            } label: {
                Label("β 版のご案内", systemImage: "testtube.2")
            }
            .accessibilityIdentifier("settings.betaWelcome")
        }
    }

    /// フィードバック(#319)。スクショはシートの下のアプリの画面を、開いた時点で撮る(送るかは本人が選ぶ)。
    private var feedbackSection: some View {
        Section {
            Button {
                feedback = FeedbackContext.capture(runtime: rt, screenshot: FeedbackScreenshot.captureAppScreen())
            } label: {
                Label("フィードバックを送る", systemImage: "envelope")
            }
            .accessibilityIdentifier("settings.feedback")
            if let notices = services.loopNotices {
                NoticesSettingsRow(client: notices) // #374 「あなたの声で直りました」
            }
        } footer: {
            Text("端末を振っても開けます。何を送るかは送る前に選べます。")
        }
    }
    #endif

    /// このアプリの次の通知(#228): 予約が本当に入っているかをここで確かめられる。
    private var notificationSection: some View {
        Section {
            if rt.notificationsDenied {
                Button {
                    if let url = URL(string: UIApplication.openSettingsURLString) { UIApplication.shared.open(url) }
                } label: {
                    Label("通知がオフです(タップで設定を開く)", systemImage: "bell.slash")
                }
            }
            if rt.placesLocationOff {
                Button {
                    if let url = URL(string: UIApplication.openSettingsURLString) { UIApplication.shared.open(url) }
                } label: {
                    Label("位置情報が許可されていないため、場所に着いた時の通知は届きません(タップで設定を開く)", systemImage: "location.slash")
                }
                .accessibilityIdentifier("settings.placesLocationOff")
            } else if rt.notificationPlaces > 0 {
                Label("保存した場所 \(rt.notificationPlaces) か所の近くに着いたら知らせます", systemImage: "location")
                    .foregroundStyle(.secondary)
            }
            let list = UpcomingNotificationList(rt.upcomingNotifications, places: rt.notificationPlaces,
                                                locationOff: rt.placesLocationOff)
            if list.isEmpty {
                Text("予約中の通知はありません").foregroundStyle(.secondary)
            }
            ForEach(list.rows) { n in
                LabeledContent {
                    Text(n.when).monospacedDigit()
                } label: {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(n.title)
                        if !n.body.isEmpty { Text(n.body).font(.caption).foregroundStyle(.secondary).lineLimit(1) }
                    }
                }
            }
        } header: {
            Text("次の通知")
        } footer: {
            Text("アプリを閉じていても、この時刻に届きます。")
        }
        .task { await rt.refreshNotifications() }
    }

    /// レコードの同期状態(Issue #48): 最新 / 未送信 N 件 / オフライン。「今すぐ同期」でキューを流し直す。
    private var syncSection: some View {
        Section {
            LabeledContent("同期") {
                HStack(spacing: Theme.Spacing.sm) {
                    Circle().fill(syncColor).frame(width: 8, height: 8)
                    Text(syncLabel)
                }
            }
            Button {
                rt.recordSync.syncNow()
            } label: {
                Label("今すぐ同期", systemImage: "arrow.triangle.2.circlepath")
            }
            .disabled(!rt.recordSync.isOnline || rt.recordSync.pendingCount == 0)
        } header: {
            Text("データ")
        } footer: {
            Text("記録はまずこの端末に保存され、接続があるときにサーバーへ送られます。")
        }
    }

    /// `RecordSyncStatus.label`(Services・日本語固定)の表示言語対応版(#44)
    private var syncLabel: String {
        switch rt.recordSync.status {
        case .synced: L10n.tr("最新")
        case .pending(let n): L10n.tr("未送信 \(n) 件")
        case .syncing(let n): L10n.tr("送信中(\(n) 件)")
        case .offline(let n): n == 0 ? L10n.tr("オフライン") : L10n.tr("オフライン・未送信 \(n) 件")
        }
    }

    private var syncColor: Color {
        switch rt.recordSync.status {
        case .synced: .green
        case .syncing, .pending: .orange
        case .offline: .gray
        }
    }

    /// 問い合わせ用の識別子(#279)。1 画面目に英数字の識別子を出さないよう、既定で畳む。
    private var supportSection: some View {
        let info = supportInfo
        return Section {
            Link(destination: LegalDocument.contactURL) { // 審査用の版にも出す(サポートの窓口)
                Label("お問い合わせ", systemImage: "envelope")
            }
            .font(.body)
            .accessibilityIdentifier("settings.contact")
            LicensesLink() // #306: 審査用の版にも出す
            DisclosureGroup(isExpanded: $showSupportDetails) {
                ForEach(info.rows, id: \.label) { row in
                    LabeledContent(row.label) {
                        Text(verbatim: row.value)
                            .font(.footnote.monospaced())
                            .lineLimit(1)
                            .truncationMode(.middle)
                            .textSelection(.enabled)
                    }
                }
                Button {
                    UIPasteboard.general.string = info.clipboardText
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
        .font(.footnote)
    }

    private var supportInfo: SupportInfo {
        let bundle = Bundle.main
        return SupportInfo(
            uid: session.user?.uid,
            voiceModel: BuildConfig.whisperModel,
            aiEndpoint: services.ai is MockAIClient ? L10n.tr("モック") : (BuildConfig.aiBaseURL?.host() ?? "live"),
            appConfigEndpoint: services.isLive ? (BuildConfig.appConfigBaseURL?.host() ?? "live") : L10n.tr("インメモリ"),
            appVersion: bundle.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "-",
            build: bundle.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "-",
            appRevision: rt.document.version)
    }

    private var statusText: String {
        switch services.transcriber.status {
        case .notDownloaded: L10n.tr("未ダウンロード")
        case .downloading: L10n.tr("ダウンロード中 \(Int(services.transcriber.downloadProgress * 100))%")
        case .loading: L10n.tr("読み込み中")
        case .ready: L10n.tr("準備完了")
        case .recognizing: L10n.tr("認識中")
        case .failed(let m): L10n.tr("失敗: \(m)")
        }
    }

    private var statusColor: Color {
        switch services.transcriber.status {
        case .ready, .recognizing: .green
        case .failed: .red
        default: .orange
        }
    }
}

#Preview {
    let services = BuildConfig.make(environment: .mock)
    SettingsSheet()
        .environment(AppRuntime(document: .fallback, services: services))
        .environment(AuthSession(service: services.auth))
        .environment(\.services, services)
}
