#if os(macOS)
import SwiftUI
import AppKit
import AVKit

/// Mac の動画の再生(#1340)。SwiftUI の `VideoPlayer` は Mac では全画面のボタンが出ないので、`AVPlayerView` を使う。
/// - 全画面のボタンを出す(`showsFullScreenToggleButton`)
/// - スペースキーで再生 / 一時停止(クリックかキーボードでフォーカスが来た時。`AVPlayerView` の標準の操作)
/// - ウインドウをしまった・アプリを隠した時に止める(iOS は画面から外れた時の `onDisappear` だけ)
struct MacVideoPlayerView: NSViewRepresentable {
    let player: AVPlayer?

    func makeNSView(context: Context) -> PausingPlayerView {
        let view = PausingPlayerView()
        Self.configure(view)
        view.player = player
        return view
    }

    /// 操作部(浮かぶ操作部・全画面のボタン)
    static func configure(_ view: AVPlayerView) {
        view.controlsStyle = .floating
        view.showsFullScreenToggleButton = true
        view.allowsPictureInPicturePlayback = false
    }

    func updateNSView(_ view: PausingPlayerView, context: Context) {
        if view.player !== player { view.player = player }
    }

    static func dismantleNSView(_ view: PausingPlayerView, coordinator: ()) {
        view.player?.pause()
        view.stopObserving()
    }

    /// ウインドウがしまわれた・アプリが隠れた時に止める `AVPlayerView`
    final class PausingPlayerView: AVPlayerView {
        private var observers: [NSObjectProtocol] = []

        override var acceptsFirstResponder: Bool { true }

        override func viewDidMoveToWindow() {
            super.viewDidMoveToWindow()
            stopObserving()
            guard let window else { return }
            let center = NotificationCenter.default
            // ウインドウをしまう・アプリを隠す
            let events: [(Notification.Name, AnyObject?)] = [(NSWindow.didMiniaturizeNotification, window),
                                                              (NSApplication.didHideNotification, nil)]
            for (name, object) in events {
                observers.append(center.addObserver(forName: name, object: object, queue: .main) { [weak self] _ in
                    MainActor.assumeIsolated { self?.player?.pause() }
                })
            }
        }

        func stopObserving() {
            observers.forEach(NotificationCenter.default.removeObserver)
            observers = []
        }
    }
}
#endif
