import AppKit

/// サービスメニューの「Tailor It に送る」(#1849)。ほかのアプリで選んだ文字を受け取り、Tailor It を前に出して
/// ⌘K と同じ会話の入力欄に入れる(送らない)。中央にアプリがあればそのアプリを直す会話、無ければ作る会話。
/// - メニューの名前と受け取る種類は Info.plist の `NSServices`(英語は ServicesMenu.xcstrings)
/// - 入力欄に入れる口は `MacDraftRequest`(β の案内と同じ)。会話が開いていればその入力欄に足す(`MacRootView`)
/// - アプリのウインドウが無ければ開き、開いたウインドウが受け取る
@MainActor
final class MacTextServiceProvider: NSObject {
    static let shared = MacTextServiceProvider()

    /// 受け取った文字の行き先。nil なら `open(_:)`(テストは差し替える)
    private let route: ((String) -> Void)?
    /// アプリのウインドウが無い時に開く(ウインドウの根が `openWindow` を入れる)
    var openMainWindow: (() -> Void)?

    init(route: ((String) -> Void)? = nil) {
        self.route = route
    }

    /// 起動時に 1 回(`MacAppDelegate`)
    func register() {
        NSApp.servicesProvider = self
        NSUpdateDynamicServices()
    }

    /// Info.plist の `NSMessage`(sendToTailorIt)
    @objc(sendToTailorIt:userData:error:)
    func sendToTailorIt(_ pboard: NSPasteboard, userData: String?, error: AutoreleasingUnsafeMutablePointer<NSString?>) {
        guard let text = Self.text(from: pboard) else {
            error.pointee = String(localized: "送る文字がありません。文字を選んでからもう一度お試しください。") as NSString
            return
        }
        if let route { route(text) } else { open(text) }
    }

    /// 貼り付け板の文字(前後の空白を除く)。無い・空なら nil
    static func text(from pboard: NSPasteboard) -> String? {
        guard let raw = pboard.string(forType: .string) else { return nil }
        let text = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        return text.isEmpty ? nil : text
    }

    /// 会話の入力欄に書きかけがあれば、消さずに次の行へ足す
    static func appending(_ text: String, to draft: String) -> String {
        let current = draft.trimmingCharacters(in: .whitespacesAndNewlines)
        return current.isEmpty ? text : current + "\n" + text
    }

    private func open(_ text: String) {
        NSApp.activate()
        showMainWindow()
        MacDraftRequest.shared.send(text)
    }

    /// 見えるアプリのウインドウが無ければ、しまったものを戻す・それも無ければ新しく開く
    private func showMainWindow() {
        let windows = NSApp.windows.filter { $0.identifier?.rawValue.hasPrefix(MacCommands.mainWindowId) == true }
        guard !windows.contains(where: \.isVisible) else { return }
        if let hidden = windows.first(where: \.isMiniaturized) {
            hidden.deminiaturize(nil)
        } else {
            openMainWindow?()
        }
    }
}
