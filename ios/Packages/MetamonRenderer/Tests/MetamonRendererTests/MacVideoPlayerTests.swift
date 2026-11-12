import XCTest
import AVKit
@testable import MetamonRenderer
#if os(macOS)
import AppKit
#endif

/// #1340: 動画の再生を Mac に合わせる(全画面のボタン・しまった時に止める・「この端末にはありません」の印)。
@MainActor
final class MacVideoPlayerTests: XCTestCase {
    func testNotOnDeviceSymbolMatchesPlatform() {
        #if os(macOS)
        XCTAssertEqual(PlatformSystem.notOnDeviceSymbol, "desktopcomputer.trianglebadge.exclamationmark")
        #else
        XCTAssertEqual(PlatformSystem.notOnDeviceSymbol, "iphone.slash")
        #endif
    }

    #if os(macOS)
    func testPlayerViewShowsFullScreenButton() {
        let view = MacVideoPlayerView.PausingPlayerView()
        MacVideoPlayerView.configure(view)
        XCTAssertTrue(view.showsFullScreenToggleButton)
        XCTAssertEqual(view.controlsStyle, .floating)
        XCTAssertTrue(view.acceptsFirstResponder) // スペースキーで再生 / 一時停止
    }

    func testMiniaturizeAndHidePause() {
        let window = NSWindow(contentRect: CGRect(x: 0, y: 0, width: 200, height: 120), styleMask: [.titled], backing: .buffered, defer: true)
        let view = MacVideoPlayerView.PausingPlayerView()
        let player = AVPlayer()
        view.player = player
        window.contentView = view

        player.rate = 1
        NotificationCenter.default.post(name: NSWindow.didMiniaturizeNotification, object: window)
        XCTAssertEqual(player.rate, 0, "ウインドウをしまったら止める")

        player.rate = 1
        NotificationCenter.default.post(name: NSApplication.didHideNotification, object: NSApp)
        XCTAssertEqual(player.rate, 0, "アプリを隠したら止める")

        player.rate = 1
        let other = NSWindow(contentRect: .zero, styleMask: [.titled], backing: .buffered, defer: true)
        NotificationCenter.default.post(name: NSWindow.didMiniaturizeNotification, object: other)
        XCTAssertEqual(player.rate, 1, "別のウインドウでは止めない")
        player.rate = 0
        view.stopObserving()
    }
    #endif
}
