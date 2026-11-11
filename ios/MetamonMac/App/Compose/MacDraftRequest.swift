import AppKit
import Observation

/// ほかのシート・ウインドウ(β の案内・#1214)から「この文を入力欄に入れて会話を開く」を頼む口。送らない。
/// 受け取るのはアプリのウインドウ(`MacRootView`)。最初に受け取った 1 枚だけが開く(`take()` で空にする)。
@MainActor
@Observable
final class MacDraftRequest {
    static let shared = MacDraftRequest()

    /// 頼まれた回数。受け取る側は `onChange` でこれを見る(同じ文を 2 回頼んでも変わる)
    private(set) var serial = 0
    @ObservationIgnored private var draft: String?

    func send(_ text: String) {
        draft = text
        serial += 1
        // 設定のウインドウから頼んだ時も会話が見えるよう、一番前のアプリのウインドウを前に出す
        NSApp?.orderedWindows.first { $0.isVisible && $0.identifier?.rawValue.hasPrefix(MacCommands.mainWindowId) == true }?
            .makeKeyAndOrderFront(nil)
    }

    /// まだ誰も受け取っていなければ文を返し、空にする
    func take() -> String? {
        defer { draft = nil }
        return draft
    }
}
