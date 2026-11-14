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
    /// 設定の「メニューバーに表示する」
    private var settingEnabled = false
    /// メニューバーの表示(#2254・試作のフラグが on の時だけ来る)の経過時間を 1 秒ごとに出し直す
    private var ticker: Timer?

    override init() {
        super.init()
        popover.behavior = .transient
        popover.animates = false
        popover.delegate = self
        MacMenuBarActivity.shared.onChange = { [weak self] in self?.refreshActivity() }
    }

    /// 起動時(最初のウインドウ)に呼ぶ。設定どおりに出す
    func install(openWindow: OpenWindowAction) {
        self.openWindow = openWindow
        apply(enabled: MacMenuBarSettings().isEnabled)
    }

    /// 設定の「メニューバーに表示する」。表示(#2254)が出ている間は設定が off でもアイコンを残す
    func apply(enabled: Bool) {
        settingEnabled = enabled
        updateItem()
    }

    private func updateItem() {
        if settingEnabled || MacMenuBarActivity.shared.current != nil {
            guard item == nil else { return }
            let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
            item.button?.image = NSImage(systemSymbolName: MacMenuBarIcon.systemImage, accessibilityDescription: "Tailor It")
            item.button?.target = self
            item.button?.action = #selector(toggle(_:))
            self.item = item
            refreshTitle()
        } else {
            popover.performClose(nil)
            if let item { NSStatusBar.system.removeStatusItem(item) }
            item = nil
        }
    }

    /// 表示(#2254)が出た・変わった・消えた
    private func refreshActivity() {
        updateItem()
        ticker?.invalidate()
        ticker = nil
        refreshTitle()
        guard MacMenuBarActivity.shared.current != nil else { return }
        let timer = Timer(timeInterval: 1, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.refreshTitle() }
        }
        RunLoop.main.add(timer, forMode: .common)
        ticker = timer
    }

    /// アイコンの横の経過時間(表示が無ければ消す)
    private func refreshTitle() {
        guard let button = item?.button else { return }
        if let activity = MacMenuBarActivity.shared.current {
            button.imagePosition = .imageLeading
            button.image = NSImage(systemSymbolName: activity.icon, accessibilityDescription: nil)
                ?? NSImage(systemSymbolName: "timer", accessibilityDescription: nil)
            button.title = MacMenuBarActivity.statusTitle(activity, now: .now)
            button.font = .monospacedDigitSystemFont(ofSize: NSFont.systemFontSize, weight: .regular)
            button.setAccessibilityLabel("\(activity.title), \(button.title)")
            item?.length = NSStatusItem.variableLength
        } else {
            button.title = ""
            button.imagePosition = .imageOnly
            button.image = NSImage(systemSymbolName: MacMenuBarIcon.systemImage, accessibilityDescription: "Tailor It")
            button.setAccessibilityLabel(nil)
            item?.length = NSStatusItem.squareLength
        }
    }

    @objc private func toggle(_ sender: NSStatusBarButton) {
        if popover.isShown {
            popover.performClose(sender)
            return
        }
        // 開くたびに作り直し、一覧と stat を読み直す
        popover.contentViewController = NSHostingController(rootView: MacLanguageRoot {
            MacMenuBarView(bringMainWindow: { [weak self] in self?.bringMainWindow() },
                           closePopover: { [weak self] in self?.popover.performClose(nil) })
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
