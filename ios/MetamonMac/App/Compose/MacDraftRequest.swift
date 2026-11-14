import AppKit
import Observation

/// ほかのシート・ウインドウ(β の案内・#1214)から「この文を入力欄に入れて会話を開く」を頼む口。送らない。
/// 受け取るのはアプリのウインドウ(`MacRootView`)。頼んだ時に前に出したウインドウだけが開く(`take(in:)` で空にする)。
/// ウインドウが何枚あっても、裏のウインドウに会話が開かないようにする
@MainActor
@Observable
final class MacDraftRequest {
    static let shared = MacDraftRequest()

    /// 頼まれた回数。受け取る側は `onChange` でこれを見る(同じ文を 2 回頼んでも変わる)
    private(set) var serial = 0
    @ObservationIgnored private var draft: String?
    /// 受け取るウインドウ(頼んだ時に前に出したもの)。nil ならどのウインドウでも受け取れる
    @ObservationIgnored private weak var target: NSWindow?
    @ObservationIgnored private var hasTarget = false

    func send(_ text: String) {
        // 設定のウインドウから頼んだ時も会話が見えるよう、一番前のアプリのウインドウを前に出し、そこで受け取る
        let window = NSApp?.orderedWindows.first { $0.isVisible && $0.identifier?.rawValue.hasPrefix(MacCommands.mainWindowId) == true }
        window?.makeKeyAndOrderFront(nil)
        send(text, to: window)
    }

    /// `window` で受け取るよう頼む(ほかのウインドウは受け取らない)。nil なら最初に受け取ったウインドウが開く
    func send(_ text: String, to window: NSWindow?) {
        draft = text
        target = window
        hasTarget = window != nil
        serial += 1
    }

    /// `window` 宛て(または宛先なし)でまだ誰も受け取っていなければ文を返し、空にする。
    /// 宛先のウインドウが閉じていたら、キーのウインドウが受け取る
    func take(in window: NSWindow?) -> String? {
        guard draft != nil else { return nil }
        if hasTarget {
            if let target {
                guard target === window else { return nil }
            } else if window?.isKeyWindow != true {
                return nil
            }
        }
        defer { draft = nil; target = nil; hasTarget = false }
        return draft
    }
}
