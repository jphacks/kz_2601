// 審査用ビルドでは外す(#319・docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices
import MetamonRenderer

/// フィードバックを書いて送る画面(#319)。
/// - 添付はどれも既定でオフ。オンにした添付は、送る中身をその場で見せる
/// - 送る前に同意文を見せ、同意のトグルを入れるまで送れない(`FeedbackConsent.version` を一緒に送る)
/// - 記録(入力したデータ)の中身は送らない
struct FeedbackComposerView: View {
    let context: FeedbackContext
    let client: any FeedbackClient
    var onClose: () -> Void

    @State private var kind: FeedbackSubmission.Kind = .bug
    @State private var message = ""
    @State private var attachScreenshot = false
    @State private var attachApp = false
    @State private var attachDevice = false
    @State private var agreed = false
    @State private var sending = false
    @State private var error: String?
    @State private var receipt: FeedbackReceipt?
    @FocusState private var editing: Bool

    private var trimmed: String { message.trimmingCharacters(in: .whitespacesAndNewlines) }
    private var canSend: Bool {
        !trimmed.isEmpty && trimmed.count <= FeedbackConsent.maxMessageLength && agreed && !sending && receipt == nil
    }

    var body: some View {
        NavigationStack {
            Form {
                messageSection
                attachmentSection
                consentSection
            }
            .navigationTitle("フィードバック")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("キャンセル", action: onClose).disabled(sending)
                }
                ToolbarItem(placement: .confirmationAction) {
                    if sending {
                        ProgressView()
                    } else {
                        Button("送信") { send() }
                            .disabled(!canSend)
                            .accessibilityIdentifier("feedback.send")
                    }
                }
            }
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
        .interactiveDismissDisabled(sending || !trimmed.isEmpty)
        .onAppear { FeedbackPresenter.isOpen = true }
        .onDisappear { FeedbackPresenter.isOpen = false }
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
                        .padding(.top, 8)
                        .padding(.leading, 5)
                        .allowsHitTesting(false)
                }
                TextEditor(text: $message)
                    .frame(minHeight: 120)
                    .focused($editing)
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
                    Image(uiImage: shot)
                        .resizable()
                        .scaledToFit()
                        .frame(maxHeight: 280)
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

    @ViewBuilder
    private func appPreview(_ app: FeedbackContext.AppSummary) -> some View {
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

    private var consentSection: some View {
        Section {
            Text(L10n.tr("送った内容(本文と、オンにした添付)は、Tailor It の不具合を直し、改善するために運営者が読みます。Google Cloud(Firestore)に保存し、\(FeedbackConsent.retentionDays) 日後に消去します。アカウントを削除したときも消去します。第三者には渡しません。"))
                .font(.footnote)
            Toggle("上の内容に同意して送る", isOn: $agreed)
                .accessibilityIdentifier("feedback.agree")
        } header: {
            Text("送る前に")
        }
    }

    private func send() {
        editing = false
        var submission = FeedbackSubmission(kind: kind, message: trimmed)
        if attachScreenshot, let shot = context.screenshot {
            guard let jpeg = FeedbackScreenshot.jpeg(shot) else {
                error = L10n.tr("スクショを小さくできませんでした。スクショをオフにして送ってください。")
                return
            }
            submission.screenshotJPEG = jpeg
        }
        if attachApp { submission.app = context.app?.ref }
        if attachDevice { submission.device = context.device }
        sending = true
        Task {
            defer { sending = false }
            do {
                receipt = try await client.send(submission)
            } catch {
                self.error = Self.message(for: error)
            }
        }
    }

    static func message(for error: Error) -> String {
        switch FailureKind.classify(error) {
        case .rateLimited: L10n.tr("今日はこれ以上送れません。明日また送ってください。")
        case .offline: L10n.tr("接続がありません。つながる場所でもう一度送ってください。")
        case .timeout, .busy: L10n.tr("サーバーが混み合っています。少し待ってからもう一度送ってください。")
        case .sessionExpired: L10n.tr("サインインし直してから送ってください。")
        default: L10n.tr("もう一度送ってください。続く場合は添付を減らしてください。")
        }
    }
}

/// どこからでも(シートの上でも)フィードバックの画面を出す。端末を振った時に使う。
@MainActor
enum FeedbackPresenter {
    /// 二重に開かない
    static var isOpen = false

    static func present(context: FeedbackContext, client: any FeedbackClient) {
        guard !isOpen, let top = topViewController() else { return }
        isOpen = true
        var host: UIViewController?
        let view = FeedbackComposerView(context: context, client: client) { host?.dismiss(animated: true) }
            .environment(\.locale, LanguagePreference.shared.locale)
        let controller = UIHostingController(rootView: view)
        host = controller
        top.present(controller, animated: true)
    }

    private static func topViewController() -> UIViewController? {
        let window = UIApplication.shared.connectedScenes
            .compactMap { $0 as? UIWindowScene }
            .flatMap(\.windows)
            .first { $0.isKeyWindow }
        var top = window?.rootViewController
        while let next = top?.presentedViewController, !next.isBeingDismissed { top = next }
        return top
    }
}

/// 端末を振ったら、その時に見えている画面(シートも含む)を撮ってフィードバックの画面を開く。
struct FeedbackShakeHost: ViewModifier {
    @Environment(AppRuntime.self) private var rt
    @Environment(\.services) private var services

    func body(content: Content) -> some View {
        content
            .onReceive(NotificationCenter.default.publisher(for: .feedbackDeviceShaken)) { _ in open() }
            #if DEBUG
            .task {
                // ヘッドレス E2E 用: `METAMON_AUTO_FEEDBACK=<秒>` でその秒数後に振った時と同じく開く
                guard let s = ProcessInfo.processInfo.environment["METAMON_AUTO_FEEDBACK"].flatMap(Double.init) else { return }
                try? await Task.sleep(for: .seconds(s))
                open()
            }
            #endif
    }

    private func open() {
        guard !FeedbackPresenter.isOpen else { return }
        let ctx = FeedbackContext.capture(runtime: rt, screenshot: FeedbackScreenshot.captureWindow())
        FeedbackPresenter.present(context: ctx, client: services.feedback)
    }
}

extension Notification.Name {
    /// 端末を振った(`UIWindow.motionEnded`)。
    static let feedbackDeviceShaken = Notification.Name("metamon.feedback.deviceShaken")
}

extension UIWindow {
    // 振ったらフィードバックの画面を開く(RootView が受ける)。文字入力中の「取り消し」は first responder が先に受けるので邪魔しない
    override open func motionEnded(_ motion: UIEvent.EventSubtype, with event: UIEvent?) {
        if motion == .motionShake { NotificationCenter.default.post(name: .feedbackDeviceShaken, object: nil) }
        super.motionEnded(motion, with: event)
    }
}
#endif
