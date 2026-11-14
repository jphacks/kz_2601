import XCTest
@testable import Metamon

/// 完成の要約の読み上げ(#1210)と設定の保存先(#1216)
@MainActor
final class MacSpeechReaderTests: XCTestCase {
    private final class FakeEngine: MacSpeechEngine {
        var isSpeaking = false
        var spoken: [(text: String, language: String)] = []
        var stops = 0

        func speak(_ text: String, language: String) {
            spoken.append((text, language))
            isSpeaking = true
        }

        func stop() {
            stops += 1
            isSpeaking = false
        }
    }

    private var defaults: UserDefaults!
    private let suite = "MacSpeechReaderTests"

    override func setUp() {
        super.setUp()
        defaults = UserDefaults(suiteName: suite)
        defaults.removePersistentDomain(forName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        super.tearDown()
    }

    private func reader(_ engine: FakeEngine, voiceOver: Bool = false, language: String = "ja") -> MacSpeechReader {
        MacSpeechReader(engine: engine, defaults: defaults, isVoiceOverRunning: { voiceOver }, languageCode: { language })
    }

    func testSpeaksWhenEnabled() {
        let engine = FakeEngine()
        let reader = reader(engine)
        reader.enabled = true
        reader.speak("  できました  ")
        XCTAssertEqual(engine.spoken.map(\.text), ["できました"])
        XCTAssertEqual(engine.spoken.first?.language, "ja-JP")
    }

    func testLanguageFollowsAppLanguage() {
        let engine = FakeEngine()
        let reader = reader(engine, language: "en")
        reader.enabled = true
        reader.speak("Done")
        XCTAssertEqual(engine.spoken.first?.language, "en-US")
    }

    func testSilentWhenDisabledByDefault() {
        let engine = FakeEngine()
        let reader = reader(engine)
        XCTAssertFalse(reader.enabled, "既定は OFF(iOS と同じ)")
        reader.speak("できました")
        XCTAssertTrue(engine.spoken.isEmpty)
    }

    func testSilentWhenVoiceOverRunning() {
        let engine = FakeEngine()
        let reader = reader(engine, voiceOver: true)
        reader.enabled = true
        reader.speak("できました")
        XCTAssertTrue(engine.spoken.isEmpty, "VoiceOver の読み上げに重ねない")
    }

    func testIgnoresEmptyText() {
        let engine = FakeEngine()
        let reader = reader(engine)
        reader.enabled = true
        reader.speak("   ")
        XCTAssertTrue(engine.spoken.isEmpty)
    }

    func testNewRequestStopsPreviousSpeech() {
        let engine = FakeEngine()
        let reader = reader(engine)
        reader.enabled = true
        reader.speak("1 つ目")
        reader.speak("2 つ目")
        XCTAssertEqual(engine.stops, 1, "前の読み上げを打ち切ってから喋る")
        XCTAssertEqual(engine.spoken.map(\.text), ["1 つ目", "2 つ目"])
        reader.stop()
        XCTAssertEqual(engine.stops, 2)
        XCTAssertFalse(engine.isSpeaking)
    }

    func testTurningOffStopsSpeech() {
        let engine = FakeEngine()
        let reader = reader(engine)
        reader.enabled = true
        reader.speak("できました")
        reader.enabled = false
        XCTAssertFalse(engine.isSpeaking)
    }

    /// iOS の `SpeechReader` と同じキーに保存する(設定を共有する)
    func testSharesStoredPreferenceKeyWithIOS() {
        XCTAssertEqual(MacSpeechReader.key, "metamon.speech.enabled")
        let reader = reader(FakeEngine())
        reader.enabled = true
        XCTAssertEqual(defaults.object(forKey: "metamon.speech.enabled") as? Bool, true)
        XCTAssertTrue(self.reader(FakeEngine()).enabled, "保存した値を次に読む")
    }
}
