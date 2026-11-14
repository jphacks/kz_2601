// 審査用ビルドでは外す(#319・iOS と同じ)。
#if !APP_STORE_REVIEW
import SwiftUI
import UserNotifications
import MetamonServices

/// フィードバックのウインドウ(#983)。ヘルプのメニューと設定の「詳細」から開く。
/// 開いた時点でアプリのウインドウを撮り、閉じたら捨てる。二重には開かない(開いていれば前に出すだけ)。
@MainActor
@Observable
final class MacFeedbackCenter {
    static let shared = MacFeedbackCenter()
    static let windowId = "feedback"

    /// Firebase につながれば appconfig へ送る(#930)。つながらない時はモック(送った内容を覚えるだけ・iOS の `FeedbackServiceKey` の既定と同じ)
    let client: any FeedbackClient = MacServices.shared.feedback
    /// 「直ったら知らせてほしい」(#364)。モック(appconfig 無し)なら nil で出さない
    let notifyWhenFixed: FeedbackFixedOptIn? = MacServices.shared.loopNotices.map { FeedbackFixedOptIn.live(client: $0) }
    private(set) var context: MacFeedbackContext?
    /// 中央に出しているアプリ(サーバーにある時だけ・#1330)。`MacRootView` が選択と版の変化で入れ替える
    var currentApp: MacFeedbackContext.CurrentApp?

    func open(_ openWindow: OpenWindowAction) {
        if context == nil { context = .capture(screenshot: MacFeedbackScreenshot.captureAppWindow(), app: currentApp) }
        openWindow(id: Self.windowId)
    }

    /// 前の起動から戻ったウインドウなど、撮らずに開いた時
    func ensureContext() {
        if context == nil { context = .capture(screenshot: nil, app: currentApp) }
    }

    func close() { context = nil }
}

/// ウインドウの中身。閉じる(赤いボタンでも)と撮ったスクショを捨てる。
struct MacFeedbackWindow: View {
    @Environment(\.dismissWindow) private var dismissWindow
    private let center = MacFeedbackCenter.shared

    var body: some View {
        Group {
            if let context = center.context {
                MacFeedbackComposerView(context: context, client: center.client, notifyWhenFixed: center.notifyWhenFixed) {
                    dismissWindow(id: MacFeedbackCenter.windowId)
                }
                .id(context.id)
            } else {
                Color.clear.frame(width: 520, height: 560)
            }
        }
        .onAppear { center.ensureContext() }
        .onDisappear { center.close() }
    }
}

/// フィードバックを書いて送る(iOS の `FeedbackComposerView`・#319)。
/// - 添付はどれも既定でオフ。オンにした添付は、送る中身をその場で見せる
/// - 送る前に同意文を見せ、同意のチェックを入れるまで送れない(`FeedbackConsent.version` を一緒に送る)
/// - 記録(入力したデータ)の中身は送らない
/// - 書きかけ(送っていない文)のまま閉じる(キャンセル・Esc・⌘W・閉じるボタン)と、捨てるかを聞く(#1309)
struct MacFeedbackComposerView: View {
    let context: MacFeedbackContext
    let client: any FeedbackClient
    var onClose: () -> Void

    /// 「直ったら知らせてほしい」(#364)。既定はオフ・nil(モック)なら出さない
    @State private var fixedNotice: MacFixedNoticeChoice
    /// 送った後の一言(同意を保存できなかった・通知が許可されていない)
    @State private var fixedNoticeNote: String?
    @State private var kind: FeedbackSubmission.Kind = .bug
    @State private var message = ""
    @State private var attachScreenshot = false
    @State private var attachDevice = false
    @State private var attachApp = false
    @State private var agreed = false
    @State private var sending = false
    @State private var error: String?
    @State private var receipt: FeedbackReceipt?
    /// 書きかけを捨てるかの確認。「捨てる」で呼ぶ閉じ方
    @State private var discardThen: (() -> Void)?

    init(context: MacFeedbackContext, client: any FeedbackClient, notifyWhenFixed: FeedbackFixedOptIn? = nil,
         onClose: @escaping () -> Void) {
        self.context = context
        self.client = client
        self.onClose = onClose
        _fixedNotice = State(initialValue: MacFixedNoticeChoice(optIn: notifyWhenFixed))
    }

    private var trimmed: String { message.trimmingCharacters(in: .whitespacesAndNewlines) }
    /// 送っていない文がある(送った後・空白だけは書きかけにしない)
    private var hasDraft: Bool { MacFeedbackDraft.isUnsent(message: message, sent: receipt != nil) }

    /// 閉じる前に、書きかけなら確認を出す
    private func requestClose(_ close: @escaping () -> Void) {
        if hasDraft { discardThen = close } else { close() }
    }

    private var canSend: Bool {
        MacFeedbackDraft.canSend(message: message, agreed: agreed, sending: sending, sent: receipt != nil)
    }

    var body: some View {
        VStack(spacing: 0) {
            Form {
                messageSection
                attachmentSection
                if fixedNotice.isAvailable { fixedNoticeSection }
                consentSection
            }
            .formStyle(.grouped)
            Divider()
            HStack {
                Spacer()
                Button("キャンセル") { requestClose(onClose) }
                    .keyboardShortcut(.cancelAction)
                    .disabled(sending)
                if sending {
                    ProgressView().controlSize(.small)
                }
                Button("送信") { send() }
                    .keyboardShortcut(.defaultAction)
                    .disabled(!canSend)
                    .accessibilityIdentifier("feedback.send")
            }
            .padding(16)
        }
        .frame(width: 520)
        .frame(minHeight: 560)
        // ⌘W・閉じるボタンも、書きかけなら同じ確認を出す
        .background {
            WindowCloseGuard { closeAgain in
                guard hasDraft, !sending else { return true }
                requestClose(closeAgain)
                return false
            }
        }
        .confirmationDialog("書いた内容を捨てますか?",
                            isPresented: Binding(get: { discardThen != nil }, set: { if !$0 { discardThen = nil } }),
                            titleVisibility: .visible) {
            Button("捨てる", role: .destructive) {
                let close = discardThen
                discardThen = nil
                close?()
            }
            Button("書き続ける", role: .cancel) { discardThen = nil }
        } message: {
            Text("送っていないフィードバックは残りません。")
        }
        .alert("送れませんでした", isPresented: Binding(get: { error != nil }, set: { if !$0 { error = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(error ?? "")
        }
        .alert("送信しました", isPresented: Binding(get: { receipt != nil }, set: { _ in })) {
            Button("閉じる", action: onClose)
        } message: {
            if let fixedNoticeNote {
                Text("ありがとうございます。いただいた内容は改善に使います。") + Text(verbatim: "\n") + Text(verbatim: fixedNoticeNote)
            } else {
                Text("ありがとうございます。いただいた内容は改善に使います。")
            }
        }
    }

    private var messageSection: some View {
        Section {
            Picker("種類", selection: $kind) {
                Text("不具合").tag(FeedbackSubmission.Kind.bug)
                Text("要望").tag(FeedbackSubmission.Kind.idea)
                Text("その他").tag(FeedbackSubmission.Kind.other)
            }
            .pickerStyle(.segmented)
            ZStack(alignment: .topLeading) {
                if message.isEmpty {
                    Text("困ったこと・思ったのと違ったこと・欲しい機能など")
                        .foregroundStyle(.tertiary)
                        .padding(.top, 1)
                        .padding(.leading, 5)
                        .allowsHitTesting(false)
                }
                TextEditor(text: $message)
                    .font(.body)
                    .scrollContentBackground(.hidden)
                    .frame(minHeight: 120)
                    .accessibilityIdentifier("feedback.message")
            }
        } footer: {
            if trimmed.count > FeedbackConsent.maxMessageLength {
                Text(L10n.tr("\(FeedbackConsent.maxMessageLength) 文字までです")).foregroundStyle(.red)
            }
        }
    }

    private var attachmentSection: some View {
        Section {
            if let shot = context.screenshot {
                Toggle("今の画面のスクショ", isOn: $attachScreenshot)
                    .accessibilityIdentifier("feedback.attachScreenshot")
                if attachScreenshot {
                    Image(decorative: shot, scale: 1)
                        .resizable()
                        .scaledToFit()
                        .frame(maxHeight: 240)
                        .frame(maxWidth: .infinity)
                        .clipShape(RoundedRectangle(cornerRadius: 8))
                        .accessibilityLabel("送るスクショ")
                    Text("この画像をそのまま送ります。個人の情報(名前・金額・連絡先など)が写っていないか確かめてください。")
                        .font(.footnote).foregroundStyle(.secondary)
                }
            }
            if let app = context.app {
                Toggle("今のアプリの設計(DSL)", isOn: $attachApp)
                    .accessibilityIdentifier("feedback.attachApp")
                if attachApp { appPreview(app) }
            }
            Toggle("アプリの版・OS・機種", isOn: $attachDevice)
                .accessibilityIdentifier("feedback.attachDevice")
            if attachDevice {
                let d = context.device
                LabeledContent("アプリの版", value: "\(d.appVersion) (\(d.build))")
                LabeledContent("OS", value: d.os)
                LabeledContent("機種", value: d.model)
                LabeledContent("言語", value: d.locale)
            }
        } header: {
            Text("添付(どれも任意)")
        } footer: {
            Text("記録(入力したデータ)の中身は送りません。オンにしたものだけを送ります。")
        }
    }

    /// 送る設計の中身(iOS と同じ: 名前・画面・項目・送る DSL の全文)
    @ViewBuilder
    private func appPreview(_ app: MacFeedbackContext.AppSummary) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(L10n.tr("「\(app.name)」の画面と項目の設計")).font(.subheadline.weight(.medium))
            Text(L10n.tr("画面: \(app.screens.joined(separator: "、"))")).font(.footnote)
            if !app.entities.isEmpty {
                Text(L10n.tr("項目: \(app.entities.joined(separator: "、"))")).font(.footnote)
            }
            Text("アプリの名前・画面の文言・項目の名前が含まれます。記録は含まれません。")
                .font(.footnote).foregroundStyle(.secondary)
        }
        if let json = app.json {
            DisclosureGroup("送る DSL をすべて見る") {
                ScrollView([.vertical, .horizontal]) {
                    Text(verbatim: json)
                        .font(.caption2.monospaced())
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .frame(maxHeight: 240)
            }
            .font(.footnote)
        }
    }

    private var fixedNoticeSection: some View {
        Section {
            Toggle("直ったら知らせてほしい", isOn: Binding(get: { fixedNotice.wants }, set: { on in Task { await fixedNotice.set(on) } }))
                .accessibilityIdentifier("feedback.notifyWhenFixed")
        } footer: {
            Text("この内容がもとで直った時に、通知でお知らせします。設定の「お知らせ」にはオフでも届きます。")
        }
    }

    private var consentSection: some View {
        Section {
            Text(FeedbackCopy.consent)
                .font(.footnote)
                .fixedSize(horizontal: false, vertical: true)
            Toggle("上の内容に同意して送る", isOn: $agreed)
                .accessibilityIdentifier("feedback.agree")
        } header: {
            Text("送る前に")
        }
    }

    private func send() {
        guard canSend else { return }
        var submission = MacFeedbackDraft.submission(kind: kind, message: message, context: context,
                                                     attachDevice: attachDevice, attachApp: attachApp)
        if attachScreenshot, let shot = context.screenshot {
            guard let jpeg = MacFeedbackScreenshot.jpeg(shot) else {
                error = L10n.tr("スクショを小さくできませんでした。スクショをオフにして送ってください。")
                return
            }
            submission.screenshotJPEG = jpeg
        }
        sending = true
        Task {
            defer { sending = false }
            do {
                let sent = try await client.send(submission)
                fixedNoticeNote = await fixedNotice.afterSent()
                receipt = sent
            } catch {
                self.error = FeedbackCopy.message(for: error)
            }
        }
    }
}
/// フィードバックの「直ったら知らせてほしい」(#364・iOS の `FeedbackComposerView` と同じ動き)。
/// オンにした時だけ通知の許可を聞き(未決定の時だけ OS の確認)、同意(`PUT /v1/me/notify-consent/feedback-fixed`)は送った後に保存する。
@MainActor
@Observable
final class MacFixedNoticeChoice {
    @ObservationIgnored let optIn: FeedbackFixedOptIn?
    /// 既定はオフ(同意をはっきりさせるため)
    private(set) var wants = false

    init(optIn: FeedbackFixedOptIn?) {
        self.optIn = optIn
    }

    var isAvailable: Bool { optIn != nil }

    func set(_ on: Bool) async {
        wants = on
        guard on, let optIn else { return }
        _ = await optIn.requestPermissionIfNeeded()
    }

    /// 送った後に呼ぶ。オンの時だけ同意を保存し、届かない時だけ一言返す
    func afterSent() async -> String? {
        guard wants, let optIn else { return nil }
        guard await optIn.saveConsent() else {
            return L10n.tr("「直ったら知らせてほしい」は保存できませんでした。設定の「お知らせ」から変えられます。")
        }
        guard await optIn.authorization() != .denied else {
            return L10n.tr("通知が許可されていないため、直った時は設定の「お知らせ」でお知らせします。")
        }
        return nil
    }
}

/// 書きかけの判定(#1309)と、送れるか・送る中身(#2325)。
enum MacFeedbackDraft {
    /// 送った後と、空白・改行だけは捨てても失うものが無いので聞かない
    static func isUnsent(message: String, sent: Bool) -> Bool {
        !sent && !message.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    /// 空・上限超え・同意なし・送信中・送った後は送れない
    static func canSend(message: String, agreed: Bool, sending: Bool, sent: Bool) -> Bool {
        let trimmed = message.trimmingCharacters(in: .whitespacesAndNewlines)
        return !trimmed.isEmpty && trimmed.count <= FeedbackConsent.maxMessageLength && agreed && !sending && !sent
    }

    /// 送る中身(スクショは view で足す)。オンにした添付だけ。アプリは appId と版だけで記録は送らない
    static func submission(kind: FeedbackSubmission.Kind, message: String, context: MacFeedbackContext,
                           attachDevice: Bool, attachApp: Bool) -> FeedbackSubmission {
        var submission = FeedbackSubmission(kind: kind, message: message.trimmingCharacters(in: .whitespacesAndNewlines))
        if attachDevice { submission.device = context.device }
        if attachApp { submission.app = context.app?.ref }
        return submission
    }
}
#endif
