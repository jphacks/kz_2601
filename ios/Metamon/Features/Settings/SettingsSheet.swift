import SwiftUI
import MetamonServices
import MetamonRenderer

/// 歯車から開く設定シート。アカウント・音声モデル・接続先・アプリのリセット。
struct SettingsSheet: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(AuthSession.self) private var session
    @Environment(\.services) private var services
    @Environment(\.dismiss) private var dismiss
    @State private var confirmReset = false
    @State private var confirmDelete = false
    @State private var isDeleting = false
    @State private var deleteError: String?

    var body: some View {
        NavigationStack {
            List {
                if Self.e2eDeleteAccount { deleteAccountSection } // E2E: 削除ボタンを画面内に出す(シミュレータはスクロール操作できない)
                accountSection
                ProSettingsSection() // 課金(#77)
                languageSection
                voiceModelSection
                clarifySection
                speechSection
                appSection
                if !rt.dsl.app.notifications.isEmpty { notificationSection }
                syncSection
                PrivacySettingsSection() // #246: AI への送信の同意・プライバシーポリシー・利用規約
                connectionSection
                if !Self.e2eDeleteAccount { deleteAccountSection }
            }
            .navigationTitle("設定")
            .screenMapSheet { dismiss() } // 設定は音声シートから開くので、閉じればその画面の文脈で話せる
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) { Button("閉じる") { dismiss() } }
            }
            .confirmationDialog("アプリを初期状態に戻しますか？", isPresented: $confirmReset, titleVisibility: .visible) {
                Button("初期状態に戻す", role: .destructive) {
                    rt.resetToInitial()
                    dismiss()
                }
                Button("キャンセル", role: .cancel) {}
            } message: {
                Text("生成したアプリは音声画面だけの状態に戻ります。サーバー上のデータは削除されません。")
            }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
        .alert("アカウントを削除しますか？", isPresented: $confirmDelete) {
            Button("削除する", role: .destructive) { deleteAccount() }
            Button("キャンセル", role: .cancel) {}
        } message: {
            Text("アカウントとすべてのアプリが使えなくなります。元に戻せません。")
        }
        .alert("アカウントを削除できませんでした", isPresented: Binding(get: { deleteError != nil }, set: { if !$0 { deleteError = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(deleteError ?? "")
        }
        .overlay {
            if isDeleting {
                ProgressView("削除しています…")
                    .padding(Theme.Spacing.lg)
                    .background(.regularMaterial, in: RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
            }
        }
        .interactiveDismissDisabled(isDeleting)
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
                        .foregroundStyle(.red)
                }
                .disabled(isDeleting)
                .accessibilityIdentifier("settings.deleteAccount")
            } footer: {
                Text("作ったアプリが使えなくなり、共有リンクと「探す」への掲載も取り下げます。この端末のデータも消えます。ほかの人と共有しているアプリへの投稿は残ります。")
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
        isDeleting = true
        Task {
            defer { isDeleting = false }
            do {
                if try await session.deleteAccount(account: services.account, runtime: rt) { dismiss() }
            } catch {
                deleteError = error.localizedDescription
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
                    VStack(alignment: .leading, spacing: 2) {
                        Text(user.isAnonymous ? L10n.tr("匿名ユーザー") : L10n.tr("Apple でサインイン済み"))
                            .font(.body.weight(.medium))
                        Text("uid \(user.uid)")
                            .font(.caption2.monospaced())
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                            .truncationMode(.middle)
                            .textSelection(.enabled)
                    }
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
                    session.signOut()
                    dismiss()
                }
            } else {
                Text("サインインしていません").foregroundStyle(.secondary)
            }
        } header: {
            Text("アカウント")
        } footer: {
            if session.user?.isAnonymous == true {
                Text("Apple でサインインすると、作ったアプリとデータをこのまま引き継げます。匿名のままアプリを消したり機種変更したりすると、作ったアプリに戻れません。")
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
            Text("新しく作るアプリと質問もこの言語になります。作成済みのアプリの文言は変わりません。")
        }
    }

    private var voiceModelSection: some View {
        Section("音声モデル(オンデバイス)") {
            LabeledContent("モデル", value: BuildConfig.whisperModel)
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
                Label("ひとつ前の状態に戻す", systemImage: "arrow.uturn.backward")
            }
            .disabled(rt.isMorphing)
            ScreenMapSettingsRow() // #116
            Button {
                dismiss()
                rt.replayTutorial() // #274
            } label: {
                Label("使い方を見る", systemImage: "questionmark.circle")
            }
            .accessibilityIdentifier("settings.tutorial")
            Button(role: .destructive) { confirmReset = true } label: {
                Label("アプリをリセット(初期状態に戻す)", systemImage: "arrow.counterclockwise")
            }
            .disabled(rt.isMorphing)
        } header: {
            Text("アプリ")
        } footer: {
            Text(L10n.tr("現在: \(rt.dsl.app.name)(v\(rt.document.version))") + (rt.isOffline ? L10n.tr("・オフライン(キャッシュ表示)") : "") + "\n" + L10n.tr("「元に戻して」と話しても同じ。「別のアプリ。…」と話すと新しいアプリとして作ります。"))
        }
    }

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
            if rt.upcomingNotifications.isEmpty {
                Text("予約中の通知はありません").foregroundStyle(.secondary)
            }
            ForEach(Array(rt.upcomingNotifications.prefix(5).enumerated()), id: \.offset) { _, n in
                LabeledContent {
                    Text(NotificationCopy.when(n.date)).monospacedDigit()
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

    private var connectionSection: some View {
        Section("接続先") {
            LabeledContent("ai", value: services.ai is MockAIClient ? L10n.tr("モック") : (BuildConfig.aiBaseURL?.host() ?? "live"))
            LabeledContent("appconfig", value: services.isLive ? (BuildConfig.appConfigBaseURL?.host() ?? "live") : L10n.tr("インメモリ"))
            LabeledContent("バージョン", value: Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "-")
        }
        .font(.footnote)
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
