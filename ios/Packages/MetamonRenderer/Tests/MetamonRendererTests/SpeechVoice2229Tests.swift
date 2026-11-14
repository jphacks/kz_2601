import AVFoundation
import XCTest
@testable import MetamonRenderer

/// 読み上げの声と速さ(#2229)。選んだ声・消えた声・言語が違う時の決め方と保存を確かめる。
final class SpeechVoice2229Tests: XCTestCase {
    private let kyoko = SpeechVoiceOption(id: "ja.kyoko", name: "Kyoko", language: "ja-JP", quality: 0)
    private let kyokoEnhanced = SpeechVoiceOption(id: "ja.kyoko.enh", name: "Kyoko", language: "ja-JP", quality: 1)
    private let otoya = SpeechVoiceOption(id: "ja.otoya", name: "Otoya", language: "ja-JP", quality: 1)
    private let mine = SpeechVoiceOption(id: "ja.personal", name: "私の声", language: "ja-JP", quality: 2, isPersonal: true)
    private let samantha = SpeechVoiceOption(id: "en.samantha", name: "Samantha", language: "en-US", quality: 2)
    private var all: [SpeechVoiceOption] { [kyoko, kyokoEnhanced, otoya, mine, samantha] }

    func testChosenVoiceIsUsed() {
        let r = SpeechVoiceChoice.resolve(language: "ja-JP", chosenId: "ja.otoya", options: all)
        XCTAssertEqual(r, .init(voiceId: "ja.otoya", chosenIsGone: false))
        let personal = SpeechVoiceChoice.resolve(language: "ja-JP", chosenId: "ja.personal", options: all)
        XCTAssertEqual(personal.voiceId, "ja.personal")
    }

    func testAutomaticPicksBestQualityButNotPersonal() {
        let r = SpeechVoiceChoice.resolve(language: "ja-JP", chosenId: nil, options: all)
        XCTAssertEqual(r, .init(voiceId: "ja.kyoko.enh", chosenIsGone: false)) // 同じ品質は先のもの
    }

    func testGoneVoiceFallsBackToAutomatic() {
        let r = SpeechVoiceChoice.resolve(language: "ja-JP", chosenId: "ja.removed", options: all)
        XCTAssertEqual(r, .init(voiceId: "ja.kyoko.enh", chosenIsGone: true))
    }

    func testDifferentLanguageUsesAutomaticWithoutForgetting() {
        let r = SpeechVoiceChoice.resolve(language: "en-US", chosenId: "ja.otoya", options: all)
        XCTAssertEqual(r, .init(voiceId: "en.samantha", chosenIsGone: false))
    }

    func testNoVoiceForLanguageReturnsNil() {
        let r = SpeechVoiceChoice.resolve(language: "fr-FR", chosenId: nil, options: all)
        XCTAssertNil(r.voiceId)
    }

    func testOptionsListOrder() {
        let ids = SpeechVoiceChoice.options(for: "ja-JP", in: all).map(\.id)
        XCTAssertEqual(ids, ["ja.personal", "ja.kyoko.enh", "ja.otoya", "ja.kyoko"])
    }

    func testRateStaysInSystemRange() {
        for r in SpeechRateChoice.allCases {
            XCTAssertGreaterThanOrEqual(r.rate, AVSpeechUtteranceMinimumSpeechRate)
            XCTAssertLessThanOrEqual(r.rate, AVSpeechUtteranceMaximumSpeechRate)
        }
        XCTAssertLessThan(SpeechRateChoice.slow.rate, SpeechRateChoice.normal.rate)
        XCTAssertLessThan(SpeechRateChoice.normal.rate, SpeechRateChoice.fast.rate)
        XCTAssertEqual(SpeechRateChoice.normal.rate, AVSpeechUtteranceDefaultSpeechRate)
    }

    func testPreferenceStoresPerLanguageAndRate() throws {
        let suite = "SpeechVoice2229Tests"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let pref = SpeechVoicePreference(defaults: defaults)
        XCTAssertNil(pref.voiceId(for: "ja-JP"))
        XCTAssertEqual(pref.rate, .normal)
        pref.setVoiceId("ja.otoya", for: "ja-JP")
        pref.rate = .fast
        XCTAssertEqual(pref.voiceId(for: "ja-JP"), "ja.otoya")
        XCTAssertNil(pref.voiceId(for: "en-US"))
        XCTAssertEqual(pref.rate, .fast)
        pref.setVoiceId(nil, for: "ja-JP")
        XCTAssertNil(pref.voiceId(for: "ja-JP"))
        defaults.set("weird", forKey: SpeechVoicePreference.rateKey)
        XCTAssertEqual(pref.rate, .normal)
    }

    @MainActor
    func testConfigureForgetsGoneVoice() throws {
        let suite = "SpeechVoice2229Tests.configure"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let saved = SpeechVoice.preference
        defer { SpeechVoice.preference = saved }
        SpeechVoice.preference = SpeechVoicePreference(defaults: defaults)
        SpeechVoice.preference.setVoiceId("com.example.no-such-voice", for: "ja-JP")
        SpeechVoice.preference.rate = .slow
        let u = AVSpeechUtterance(string: "こんにちは")
        SpeechVoice.configure(u, language: "ja-JP")
        XCTAssertNil(SpeechVoice.preference.voiceId(for: "ja-JP"))
        XCTAssertEqual(u.rate, SpeechRateChoice.slow.rate)
    }
}
