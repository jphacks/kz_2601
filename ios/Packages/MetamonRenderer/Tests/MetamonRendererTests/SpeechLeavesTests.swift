import XCTest
import SwiftUI
import MetamonDSL
@testable import MetamonRenderer

/// `transcribeField`(話して入力)と `speakButton`(読み上げ): 文字の足し方・読み上げる中身と声の言語・録音中は喋らない・
/// ホストが無い / フォームの外でも落ちない。
final class SpeechLeavesTests: XCTestCase {
    private func component(_ json: String) throws -> Component {
        try JSONDecoder().decode(Component.self, from: Data(json.utf8))
    }

    private func note() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"note","name":"メモ","fields":[
          {"id":"title","name":"タイトル","type":"string"},
          {"id":"body","name":"本文","type":"text"}
        ]}
        """#.utf8))
    }

    func testDictationAppends() {
        XCTAssertEqual(DictationText.appending("こんにちは", to: ""), "こんにちは")
        XCTAssertEqual(DictationText.appending("こんにちは", to: "  \n"), "こんにちは", "空白だけなら置き換える")
        XCTAssertEqual(DictationText.appending(" 二つ目 ", to: "一つ目"), "一つ目\n二つ目", "改行で区切って後ろに足す")
        XCTAssertEqual(DictationText.appending("二つ目", to: "一つ目\n"), "一つ目\n二つ目")
        XCTAssertEqual(DictationText.appending("  ", to: "一つ目"), "一つ目", "聞き取れなかった分は足さない")
    }

    func testVoiceFollowsUILanguage() {
        XCTAssertEqual(LeafSpeech.voiceLanguage(uiCode: "ja"), "ja-JP")
        XCTAssertEqual(LeafSpeech.voiceLanguage(uiCode: "en"), "en-US")
    }

    @MainActor func testDoesNotSpeakWhileRecordingOrEmpty() {
        let speech = LeafSpeech.shared
        XCTAssertFalse(speech.speak("   ", owner: "a"))
        speech.stopForRecording()
        XCTAssertFalse(speech.speak("こんにちは", owner: "a"), "録音中は喋らない(マイクが拾う)")
        XCTAssertNil(speech.speakingOwner)
        speech.recordingFinished()
        speech.stop(owner: "other")
        XCTAssertNil(speech.speakingOwner)
    }

    @MainActor func testSpeakContentSources() throws {
        let entity = try note()
        let form = FormDraft(entity: entity)
        form.values["title"] = "買い物"
        form.values["body"] = "牛乳を買う"
        let ev = { ExprEvaluator(entities: [entity], records: { _ in [] }) }
        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"speakButton","field":"body"}"#), form: form, evaluator: ev), "牛乳を買う")
        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"speakButton","text":"{{title}}。{{body}}"}"#), form: form, evaluator: ev), "買い物。牛乳を買う")
        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"speakButton","text":"おはよう"}"#), form: nil, evaluator: ev), "おはよう")
        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"speakButton","field":"body"}"#), form: nil, evaluator: ev), "")
    }

    /// ホストが無い(プレビュー)・フォームの外でも描ける。
    @MainActor func testRenderWithoutHost() throws {
        let entity = try note()
        let form = FormDraft(entity: entity)
        let ctx = RenderContext(route: Route(screen: "form"), form: form)
        let views: [AnyView] = [
            AnyView(TranscribeFieldLeaf(c: try component(#"{"type":"transcribeField","field":"body","label":"本文"}"#), ctx: ctx)),
            AnyView(TranscribeFieldLeaf(c: try component(#"{"type":"transcribeField","field":"body"}"#), ctx: RenderContext(route: Route(screen: "home")))),
            AnyView(SpeakButtonLeaf(c: try component(#"{"type":"speakButton","field":"body"}"#), ctx: ctx)),
            AnyView(SpeakButtonLeaf(c: try component(#"{"type":"speakButton","text":"こんにちは","label":"聞く"}"#), ctx: RenderContext(route: Route(screen: "home")))),
        ]
        for v in views {
            XCTAssertNotNil(ImageRenderer(content: v.frame(width: 320, height: 240)).platformImage)
        }
    }

    func testRegisteredAndFormBound() {
        XCTAssertTrue(ComponentRegistry.standard.registeredTypes.isSuperset(of: ["transcribeField", "speakButton"]))
        XCTAssertTrue(RenderTree.formBound.contains(.transcribeField))
    }
}
