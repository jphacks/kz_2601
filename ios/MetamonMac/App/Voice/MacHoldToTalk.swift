import AppKit
import SwiftUI

/// 押している間だけ録音(#1220)。押して録音・もう一度押して止める(#966)はそのまま残し、長く押した時だけ「離したら止めて送る」にする。
/// - `.immediate`(マイクのボタン・⇧⌘M): 押した瞬間に録音を始める。`threshold` 以上押してから離すと止めて送る。短く押した時は録音を続ける(今の動き)。
/// - `.delayed`(入力欄が空の時のスペース): `threshold` 押し続けた時だけ録音を始め、離すと止めて送る。短く押した時は普通のスペース。
/// 録音中に押した時は、どちらも止めて送る(もう一度押して止める)。
struct HoldToTalkGesture: Equatable {
    enum Mode: Equatable { case immediate, delayed }
    enum Action: Equatable {
        case none
        case start
        case stop
        /// 短く押したスペースは文字として入れる
        case insertSpace
    }

    /// 短く押す / 長く押すを分ける時間
    static let threshold: TimeInterval = 0.4

    private(set) var mode: Mode?
    private var pressedAt: Date?
    private var started = false
    private var stoppedOnPress = false

    var isPressed: Bool { pressedAt != nil }

    mutating func press(_ mode: Mode, at time: Date, isRecording: Bool) -> Action {
        guard pressedAt == nil else { return .none } // キーの繰り返し・二重の押下
        self.mode = mode
        pressedAt = time
        started = false
        stoppedOnPress = false
        if isRecording {
            stoppedOnPress = true
            return .stop
        }
        switch mode {
        case .immediate:
            started = true
            return .start
        case .delayed:
            return .none
        }
    }

    /// 押している間に時間が経った(`.delayed` はここで録音を始める)
    mutating func holdElapsed(at time: Date) -> Action {
        guard mode == .delayed, let pressedAt, !started, !stoppedOnPress,
              time.timeIntervalSince(pressedAt) >= Self.threshold else { return .none }
        started = true
        return .start
    }

    mutating func release(at time: Date) -> Action {
        guard let pressedAt, let mode else { return .none }
        defer { reset() }
        if stoppedOnPress { return .none }
        switch mode {
        case .immediate:
            return time.timeIntervalSince(pressedAt) >= Self.threshold ? .stop : .none
        case .delayed:
            if started { return .stop }
            // 長く押したが録音は始まっていない(時計が間に合わなかった): 何もしない
            return time.timeIntervalSince(pressedAt) < Self.threshold ? .insertSpace : .none
        }
    }

    /// 押したまま別の操作で終わった(窓が切り替わった等)。録音を始めていれば止める
    mutating func interrupt() -> Action {
        defer { reset() }
        return started && pressedAt != nil ? .stop : .none
    }

    private mutating func reset() {
        mode = nil
        pressedAt = nil
        started = false
        stoppedOnPress = false
    }
}

/// 押している間だけ録音するキー(#1220)。入力欄が空の時のスペースと ⇧⌘M。
enum HoldToTalkKey {
    static let spaceKeyCode: UInt16 = 49

    /// このキーの押下を声で頼むに使うか。入力欄に文字がある・変換中・別の修飾キー付きの時は使わない(普通に入力する)
    static func mode(keyCode: UInt16, characters: String?, modifiers: NSEvent.ModifierFlags,
                     draftIsEmpty: Bool, fieldHasText: Bool, isComposing: Bool) -> HoldToTalkGesture.Mode? {
        let mods = modifiers.intersection([.command, .shift, .option, .control])
        if keyCode == spaceKeyCode {
            guard mods.isEmpty, draftIsEmpty, !fieldHasText, !isComposing else { return nil }
            return .delayed
        }
        if characters?.lowercased() == "m", mods == [.command, .shift] {
            return .immediate
        }
        return nil
    }
}

/// 会話のシートのキーを見る。シートの窓が手前の時だけ、スペース(入力欄にいて空の時)と ⇧⌘M を押している間だけ録音にする。
struct MacHoldToTalkKeys: ViewModifier {
    @Environment(MacVoiceInput.self) private var voice
    let draftIsEmpty: Bool
    let enabled: Bool
    let deliver: (String) -> Void
    let insertSpace: () -> Void

    @State private var monitor = Monitor()

    func body(content: Content) -> some View {
        // 表示のたびに最新の入力欄の状態と送り先を渡す(モニターは窓に 1 つ)
        let _ = monitor.configure(voice: voice, draftIsEmpty: draftIsEmpty, enabled: enabled,
                                  deliver: deliver, insertSpace: insertSpace)
        content
            .background(WindowReader { monitor.window = $0 })
            .onAppear { monitor.install() }
            .onDisappear { monitor.remove() }
    }

    @MainActor
    final class Monitor {
        weak var window: NSWindow?
        private weak var voice: MacVoiceInput?
        private var draftIsEmpty = true
        private var enabled = true
        private var deliver: (String) -> Void = { _ in }
        private var insertSpace: () -> Void = {}
        private(set) var gesture = HoldToTalkGesture()
        private var token: Any?
        /// 窓・アプリが手前でなくなった時(離したキーの keyUp が届かないので、押したままにしない)
        private var resignTokens: [NSObjectProtocol] = []
        private var timer: Timer?

        func configure(voice: MacVoiceInput, draftIsEmpty: Bool, enabled: Bool,
                       deliver: @escaping (String) -> Void, insertSpace: @escaping () -> Void) {
            self.voice = voice
            self.draftIsEmpty = draftIsEmpty
            self.enabled = enabled
            self.deliver = deliver
            self.insertSpace = insertSpace
        }

        private func handle(_ action: HoldToTalkGesture.Action) {
            switch action {
            case .none: break
            case .start: voice?.holdStart(deliver: deliver)
            case .stop: voice?.holdStop(deliver: deliver)
            case .insertSpace: insertSpace()
            }
        }

        func install() {
            guard token == nil else { return }
            token = NSEvent.addLocalMonitorForEvents(matching: [.keyDown, .keyUp, .flagsChanged]) { [weak self] event in
                let consume = MainActor.assumeIsolated { self?.consumes(event) ?? false }
                return consume ? nil : event
            }
            let center = NotificationCenter.default
            resignTokens = [
                center.addObserver(forName: NSWindow.didResignKeyNotification, object: nil, queue: .main) { [weak self] note in
                    MainActor.assumeIsolated {
                        guard let self, let window = self.window, note.object as? NSWindow === window else { return }
                        self.interrupt(discard: true)
                    }
                },
                center.addObserver(forName: NSApplication.didResignActiveNotification, object: nil, queue: .main) { [weak self] _ in
                    MainActor.assumeIsolated { self?.interrupt(discard: true) }
                },
            ]
        }

        func remove() {
            if let token { NSEvent.removeMonitor(token) }
            token = nil
            resignTokens.forEach(NotificationCenter.default.removeObserver)
            resignTokens = []
            interrupt(discard: false)
        }

        /// 録音を捨てる(テストは差し替える)
        var discardRecording: (() -> Void)?

        /// 押したまま別の操作で終わった。録音を始めていれば止める(`HoldToTalkGesture.interrupt`)。
        /// `discard`: 窓・アプリが手前でなくなった時は送らずに捨てる(離したつもりの無い録音を送らない)。窓を閉じた時は今まで通り止めて送る
        func interrupt(discard: Bool) {
            timer?.invalidate()
            heldKeyCode = nil
            let action = gesture.interrupt()
            if discard, action == .stop {
                if let discardRecording { discardRecording() } else { voice?.cancel() }
            } else {
                handle(action)
            }
        }

        /// キーを押した(`consumes` の keyDown)
        func press(_ mode: HoldToTalkGesture.Mode, keyCode: UInt16, isRecording: Bool) {
            heldKeyCode = keyCode
            handle(gesture.press(mode, at: Date(), isRecording: isRecording))
        }

        private func consumes(_ event: NSEvent) -> Bool {
            guard let window, event.window === window, window.isKeyWindow else { return false }
            switch event.type {
            case .keyDown:
                if gesture.isPressed {
                    // 押しているキーの繰り返しは飲み込む(スペースが入らない・⇧⌘M が何度も効かない)
                    return isHeldKey(event)
                }
                let field = window.firstResponder as? NSTextView
                // スペースは入力欄にいる時だけ(マイクのボタン等にいる時はそのボタンの操作に任せる)
                if event.keyCode == HoldToTalkKey.spaceKeyCode, field == nil { return false }
                let isRecording = voice?.isRecording ?? false
                guard enabled || isRecording,
                      let mode = HoldToTalkKey.mode(keyCode: event.keyCode, characters: event.charactersIgnoringModifiers,
                                                     modifiers: event.modifierFlags, draftIsEmpty: draftIsEmpty,
                                                     fieldHasText: !(field?.string.isEmpty ?? true),
                                                     isComposing: field?.hasMarkedText() ?? false) else { return false }
                press(mode, keyCode: event.keyCode, isRecording: isRecording)
                if mode == .delayed {
                    timer?.invalidate()
                    timer = Timer.scheduledTimer(withTimeInterval: HoldToTalkGesture.threshold, repeats: false) { [weak self] _ in
                        MainActor.assumeIsolated {
                            guard let self else { return }
                            self.handle(self.gesture.holdElapsed(at: Date()))
                        }
                    }
                }
                return true
            case .keyUp:
                guard gesture.isPressed, isHeldKey(event) else { return false }
                release()
                return true
            case .flagsChanged:
                // ⇧⌘M は ⌘ を離した時に M の keyUp が届かないことがある。⌘ が外れたら離したことにする
                if gesture.isPressed, gesture.mode == .immediate, !event.modifierFlags.contains(.command) { release() }
                return false
            default:
                return false
            }
        }

        private(set) var heldKeyCode: UInt16?

        private func isHeldKey(_ event: NSEvent) -> Bool { event.keyCode == heldKeyCode }

        private func release() {
            timer?.invalidate()
            heldKeyCode = nil
            handle(gesture.release(at: Date()))
        }
    }
}

extension View {
    /// 会話のシートに、押している間だけ録音するキー(#1220)を付ける
    func macHoldToTalkKeys(draftIsEmpty: Bool, enabled: Bool,
                           deliver: @escaping (String) -> Void, insertSpace: @escaping () -> Void) -> some View {
        modifier(MacHoldToTalkKeys(draftIsEmpty: draftIsEmpty, enabled: enabled, deliver: deliver, insertSpace: insertSpace))
    }
}

/// ビューが載っている窓を知る
private struct WindowReader: NSViewRepresentable {
    let found: (NSWindow?) -> Void

    func makeNSView(context: Context) -> NSView {
        let view = NSView()
        DispatchQueue.main.async { found(view.window) }
        return view
    }

    func updateNSView(_ nsView: NSView, context: Context) {
        DispatchQueue.main.async { found(nsView.window) }
    }
}
