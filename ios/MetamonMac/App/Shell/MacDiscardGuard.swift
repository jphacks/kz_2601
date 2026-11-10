import SwiftUI
import AppKit
import ObjectiveC

/// ウインドウ 1 つぶんの「保存していない入力」の見張り(#943)。中央のアプリを丸ごと捨てる操作
/// (サイドバー・⌥⌘↑↓ でのアプリの切り替え、⌘W・閉じるボタンでウインドウを閉じる)の前に確認する。
/// 確認は中央のアプリの確認ダイアログと同じ場所に出す(`MacAppRuntime.discardRequest`)。
@MainActor
final class MacDiscardGuard {
    /// 開いているウインドウの見張り(⌘Q の前に全部を見る・`MacAppDelegate`)
    private static let all = NSHashTable<MacDiscardGuard>.weakObjects()

    /// 中央で開いているアプリのランタイム(`AppCanvas` が作ったら入れる)
    weak var runtime: MacAppRuntime?
    /// 見張っているウインドウ(`WindowCloseGuard` が入れる)。⌘Q の確認を出す前に前へ出す
    weak var window: NSWindow?
    /// 会話のシートを開く前に「破棄」を選んだ(または入力が無かった)。シートを閉じるまでは聞き直さない
    var released = false

    init() { Self.all.add(self) }

    /// 保存していない入力を失うか(`released` の間は失ってよいと答えた後なので false)
    var hasUnsavedInput: Bool { !released && (runtime?.hasUnsavedInput() ?? false) }

    /// 入力を失わなければすぐ `action`。失うなら確認を出し、「破棄」で `action`。
    func run(_ action: @escaping () -> Void) {
        guard let runtime, !released else { return action() }
        runtime.confirmDiscard(then: action)
    }

    /// ウインドウを閉じてよいか。入力があれば確認を出して false(「破棄」で `close` を呼ぶ)。
    func allowsClose(_ close: @escaping () -> Void) -> Bool {
        guard hasUnsavedInput, let runtime else { return true }
        runtime.discardRequest = DiscardRequest(perform: close)
        return false
    }

    /// ⌘Q: 入力のあるウインドウ(前にあるものを優先)で確認を出す。出したら true(「破棄」で `quit`)。
    static func confirmQuit(_ quit: @escaping () -> Void) -> Bool {
        let guards = all.allObjects.filter(\.hasUnsavedInput)
        guard let target = guards.first(where: { $0.window?.isKeyWindow == true }) ?? guards.first,
              let runtime = target.runtime else { return false }
        target.window?.makeKeyAndOrderFront(nil)
        runtime.discardRequest = DiscardRequest(perform: quit)
        return true
    }
}

/// ⌘Q の前に保存していない入力を確かめる(#943)。入力があれば終了をいったん止めて確認を出し、
/// 「破棄」でもう一度終了する(`.terminateLater` で待つと確認の表示より先に終了の待ちに入るため、止めて出し直す)。
/// 複数のウインドウに入力があっても確認は 1 回(「破棄」で全部を捨てる)。
@MainActor
final class MacAppDelegate: NSObject, NSApplicationDelegate {
    private var quitConfirmed = false

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        if quitConfirmed { return .terminateNow }
        let asked = MacDiscardGuard.confirmQuit { [weak self] in
            self?.quitConfirmed = true
            NSApp.terminate(nil)
            self?.quitConfirmed = false // ここに戻った = 終了がほかの理由で取りやめられた
        }
        return asked ? .terminateCancel : .terminateNow
    }
}

/// ⌘W・閉じるボタンの前に `MacDiscardGuard` に聞く。SwiftUI のウインドウの delegate はそのまま残し、
/// `windowShouldClose` だけ前に挟む(他の呼び出しは元の delegate へ渡す)。
struct WindowCloseGuard: NSViewRepresentable {
    let discardGuard: MacDiscardGuard

    func makeNSView(context: Context) -> NSView {
        let view = NSView(frame: .zero)
        DispatchQueue.main.async { [weak view] in install(on: view?.window) }
        return view
    }

    func updateNSView(_ view: NSView, context: Context) {
        install(on: view.window) // SwiftUI が delegate を差し替えていたら挟み直す
    }

    private func install(on window: NSWindow?) {
        guard let window else { return }
        discardGuard.window = window
        if let proxy = window.delegate as? CloseGuardDelegate {
            proxy.discardGuard = discardGuard
            return
        }
        let proxy = CloseGuardDelegate(original: window.delegate, discardGuard: discardGuard)
        objc_setAssociatedObject(window, &CloseGuardDelegate.key, proxy, .OBJC_ASSOCIATION_RETAIN_NONATOMIC) // delegate は弱参照
        window.delegate = proxy
    }
}

@MainActor
private final class CloseGuardDelegate: NSObject, NSWindowDelegate {
    static var key: UInt8 = 0

    weak var original: NSWindowDelegate?
    weak var discardGuard: MacDiscardGuard?
    /// 「破棄」の後の閉じ直し。もう聞かない
    private var confirmed = false

    init(original: NSWindowDelegate?, discardGuard: MacDiscardGuard) {
        self.original = original
        self.discardGuard = discardGuard
    }

    func windowShouldClose(_ sender: NSWindow) -> Bool {
        if !confirmed, let discardGuard,
           !discardGuard.allowsClose({ [weak self, weak sender] in
               self?.confirmed = true
               sender?.performClose(nil)
           }) {
            return false
        }
        confirmed = false
        return original?.windowShouldClose?(sender) ?? true
    }

    override func responds(to aSelector: Selector!) -> Bool {
        super.responds(to: aSelector) || (original?.responds(to: aSelector) ?? false)
    }

    override func forwardingTarget(for aSelector: Selector!) -> Any? {
        (original?.responds(to: aSelector) ?? false) ? original : super.forwardingTarget(for: aSelector)
    }
}
