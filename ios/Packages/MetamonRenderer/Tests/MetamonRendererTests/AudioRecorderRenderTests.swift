import XCTest
import SwiftUI
import AVFoundation
import MetamonDSL
@testable import MetamonRenderer

/// テスト用の AVAudioSession(設定と有効化の呼び出しだけ記録する)。
private final class FakeSession: AudioSessionControlling {
    var category: AVAudioSession.Category = .playAndRecord
    var mode: AVAudioSession.Mode = .default
    var categoryOptions: AVAudioSession.CategoryOptions = [.defaultToSpeaker, .allowBluetoothHFP]
    var active = false
    var deactivations: [AVAudioSession.SetActiveOptions] = []
    var failActivate = false

    func setCategory(_ category: AVAudioSession.Category, mode: AVAudioSession.Mode, options: AVAudioSession.CategoryOptions) throws {
        self.category = category
        self.mode = mode
        categoryOptions = options
    }

    func setActive(_ active: Bool, options: AVAudioSession.SetActiveOptions) throws {
        if active, failActivate { throw CocoaError(.featureUnsupported) }
        self.active = active
        if !active { deactivations.append(options) }
    }
}

/// #420: 録音の部品。セッションの貸し借り(音声入力を壊さない)と描画。
final class AudioRecorderRenderTests: XCTestCase {
    // MARK: - audio session

    /// 音声入力(WhisperKit)が設定した playAndRecord に、録音・再生の後で戻す。
    func testLeaseRestoresVoiceInputSession() throws {
        let s = FakeSession()
        let lease = try AudioSessionLease.forPlayback(s)
        XCTAssertEqual(s.category, .playback)
        XCTAssertTrue(s.active)
        lease.end()
        XCTAssertEqual(s.category, .playAndRecord)
        XCTAssertEqual(s.mode, .default)
        XCTAssertEqual(s.categoryOptions, [.defaultToSpeaker, .allowBluetoothHFP])
        XCTAssertFalse(s.active)
        XCTAssertEqual(s.deactivations, [.notifyOthersOnDeactivation], "他のアプリの音楽を戻す")
    }

    func testRecordingLeaseUsesPlayAndRecordWithBluetooth() throws {
        let s = FakeSession()
        s.category = .soloAmbient
        s.categoryOptions = []
        let lease = try AudioSessionLease.forRecording(s)
        XCTAssertEqual(s.category, .playAndRecord)
        XCTAssertTrue(s.categoryOptions.contains(.allowBluetoothHFP), "AirPods のマイクも使える")
        lease.end()
        XCTAssertEqual(s.category, .soloAmbient)
        XCTAssertEqual(s.categoryOptions, [])
    }

    /// 録音中に音声入力が始まって設定を変えたら、終わっても上書きしない。
    func testLeaseDoesNotClobberAnotherOwner() throws {
        let s = FakeSession()
        let lease = try AudioSessionLease.forRecording(s)
        try s.setCategory(.playAndRecord, mode: .measurement, options: [.allowBluetoothHFP])
        lease.end()
        XCTAssertEqual(s.mode, .measurement)
        XCTAssertTrue(s.active)
        XCTAssertTrue(s.deactivations.isEmpty)
    }

    func testLeaseFailureRestoresCategory() {
        let s = FakeSession()
        s.failActivate = true
        XCTAssertThrowsError(try AudioSessionLease.forRecording(s))
        XCTAssertEqual(s.categoryOptions, [.defaultToSpeaker, .allowBluetoothHFP])
    }

    // MARK: - render

    private func memo() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"memo","name":"メモ","fields":[{"id":"title","name":"タイトル","type":"string"},{"id":"voice","name":"録音","type":"audio"}]}
        """#.utf8))
    }

    private func recorder(field: String = "voice", maxDuration: Int? = nil) throws -> Component {
        var json = #"{"type":"audioRecorder","field":"\#(field)","label":"会議の録音""#
        if let maxDuration { json += #","maxDuration":\#(maxDuration)"# }
        return try JSONDecoder().decode(Component.self, from: Data((json + "}").utf8))
    }

    @MainActor private func render<V: View>(_ view: V) -> UIImage? {
        let r = ImageRenderer(content: view.frame(width: 360))
        r.scale = 1
        return r.uiImage
    }

    @MainActor func testRegistryBuildsTheRecorder() {
        XCTAssertNotNil(ComponentRegistry.standard.builder(for: "audioRecorder"))
        XCTAssertTrue(ClientCapabilities.reviewExcludedComponents.contains(.audioRecorder), "審査用ビルドでは外す")
    }

    @MainActor func testRendersEmptyAndRecordedStates() throws {
        let form = FormDraft(entity: try memo())
        let ctx = RenderContext(route: Route(screen: "form"), form: form)
        let c = try recorder(maxDuration: 3600)

        let empty = try XCTUnwrap(render(AudioRecorderLeaf(c: c, ctx: ctx)))
        XCTAssertGreaterThan(empty.size.height, 40)

        form.values["voice"] = .string(AudioValue(id: "not-on-this-device", seconds: 83).encoded)
        let recorded = try XCTUnwrap(render(AudioRecorderLeaf(c: c, ctx: ctx)))
        XCTAssertGreaterThan(recorded.size.height, empty.size.height, "録音済みは再生の行 + 録り直し / 削除")
    }

    @MainActor func testOutsideAFormShowsPlaceholder() throws {
        XCTAssertNotNil(render(AudioRecorderLeaf(c: try recorder(), ctx: RenderContext(route: Route(screen: "home")))))
    }

    @MainActor func testControllerClampsMaxDuration() {
        XCTAssertEqual(AudioRecorderController(maxDuration: nil, hasClip: false).maxDuration, 600)
        XCTAssertEqual(AudioRecorderController(maxDuration: 99_999, hasClip: true).maxDuration, 7200)
        XCTAssertEqual(AudioRecorderController(maxDuration: 30, hasClip: true).machine.phase, .recorded)
    }

    @MainActor func testRemoteCommandsAttachAndDetachByOwner() {
        let commands = RemoteRecorderCommands.shared
        let a = NSObject(), b = NSObject()
        commands.attach(owner: a, title: "A") { _ in }
        commands.attach(owner: b, title: "B") { _ in }
        commands.detach(owner: a)
        XCTAssertTrue(commands.isAttached, "前の画面が消えても新しい部品の登録は残す")
        commands.detach(owner: b)
        XCTAssertFalse(commands.isAttached)
    }

    @MainActor func testPlayButtonWithoutLocalFile() {
        let missing = AudioValue(id: "missing", seconds: 5)
        XCTAssertFalse(AudioPlayback.shared.toggle(missing), "この端末に無い録音は鳴らさない")
        XCTAssertNil(AudioPlayback.shared.playingID)
        XCTAssertNotNil(render(AudioPlayButton(value: .string(missing.encoded))))
    }
}
