import SwiftUI
#if canImport(UIKit)
import UIKit
#else
import AppKit
#endif
@testable import MetamonRenderer

// テストで SwiftUI の View を載せる所の iOS / Mac の差(#1041)。iOS は UIHostingController / UIWindow のまま。

#if canImport(UIKit)
typealias TestHostingController<V: View> = UIHostingController<V>
#else
typealias TestHostingController<V: View> = NSHostingController<V>
#endif

extension TestHostingController {
    /// 今すぐレイアウトする。
    func layoutNow() {
        #if canImport(UIKit)
        view.setNeedsLayout()
        view.layoutIfNeeded()
        #else
        view.needsLayout = true
        view.layoutSubtreeIfNeeded()
        #endif
    }
}

/// View を画面の窓に載せる(onAppear / onDisappear を走らせる)。
@MainActor
final class TestWindow {
    #if canImport(UIKit)
    let window: UIWindow
    #else
    let window: NSWindow
    #endif

    init(size: CGSize) {
        #if canImport(UIKit)
        window = UIWindow(frame: CGRect(origin: .zero, size: size))
        #else
        window = NSWindow(contentRect: CGRect(origin: .zero, size: size), styleMask: [.titled],
                          backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false
        #endif
    }

    /// 載せて表示する。
    func show<V: View>(_ host: TestHostingController<V>) {
        #if canImport(UIKit)
        window.rootViewController = host
        window.makeKeyAndVisible()
        #else
        window.contentViewController = host
        window.makeKeyAndOrderFront(nil)
        #endif
        host.layoutNow()
    }

    /// 外して隠す(画面を離れた時と同じ)。
    func remove() {
        #if canImport(UIKit)
        window.rootViewController = nil
        window.isHidden = true
        #else
        window.contentViewController = nil
        window.orderOut(nil)
        #endif
    }

    /// 隠すだけ。
    func hide() {
        #if canImport(UIKit)
        window.isHidden = true
        #else
        window.orderOut(nil)
        #endif
    }
}

/// テスト用の画像(原点は左上)。`PlatformGraphics.image` に寄せる。
func testImage(width: CGFloat, height: CGFloat, scale: CGFloat = 1, opaque: Bool = false,
               draw: (CGContext) -> Void) -> PlatformImage {
    PlatformGraphics.image(size: CGSize(width: width, height: height), scale: scale, opaque: opaque, draw: draw)
}
