// 審査用ビルドでは外す(#319・iOS と同じ)。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// フィードバックのウインドウ(#983)。ヘルプのメニューと設定の「詳細」から開く。
/// 開いた時点でアプリのウインドウを撮り、閉じたら捨てる。二重には開かない(開いていれば前に出すだけ)。
@MainActor
@Observable
final class MacFeedbackCenter {
    static let shared = MacFeedbackCenter()
    static let windowId = "feedback"

    /// Mac に Firebase が無い間(#930)はモック(送った内容を覚えるだけ)。iOS の `FeedbackServiceKey` の既定と同じ
    let client: any FeedbackClient = MockFeedbackClient()
    private(set) var context: MacFeedbackContext?

    func open(_ openWindow: OpenWindowAction) {
        if context == nil { context = .capture(screenshot: MacFeedbackScreenshot.captureAppWindow()) }
        openWindow(id: Self.windowId)
    }

    /// 前の起動から戻ったウインドウなど、撮らずに開いた時
    func ensureContext() {
        if context == nil { context = .capture(screenshot: nil) }
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
                MacFeedbackComposerView(context: context, client: center.client) {
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
struct MacFeedbackComposerView: View {
    let context: MacFeedbackContext
    let client: any FeedbackClient
    var onClose: () -> Void

    @State private var kind: FeedbackSubmission.Kind = .bug
    @State private var message = ""
    @State private var attachScreenshot = false
    @State private var attachDevice = false
    @State private var agreed = false
    @State private var sending = false
    @State private var error: String?
    @State private var receipt: FeedbackReceipt?

    private var trimmed: String { message.trimmingCharacters(in: .whitespacesAndNewlines) }
    private var canSend: Bool {
        !trimmed.isEmpty && trimmed.count <= FeedbackConsent.maxMessageLength && agreed && !sending && receipt == nil
    }

    var body: some View {
        VStack(spacing: 0) {
            Form {
                messageSection
                attachmentSection
                consentSection
            }
            .formStyle(.grouped)
            Divider()
            HStack {
                Spacer()
                Button("キャンセル", action: onClose)
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
        .alert("送れませんでした", isPresented: Binding(get: { error != nil }, set: { if !$0 { error = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(error ?? "")
        }
        .alert("送信しました", isPresented: Binding(get: { receipt != nil }, set: { _ in })) {
            Button("閉じる", action: onClose)
        } message: {
            Text("ありがとうございます。いただいた内容は改善に使います。")
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
        var submission = FeedbackSubmission(kind: kind, message: trimmed)
        if attachScreenshot, let shot = context.screenshot {
            guard let jpeg = MacFeedbackScreenshot.jpeg(shot) else {
                error = L10n.tr("スクショを小さくできませんでした。スクショをオフにして送ってください。")
                return
            }
            submission.screenshotJPEG = jpeg
        }
        if attachDevice { submission.device = context.device }
        sending = true
        Task {
            defer { sending = false }
            do {
                receipt = try await client.send(submission)
            } catch {
                self.error = FeedbackCopy.message(for: error)
            }
        }
    }
}
#endif
