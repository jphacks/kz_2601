import SwiftUI
import AppKit

/// 生成アプリごとのウインドウ(#1853)の値。`WindowGroup(for: AppID.self)` で開く。
/// SwiftUI は同じ値のウインドウが開いていれば、新しく開かずにそれを前に出す
struct AppID: Hashable, Codable, Sendable {
    let rawValue: String
}

/// 生成アプリを別のウインドウで開く(#1853)。サイドバーの右クリック「新しいウインドウで開く」と ⌥クリック。
/// 中身はメインと同じ `MacRootView`(選択・お知らせ・新着の数はウインドウごと)で、サイドバーを畳んで開く。
/// 記録は同じアプリのウインドウ同士でエンジンを共有し(`MacRecordEngines`)、端末に書いたら互いに読み直す。
enum MacAppWindows {
    static let windowId = "app"

    /// ウインドウの大きさと位置をアプリごとに覚える名前
    static func frameAutosaveName(_ app: AppID) -> String { "app-window-\(app.rawValue)" }

    /// ウインドウを開いた時に選ぶアプリ。前に選んでいた(SceneStorage)アプリがまだあればそれ、無ければウインドウの値
    static func initialSelection(window: AppID?, restored: String?, appIds: [String]) -> String? {
        if let restored, appIds.contains(restored) { return restored }
        if let window, appIds.contains(window.rawValue) { return window.rawValue }
        return nil
    }

    /// サイドバーの行を ⌥クリックした(選ばずに別のウインドウで開く)。キーの ⌥↑↓ は今までどおり選ぶ
    static func opensNewWindow(eventType: NSEvent.EventType?, modifiers: NSEvent.ModifierFlags) -> Bool {
        guard let eventType, [.leftMouseDown, .leftMouseUp].contains(eventType) else { return false }
        return modifiers.intersection(.deviceIndependentFlagsMask) == .option
    }

    static func opensNewWindow(_ event: NSEvent?) -> Bool {
        opensNewWindow(eventType: event?.type, modifiers: event?.modifierFlags ?? [])
    }
}

/// ウインドウの大きさと位置をアプリごとに覚える(#1853)
struct WindowFrameAutosave: NSViewRepresentable {
    let name: String

    func makeNSView(context: Context) -> NSView {
        let view = NSView(frame: .zero)
        DispatchQueue.main.async { [weak view] in apply(to: view?.window) }
        return view
    }

    func updateNSView(_ view: NSView, context: Context) { apply(to: view.window) }

    private func apply(to window: NSWindow?) {
        guard let window, window.frameAutosaveName != name else { return }
        window.setFrameUsingName(name) // 前に覚えた大きさ・位置があれば戻す
        window.setFrameAutosaveName(name)
    }
}
