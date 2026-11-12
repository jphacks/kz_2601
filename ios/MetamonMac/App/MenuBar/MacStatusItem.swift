import AppKit
import SwiftUI

/// メニューバーのアイコン(#1848)。SwiftUI の `MenuBarExtra` の scene を足すと、起動時に FocusedValue の更新が
/// 止まらず固まる(メニューの `MacCommands` と取り合う・単体テストのホストが接続前に止まった)ので、AppKit の
/// `NSStatusItem` + `NSPopover` で window の形と同じ窓を出す。#985 のタイマーもこのアイコンにまとめる
@MainActor
final class MacStatusItem: NSObject, NSPopoverDelegate {
    static let shared = MacStatusItem()

    private var item: NSStatusItem?
    private let popover = NSPopover()
    /// 本体のウインドウを作る(ウインドウが 1 枚も無い時)。最初のウインドウが出た時に受け取る
    private var openWindow: OpenWindowAction?

    var isShown: Bool { item != nil }

    override init() {
        super.init()
        popover.behavior = .transient
        popover.animates = false
        popover.delegate = self
    }

    /// 起動時(最初のウインドウ)に呼ぶ。設定どおりに出す
    func install(openWindow: OpenWindowAction) {
        self.openWindow = openWindow
        apply(enabled: MacMenuBarSettings().isEnabled)
    }

    /// 設定の「メニューバーに表示する」
    func apply(enabled: Bool) {
        if enabled {
            guard item == nil else { return }
            let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
            item.button?.image = NSImage(systemSymbolName: MacMenuBarIcon.systemImage, accessibilityDescription: "Tailor It")
            item.button?.target = self
            item.button?.action = #selector(toggle(_:))
            self.item = item
        } else {
            popover.performClose(nil)
            if let item { NSStatusBar.system.removeStatusItem(item) }
            item = nil
        }
    }

    @objc private func toggle(_ sender: NSStatusBarButton) {
        if popover.isShown {
            popover.performClose(sender)
            return
        }
        // 開くたびに作り直し、一覧と stat を読み直す
        popover.contentViewController = NSHostingController(rootView: MacLanguageRoot {
            MacMenuBarView(bringMainWindow: { [weak self] in self?.bringMainWindow() })
        })
        popover.show(relativeTo: sender.bounds, of: sender, preferredEdge: .minY)
        popover.contentViewController?.view.window?.makeKey()
    }

    nonisolated func popoverDidClose(_ notification: Notification) {
        MainActor.assumeIsolated { popover.contentViewController = nil }
    }

    private func bringMainWindow() {
        popover.performClose(nil)
        NSApp.activate(ignoringOtherApps: true)
        let window = NSApp.orderedWindows.first { $0.identifier?.rawValue.hasPrefix(MacCommands.mainWindowId) == true }
        if let window {
            if window.isMiniaturized { window.deminiaturize(nil) }
            window.makeKeyAndOrderFront(nil)
        } else {
            openWindow?(id: MacCommands.mainWindowId)
        }
    }
}

/// 最初のウインドウが出た時にアイコンを設定どおりに出し、ウインドウを作る口を渡す
struct MacStatusItemInstaller: ViewModifier {
    @Environment(\.openWindow) private var openWindow

    func body(content: Content) -> some View {
        content.task { MacStatusItem.shared.install(openWindow: openWindow) }
    }
}
