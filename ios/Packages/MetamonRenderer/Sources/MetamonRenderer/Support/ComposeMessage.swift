import Foundation
#if canImport(UIKit)
import UIKit
#endif
#if canImport(AppKit)
import AppKit
#endif
#if canImport(MessageUI) && canImport(UIKit)
import MessageUI
#endif

/// 宛先・件名・本文を入れた「メッセージ / メールの下書き」を開く口(#2223・親 #1812)。
/// 部品・式・AI の決まり・`supportedFeatures` の申告はまだ無い(#1775 の決定待ち)。
/// `DeviceCalendarSource` / `PhotoLibraryAdding` と同じ形: protocol + 実体(`LiveComposePresenter`)+ 偽物(`FakeComposePresenter`)。
///
/// 2 段構え:
/// 1. `MFMessageComposeViewController` / `MFMailComposeViewController` が使えるなら、それで下書きを出す
/// 2. 使えない時(シミュレータ・メールの設定が無い・Mac)は `sms:` / `mailto:` の URL で開く
///
/// 宛先・件名・本文はログ・クラッシュ報告・サーバーに出さない(`description` も伏せる)。

// MARK: - 値

public enum ComposeChannel: String, Sendable, Equatable, CaseIterable {
    /// SMS / iMessage(`MFMessageComposeViewController` ・ `sms:`)
    case message
    /// メール(`MFMailComposeViewController` ・ `mailto:`)
    case mail
}

/// 下書きの中身。件名はメールだけ(`sms:` には件名が無いので捨てる)。
public struct ComposeDraft: Sendable, Equatable, CustomStringConvertible, CustomDebugStringConvertible {
    /// 宛先(メールアドレス / 電話番号)。空文字と前後の空白は組み立ての時に落とす
    public var recipients: [String]
    public var subject: String?
    public var body: String?

    public init(recipients: [String] = [], subject: String? = nil, body: String? = nil) {
        self.recipients = recipients
        self.subject = subject
        self.body = body
    }

    /// 宛先・件名・本文は出さない(誤ってログに入れても漏れない)
    public var description: String {
        "ComposeDraft(recipients: \(recipients.count), subject: \(subject != nil), body: \(body != nil))"
    }

    public var debugDescription: String { description }
}

/// 下書きを開いた結果。送ったかどうかは OS の結果のまま返す。
public enum ComposeResult: String, Sendable, Equatable, CaseIterable {
    /// OS の画面から送った
    case sent
    /// 下書きとして保存した(メールだけ)
    case saved
    /// やめた(閉じた)
    case cancelled
    /// 出せなかった / 送れなかった
    case failed
    /// `sms:` / `mailto:` で外のアプリに渡した。送ったかどうかはこちらでは分からない
    case opened
    /// 下書きを出す手段がこの端末に無い(URL も開けなかった)
    case unavailable
}

// MARK: - URL の組み立て(純関数・単体テストの対象)

/// `sms:` / `mailto:` の URL を組み立てる。OS の API を一切使わないので Mac でもテストできる。
public enum ComposeMessageURL {
    /// RFC 3986 の unreserved(`A-Za-z0-9` と `-._~`)だけを残す。
    /// `.urlQueryAllowed` を使うと `&`・`=`・`?`・`+`・`#` が残ってしまい、本文に `&` が入っただけで
    /// クエリが割れて壊れる。件名・本文は「値」なので unreserved 以外は全部 % に逃がすのが正しい。
    /// 空白は `%20`(`mailto:` では `+` は空白ではなくただの `+` なので `+` にしてはいけない)。
    /// 改行は `%0A`(CRLF なら `%0D%0A`)、日本語は UTF-8 のパーセントエンコーディングになる。
    public static let unreserved: CharacterSet = {
        var set = CharacterSet(charactersIn: "A"..."Z")
        set.insert(charactersIn: "a"..."z")
        set.insert(charactersIn: "0"..."9")
        set.insert(charactersIn: "-._~")
        return set
    }()

    /// メールアドレス用。`@` は `mailto:` の宛先でそのまま置けるので残す(読みやすさのため)。
    /// `+`(Gmail のエイリアス)などは `%2B` に逃がす。宛先の区切りの `,` は各宛先を包んだ後に足す。
    public static let addressAllowed: CharacterSet = unreserved.union(CharacterSet(charactersIn: "@"))

    /// 電話番号用。国番号の `+` だけ残す(`sms:` の宛先では空白の意味にならない)。
    /// `#`・`*` はそのままだと断片・あいまいなので `%23`・`%2A` に逃がす。
    public static let phoneAllowed: CharacterSet = unreserved.union(CharacterSet(charactersIn: "+"))

    /// 1 つの値をパーセントエンコーディングする。失敗しない(`allowed` が非 ASCII を含まない限り)。
    public static func encode(_ text: String, allowed: CharacterSet) -> String {
        text.addingPercentEncoding(withAllowedCharacters: allowed) ?? ""
    }

    /// 前後の空白を落とし、空の宛先を捨てる。
    public static func normalizedRecipients(_ recipients: [String]) -> [String] {
        recipients.map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }.filter { !$0.isEmpty }
    }

    /// `mailto:a@x.com,b@y.com?subject=…&body=…`。宛先が無くても `mailto:` は作る(空の下書きが開く)。
    public static func mailto(_ draft: ComposeDraft) -> URL? {
        let to = normalizedRecipients(draft.recipients)
            .map { encode($0, allowed: addressAllowed) }
            .joined(separator: ",")
        var query: [String] = []
        if let subject = nonEmpty(draft.subject) { query.append("subject=" + encode(subject, allowed: unreserved)) }
        if let body = nonEmpty(draft.body) { query.append("body=" + encode(body, allowed: unreserved)) }
        let tail = query.isEmpty ? "" : "?" + query.joined(separator: "&")
        return URL(string: "mailto:" + to + tail)
    }

    /// `sms:09012345678,09087654321&body=…`。
    /// iOS の `sms:` は本文を `?body=` ではなく `&body=` で受ける(宛先の後ろに足す形)。件名は無いので捨てる。
    public static func sms(_ draft: ComposeDraft) -> URL? {
        let to = normalizedRecipients(draft.recipients)
            .map { encode($0, allowed: phoneAllowed) }
            .joined(separator: ",")
        let tail = nonEmpty(draft.body).map { "&body=" + encode($0, allowed: unreserved) } ?? ""
        return URL(string: "sms:" + to + tail)
    }

    public static func url(for channel: ComposeChannel, draft: ComposeDraft) -> URL? {
        switch channel {
        case .mail: mailto(draft)
        case .message: sms(draft)
        }
    }

    private static func nonEmpty(_ text: String?) -> String? {
        guard let text, !text.isEmpty else { return nil }
        return text
    }
}

// MARK: - 出す口

@MainActor
public protocol ComposePresenting {
    /// OS の下書き画面(`MFMailComposeViewController` / `MFMessageComposeViewController`)が使えるか。
    /// シミュレータ・メール未設定・Mac では false になり、`sms:` / `mailto:` に落ちる。
    func canCompose(_ channel: ComposeChannel) -> Bool
    /// OS の下書き画面を出して、閉じるまで待つ。
    func present(_ channel: ComposeChannel, draft: ComposeDraft) async -> ComposeResult
    /// `sms:` / `mailto:` を開く。開けたら true。
    func open(_ url: URL) async -> Bool
}

@MainActor
public enum ComposeMessage {
    /// 下書きを出す口の実体。テスト・プレビューでは差し替える。
    public static var presenter: any ComposePresenting = LiveComposePresenter()

    public static func open(_ channel: ComposeChannel, draft: ComposeDraft) async -> ComposeResult {
        await open(channel, draft: draft, using: presenter)
    }

    /// 出す口を差し替えられる形(単体テストはこちらを呼ぶ)。
    /// OS の下書き画面が使えるならそれ、使えなければ `sms:` / `mailto:` を開く。どちらも駄目なら `.unavailable`。
    public static func open(_ channel: ComposeChannel, draft: ComposeDraft,
                            using presenter: any ComposePresenting) async -> ComposeResult {
        if presenter.canCompose(channel) {
            return await presenter.present(channel, draft: draft)
        }
        guard let url = ComposeMessageURL.url(for: channel, draft: draft) else { return .unavailable }
        return await presenter.open(url) ? .opened : .unavailable
    }
}

// MARK: - 実体

/// iOS では MessageUI の下書き画面、Mac では `mailto:` / `sms:` を開くだけ。
@MainActor
public final class LiveComposePresenter: ComposePresenting {
    public init() {}

    public func canCompose(_ channel: ComposeChannel) -> Bool {
        #if canImport(MessageUI) && canImport(UIKit)
        switch channel {
        case .mail: MFMailComposeViewController.canSendMail()
        case .message: MFMessageComposeViewController.canSendText()
        }
        #else
        false
        #endif
    }

    public func present(_ channel: ComposeChannel, draft: ComposeDraft) async -> ComposeResult {
        #if canImport(MessageUI) && canImport(UIKit)
        guard let host = Self.topViewController() else { return .failed }
        let bridge = ComposeDelegateBridge()
        let controller: UIViewController
        switch channel {
        case .mail:
            let mail = MFMailComposeViewController()
            mail.mailComposeDelegate = bridge
            mail.setToRecipients(ComposeMessageURL.normalizedRecipients(draft.recipients))
            if let subject = draft.subject { mail.setSubject(subject) }
            if let body = draft.body { mail.setMessageBody(body, isHTML: false) }
            controller = mail
        case .message:
            let message = MFMessageComposeViewController()
            message.messageComposeDelegate = bridge
            message.recipients = ComposeMessageURL.normalizedRecipients(draft.recipients)
            if let subject = draft.subject, MFMessageComposeViewController.canSendSubject() { message.subject = subject }
            if let body = draft.body { message.body = body }
            controller = message
        }
        return await withCheckedContinuation { continuation in
            bridge.finish = { [weak controller] result in
                controller?.dismiss(animated: true)
                continuation.resume(returning: result)
            }
            host.present(controller, animated: true)
        }
        #else
        return .failed
        #endif
    }

    public func open(_ url: URL) async -> Bool {
        #if canImport(UIKit)
        return await UIApplication.shared.open(url)
        #elseif canImport(AppKit)
        return NSWorkspace.shared.open(url)
        #else
        return false
        #endif
    }

    #if canImport(UIKit)
    /// 今いちばん上の画面(そこに下書きを重ねる)。
    static func topViewController() -> UIViewController? {
        let window = UIApplication.shared.connectedScenes
            .compactMap { ($0 as? UIWindowScene)?.keyWindow }
            .first
        var top = window?.rootViewController
        while let next = top?.presentedViewController { top = next }
        return top
    }
    #endif
}

#if canImport(MessageUI) && canImport(UIKit)
/// MessageUI の 2 つのデリゲートを 1 つの `ComposeResult` にまとめる。閉じるまで自分で自分を持つ。
@MainActor
private final class ComposeDelegateBridge: NSObject, @preconcurrency MFMailComposeViewControllerDelegate,
                                            @preconcurrency MFMessageComposeViewControllerDelegate {
    var finish: ((ComposeResult) -> Void)?
    private var retain: ComposeDelegateBridge?

    override init() {
        super.init()
        retain = self
    }

    private func done(_ result: ComposeResult) {
        let finish = self.finish
        self.finish = nil
        retain = nil
        finish?(result)
    }

    func mailComposeController(_ controller: MFMailComposeViewController,
                               didFinishWith result: MFMailComposeResult, error: Error?) {
        if error != nil { return done(.failed) }
        switch result {
        case .sent: done(.sent)
        case .saved: done(.saved)
        case .cancelled: done(.cancelled)
        case .failed: done(.failed)
        @unknown default: done(.failed)
        }
    }

    func messageComposeViewController(_ controller: MFMessageComposeViewController,
                                      didFinishWith result: MessageComposeResult) {
        switch result {
        case .sent: done(.sent)
        case .cancelled: done(.cancelled)
        case .failed: done(.failed)
        @unknown default: done(.failed)
        }
    }
}
#endif

// MARK: - 偽物

/// テスト・プレビュー用。使えるかどうかと結果を外から決められる。
@MainActor
public final class FakeComposePresenter: ComposePresenting {
    /// `canCompose` が true を返すチャンネル
    public var composable: Set<ComposeChannel>
    /// 下書き画面を出した時に返す結果
    public var presentResult: ComposeResult
    /// `open(_:)` が成功するか(false ならアプリが受けられなかった)
    public var canOpen: Bool
    /// 下書き画面に渡された中身(新しいものが後ろ)
    public private(set) var presented: [(channel: ComposeChannel, draft: ComposeDraft)] = []
    /// 開いた URL(新しいものが後ろ)
    public private(set) var opened: [URL] = []

    public init(composable: Set<ComposeChannel> = Set(ComposeChannel.allCases),
                presentResult: ComposeResult = .sent,
                canOpen: Bool = true) {
        self.composable = composable
        self.presentResult = presentResult
        self.canOpen = canOpen
    }

    public func canCompose(_ channel: ComposeChannel) -> Bool { composable.contains(channel) }

    public func present(_ channel: ComposeChannel, draft: ComposeDraft) async -> ComposeResult {
        presented.append((channel, draft))
        return presentResult
    }

    public func open(_ url: URL) async -> Bool {
        opened.append(url)
        return canOpen
    }
}
