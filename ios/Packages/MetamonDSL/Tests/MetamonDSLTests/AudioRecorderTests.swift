import XCTest
@testable import MetamonDSL

/// #420: audio field の値・ファイル名・上限・表示・録音の状態遷移とイヤホンのボタン。
final class AudioRecorderTests: XCTestCase {
    // MARK: - value

    func testValueRoundTrip() throws {
        let v = AudioValue(id: "6F1C2B7A-3D4E-4F00-9A1B-2C3D4E5F6A7B", seconds: 83)
        XCTAssertEqual(v.encoded, "audio:6F1C2B7A-3D4E-4F00-9A1B-2C3D4E5F6A7B|83")
        XCTAssertEqual(AudioValue(.string(v.encoded)), v)
        XCTAssertEqual(AudioValue(string: "audio:abc"), AudioValue(id: "abc", seconds: 0), "長さ無しも読める")
        XCTAssertEqual(AudioValue(string: "audio:abc|x"), AudioValue(id: "abc", seconds: 0), "壊れた長さは 0")
        XCTAssertEqual(AudioValue(id: "a", seconds: -3).seconds, 0)
    }

    func testRejectsOtherValues() {
        for s in ["", "audio:", "audio:../etc/passwd", "audio:a/b|3", "video:abc", "https://example.com/a.m4a", "abc"] {
            XCTAssertNil(AudioValue(string: s), s)
        }
        XCTAssertNil(AudioValue(.number(3)))
        XCTAssertNil(AudioValue(.null))
    }

    func testLocalIDs() {
        let data: [String: JSONValue] = ["voice": "audio:a1|3", "memo": "audio のメモ", "clip": "video:v1", "n": 1]
        XCTAssertEqual(AudioValue.localIDs(in: data), ["a1"])
    }

    func testFieldTypeDecodesAudio() throws {
        let f = try JSONDecoder().decode(Field.self, from: Data(#"{"id":"voice","name":"録音","type":"audio"}"#.utf8))
        XCTAssertEqual(f.type, .audio)
        XCTAssertEqual(try JSONDecoder().decode(Component.self, from: Data(#"{"type":"audioRecorder","field":"voice","maxDuration":120}"#.utf8)).type, .audioRecorder)
    }

    func testDisplay() throws {
        let f = try JSONDecoder().decode(Field.self, from: Data(#"{"id":"voice","name":"録音","type":"audio"}"#.utf8))
        XCTAssertEqual(FieldCoding.display("audio:a|83", field: f), "録音 1:23")
        XCTAssertEqual(FieldCoding.display("audio:a", field: f), "録音")
        XCTAssertEqual(FieldCoding.display(.null, field: f), "")
        XCTAssertEqual(FieldCoding.display("audio:a|5", field: f, words: .init(yes: "Yes", no: "No", photo: "Photo", audio: "Recording")), "Recording 0:05")
    }

    // MARK: - files

    func testStoreFileNaming() throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("audio-store-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = LocalAudioStore(directory: dir)
        XCTAssertEqual(store.url(for: "abc")?.lastPathComponent, "abc.m4a")
        XCTAssertNil(store.url(for: "../x"))
        XCTAssertNil(store.url(for: ""))
        XCTAssertTrue(LocalAudioStore.isValidID(LocalAudioStore.newID()))
        XCTAssertNotEqual(LocalAudioStore.newID(), LocalAudioStore.newID())

        let tmp = LocalAudioStore.temporaryRecordingURL(id: "r1")
        XCTAssertEqual(tmp.lastPathComponent, "metamon-rec-r1.m4a")
        try Data([1, 2, 3]).write(to: tmp)
        let id = try store.importFile(at: tmp)
        XCTAssertTrue(store.exists(id))
        XCTAssertFalse(FileManager.default.fileExists(atPath: tmp.path), "一時ファイルは move される")
        store.remove([id])
        XCTAssertFalse(store.exists(id))
    }

    // MARK: - policy

    func testMaxDurationIsBounded() {
        XCTAssertEqual(AudioRecorderPolicy.maxDuration(nil), 600, "既定 10 分")
        XCTAssertEqual(AudioRecorderPolicy.maxDuration(0), 600)
        XCTAssertEqual(AudioRecorderPolicy.maxDuration(-5), 600)
        XCTAssertEqual(AudioRecorderPolicy.maxDuration(90), 90)
        XCTAssertEqual(AudioRecorderPolicy.maxDuration(7200), 7200)
        XCTAssertEqual(AudioRecorderPolicy.maxDuration(99_999), 7200, "2 時間で頭打ち")
    }

    func testElapsedText() {
        XCTAssertEqual(AudioRecorderPolicy.elapsedText(0), "0:00")
        XCTAssertEqual(AudioRecorderPolicy.elapsedText(9.9), "0:09")
        XCTAssertEqual(AudioRecorderPolicy.elapsedText(600), "10:00")
        XCTAssertEqual(AudioRecorderPolicy.elapsedText(3723), "1:02:03")
        XCTAssertEqual(AudioRecorderPolicy.elapsedText(-1), "0:00")
        XCTAssertEqual(AudioRecorderPolicy.elapsedText(.nan), "0:00")
    }

    func testStoredSecondsRoundsUp() {
        XCTAssertEqual(AudioRecorderPolicy.storedSeconds(0.4), 1)
        XCTAssertEqual(AudioRecorderPolicy.storedSeconds(83.0), 83)
        XCTAssertEqual(AudioRecorderPolicy.storedSeconds(0), 0)
        XCTAssertEqual(AudioRecorderPolicy.storedSeconds(.infinity), 0)
    }

    func testMeterLevel() {
        XCTAssertEqual(AudioRecorderPolicy.meterLevel(power: -160), 0)
        XCTAssertEqual(AudioRecorderPolicy.meterLevel(power: -50), 0)
        XCTAssertEqual(AudioRecorderPolicy.meterLevel(power: -25), 0.5, accuracy: 0.001)
        XCTAssertEqual(AudioRecorderPolicy.meterLevel(power: 0), 1)
        XCTAssertEqual(AudioRecorderPolicy.meterLevel(power: 3), 1)
        XCTAssertEqual(AudioRecorderPolicy.meterLevel(power: .nan), 0)
    }

    // MARK: - state machine

    func testRecordStopDiscard() {
        var m = AudioRecorderMachine(hasClip: false)
        XCTAssertEqual(m.phase, .idle)
        XCTAssertFalse(m.send(.stop), "録音していないのに止めない")
        XCTAssertFalse(m.send(.discard))
        XCTAssertTrue(m.send(.start))
        XCTAssertEqual(m.phase, .recording)
        XCTAssertFalse(m.send(.start), "二重に始めない")
        XCTAssertTrue(m.send(.stop))
        XCTAssertEqual(m.phase, .recorded)
        XCTAssertTrue(m.send(.discard))
        XCTAssertEqual(m.phase, .idle)
    }

    func testFailureReturnsToPreviousState() {
        var fresh = AudioRecorderMachine(hasClip: false)
        fresh.send(.start)
        fresh.send(.fail)
        XCTAssertEqual(fresh.phase, .idle)

        var rerecord = AudioRecorderMachine(hasClip: true)
        XCTAssertEqual(rerecord.phase, .recorded)
        rerecord.send(.start)
        rerecord.send(.fail)
        XCTAssertEqual(rerecord.phase, .recorded, "録り直しに失敗したら元の録音に戻る")
    }

    func testEarphoneButtons() {
        var m = AudioRecorderMachine(hasClip: false)
        XCTAssertEqual(m.remoteAction(.togglePlayPause, isPlaying: false), .startRecording)
        XCTAssertEqual(m.remoteAction(.play, isPlaying: false), .startRecording)
        XCTAssertNil(m.remoteAction(.pause, isPlaying: false))
        m.send(.start)
        XCTAssertEqual(m.remoteAction(.togglePlayPause, isPlaying: false), .stopRecording)
        XCTAssertEqual(m.remoteAction(.pause, isPlaying: false), .stopRecording)
        XCTAssertNil(m.remoteAction(.play, isPlaying: false), "録音中の play は無視")
        m.send(.stop)
        XCTAssertEqual(m.remoteAction(.togglePlayPause, isPlaying: false), .startRecording, "録音済みなら録り直し")
        XCTAssertEqual(m.remoteAction(.togglePlayPause, isPlaying: true), .stopPlayback, "再生中は止めるだけ")
        XCTAssertEqual(m.remoteAction(.pause, isPlaying: true), .stopPlayback)
        XCTAssertNil(m.remoteAction(.play, isPlaying: true))
    }
}
