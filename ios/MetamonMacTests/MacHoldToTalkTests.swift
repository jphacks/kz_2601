import AppKit
import XCTest
import MetamonServices
@testable import Metamon

/// 押している間だけ録音(#1220)と入力の装置の選択(#1221)
@MainActor
final class MacHoldToTalkTests: XCTestCase {
    private let t0 = Date(timeIntervalSince1970: 1_000)
    private func at(_ s: TimeInterval) -> Date { t0.addingTimeInterval(s) }

    // MARK: - 押している間だけ録音の状態

    func testImmediateShortPressKeepsToggleBehavior() {
        var g = HoldToTalkGesture()
        XCTAssertEqual(g.press(.immediate, at: at(0), isRecording: false), .start)
        XCTAssertEqual(g.release(at: at(0.1)), .none) // 短く押した: 録音を続ける(もう一度押して止める)
        XCTAssertFalse(g.isPressed)
    }

    func testImmediateLongPressStopsOnRelease() {
        var g = HoldToTalkGesture()
        XCTAssertEqual(g.press(.immediate, at: at(0), isRecording: false), .start)
        XCTAssertEqual(g.release(at: at(1.5)), .stop)
    }

    func testPressWhileRecordingStopsAndReleaseDoesNothing() {
        var g = HoldToTalkGesture()
        XCTAssertEqual(g.press(.immediate, at: at(0), isRecording: true), .stop)
        XCTAssertEqual(g.release(at: at(2)), .none)
        XCTAssertEqual(g.press(.delayed, at: at(3), isRecording: true), .stop)
        XCTAssertEqual(g.holdElapsed(at: at(4)), .none)
        XCTAssertEqual(g.release(at: at(5)), .none)
    }

    func testKeyRepeatIsIgnored() {
        var g = HoldToTalkGesture()
        XCTAssertEqual(g.press(.immediate, at: at(0), isRecording: false), .start)
        XCTAssertEqual(g.press(.immediate, at: at(0.5), isRecording: true), .none)
        XCTAssertEqual(g.release(at: at(1)), .stop)
    }

    func testDelayedShortTapTypesASpace() {
        var g = HoldToTalkGesture()
        XCTAssertEqual(g.press(.delayed, at: at(0), isRecording: false), .none)
        XCTAssertEqual(g.holdElapsed(at: at(0.1)), .none) // まだ短い
        XCTAssertEqual(g.release(at: at(0.15)), .insertSpace)
    }

    func testDelayedHoldStartsAfterThresholdAndStopsOnRelease() {
        var g = HoldToTalkGesture()
        XCTAssertEqual(g.press(.delayed, at: at(0), isRecording: false), .none)
        XCTAssertEqual(g.holdElapsed(at: at(HoldToTalkGesture.threshold + 0.01)), .start)
        XCTAssertEqual(g.holdElapsed(at: at(1)), .none) // 二度は始めない
        XCTAssertEqual(g.release(at: at(2)), .stop)
    }

    func testInterruptStopsOnlyWhenStarted() {
        var g = HoldToTalkGesture()
        _ = g.press(.delayed, at: at(0), isRecording: false)
        XCTAssertEqual(g.interrupt(), .none)
        _ = g.press(.immediate, at: at(1), isRecording: false)
        XCTAssertEqual(g.interrupt(), .stop)
        XCTAssertFalse(g.isPressed)
    }

    func testResigningKeyWindowOrAppReleasesHeldKey() {
        let monitor = MacHoldToTalkKeys.Monitor()
        let window = NSWindow()
        monitor.window = window
        var discarded = 0
        monitor.discardRecording = { discarded += 1 }
        monitor.install()
        defer { monitor.remove() }

        monitor.press(.immediate, keyCode: 46, isRecording: false)
        XCTAssertTrue(monitor.gesture.isPressed)
        NotificationCenter.default.post(name: NSWindow.didResignKeyNotification, object: NSWindow()) // ほかの窓
        XCTAssertTrue(monitor.gesture.isPressed, "ほかの窓は関係ない")
        NotificationCenter.default.post(name: NSWindow.didResignKeyNotification, object: window)
        XCTAssertFalse(monitor.gesture.isPressed, "keyUp が届かなくても押したままにしない")
        XCTAssertNil(monitor.heldKeyCode)
        XCTAssertEqual(discarded, 1, "始めていた録音は送らずに捨てる")

        monitor.press(.delayed, keyCode: HoldToTalkKey.spaceKeyCode, isRecording: false)
        XCTAssertTrue(monitor.gesture.isPressed)
        NotificationCenter.default.post(name: NSApplication.didResignActiveNotification, object: NSApp)
        XCTAssertFalse(monitor.gesture.isPressed)
        XCTAssertNil(monitor.heldKeyCode)
        XCTAssertEqual(discarded, 1, "まだ始めていない(短いスペース)なら捨てる物は無い")

        monitor.press(.immediate, keyCode: 46, isRecording: false)
        NotificationCenter.default.post(name: NSApplication.didResignActiveNotification, object: NSApp)
        XCTAssertEqual(discarded, 2)

        monitor.press(.immediate, keyCode: 46, isRecording: false)
        monitor.interrupt(discard: false) // 窓を閉じた時(`remove`)は今まで通り止めて送る
        XCTAssertEqual(discarded, 2)
    }

    // MARK: - どのキーを使うか

    func testKeyClassification() {
        func mode(_ code: UInt16, _ chars: String?, _ mods: NSEvent.ModifierFlags,
                  empty: Bool = true, text: Bool = false, composing: Bool = false) -> HoldToTalkGesture.Mode? {
            HoldToTalkKey.mode(keyCode: code, characters: chars, modifiers: mods,
                               draftIsEmpty: empty, fieldHasText: text, isComposing: composing)
        }
        XCTAssertEqual(mode(49, " ", []), .delayed)
        XCTAssertEqual(mode(49, " ", [.capsLock]), .delayed)
        XCTAssertNil(mode(49, " ", [], empty: false)) // 入力中は普通のスペース
        XCTAssertNil(mode(49, " ", [], text: true))
        XCTAssertNil(mode(49, " ", [], composing: true)) // 変換中
        XCTAssertNil(mode(49, " ", [.option]))
        XCTAssertNil(mode(49, " ", [.command]))
        XCTAssertEqual(mode(46, "m", [.command, .shift]), .immediate)
        XCTAssertEqual(mode(46, "M", [.command, .shift], empty: false, text: true), .immediate)
        XCTAssertNil(mode(46, "m", [.command]))
        XCTAssertNil(mode(46, "m", []))
        XCTAssertNil(mode(0, "a", []))
    }

    // MARK: - 録音の開始待ちに離した時

    func testReleaseDuringPendingStartStopsAndDelivers() async throws {
        let voice = MacVoiceInput(transcriber: MockTranscriber(cannedText: "家計簿", delay: .zero),
                                  defaults: makeDefaults(), slowHardware: false,
                                  isExpensiveNetwork: { false }, currentInputUID: { "mic-a" })
        var delivered: [String] = []
        voice.holdStart { delivered.append($0) }
        XCTAssertEqual(voice.state, .recording) // マイクを開く前から録音中
        voice.holdStop { delivered.append($0) } // 開く前に離した
        try await waitUntil { voice.state == .idle }
        XCTAssertEqual(delivered, ["家計簿"])
    }

    func testLongHoldStopsAndDelivers() async throws {
        let voice = MacVoiceInput(transcriber: MockTranscriber(cannedText: "支出", delay: .zero),
                                  defaults: makeDefaults(), slowHardware: false,
                                  isExpensiveNetwork: { false }, currentInputUID: { "mic-a" })
        var delivered: [String] = []
        voice.holdStart { delivered.append($0) }
        try await Task.sleep(for: .milliseconds(50))
        voice.holdStop { delivered.append($0) }
        try await waitUntil { voice.state == .idle }
        XCTAssertEqual(delivered, ["支出"])
    }

    // MARK: - 入力の装置

    private let micA = MacInputDevice(id: 10, uid: "mic-a", name: "外付けマイク")
    private let airPods = MacInputDevice(id: 11, uid: "airpods", name: "AirPods")

    func testResolveFallsBackToSystemDefault() {
        XCTAssertNil(MacAudioInput.resolve(preferredUID: nil, devices: [micA, airPods]))
        XCTAssertEqual(MacAudioInput.resolve(preferredUID: "airpods", devices: [micA, airPods]), airPods)
        XCTAssertNil(MacAudioInput.resolve(preferredUID: "airpods", devices: [micA])) // 外した: 既定に戻る
    }

    func testDeviceListPersistsSelectionAndFallsBack() {
        var current = [micA, airPods]
        var saved: (uid: String?, name: String?) = (nil, nil)
        let store = MacInputDevices.Store(load: { saved }, save: { uid, name in saved = (uid, name) })
        let list = MacInputDevices(provider: { current }, store: store)
        XCTAssertNil(list.effectiveDevice)

        list.select("airpods")
        XCTAssertEqual(saved.uid, "airpods")
        XCTAssertEqual(saved.name, "AirPods")
        XCTAssertEqual(list.effectiveDevice, airPods)

        var notified: Set<String> = []
        list.onChange = { notified = $0 }
        current = [micA] // AirPods を外した
        list.refresh()
        XCTAssertEqual(notified, ["mic-a"])
        XCTAssertNil(list.effectiveDevice)
        XCTAssertTrue(list.isPreferredMissing)
        XCTAssertEqual(list.preferredUID, "airpods") // 選んだ値は残す

        current = [micA, airPods] // つなぎ直した
        list.refresh()
        XCTAssertEqual(list.effectiveDevice, airPods)
        XCTAssertFalse(list.isPreferredMissing)

        // 起動し直しても UID で戻る
        let reloaded = MacInputDevices(provider: { current }, store: store)
        XCTAssertEqual(reloaded.effectiveDevice, airPods)

        list.select(nil)
        XCTAssertNil(saved.uid)
        XCTAssertNil(list.effectiveDevice)
    }

    func testRecordingDeviceRemovedCancelsWithMessage() async throws {
        let mock = MockTranscriber(cannedText: "x", delay: .zero)
        let voice = MacVoiceInput(transcriber: mock, defaults: makeDefaults(), slowHardware: false,
                                  isExpensiveNetwork: { false }, currentInputUID: { "airpods" })
        voice.holdStart { _ in }
        try await waitUntil { mock.isRecording }
        voice.inputDevicesChanged(available: ["airpods", "mic-a"]) // 別の装置の抜き差しは無視
        XCTAssertEqual(voice.state, .recording)
        voice.inputDevicesChanged(available: ["mic-a"])
        XCTAssertEqual(voice.state, .idle)
        XCTAssertFalse(mock.isRecording)
        XCTAssertEqual(voice.failure, MacVoiceCopy.inputDeviceLost)
    }

    // MARK: -

    private func makeDefaults() -> UserDefaults {
        let name = "MacHoldToTalkTests.\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: name)!
        defaults.removePersistentDomain(forName: name)
        return defaults
    }

    private func waitUntil(_ condition: () -> Bool) async throws {
        for _ in 0..<200 where !condition() { try await Task.sleep(for: .milliseconds(10)) }
        XCTAssertTrue(condition())
    }
}
