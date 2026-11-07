import XCTest
import SwiftUI
import MetamonDSL
@testable import MetamonRenderer

/// `text.highlight`(jphacks/E_2202): 語の照合(大文字小文字・全角半角・日本語の一部)・field の語の区切りと上限・
/// 印の付け方・`{{fieldId}}` の差し込み。
final class TextHighlightTests: XCTestCase {
    private func marked(_ text: String, _ words: [String]) -> [String] {
        TextHighlightMatcher.ranges(in: text, words: words).map { String(text[$0]) }
    }

    func testMatchesCaseWidthAndJapaneseSubstrings() {
        XCTAssertEqual(marked("TypeError: Cannot read", ["typeerror"]), ["TypeError"])
        XCTAssertEqual(marked("ＴｙｐｅＥｒｒｏｒ が出た", ["TypeError"]), ["ＴｙｐｅＥｒｒｏｒ"], "全角の文字も合う(元の文字のまま印)")
        XCTAssertEqual(marked("ﾀｲﾑｱｳﾄしました", ["タイムアウト"]), ["ﾀｲﾑｱｳﾄ"], "半角カナも合う")
        XCTAssertEqual(marked("接続がタイムアウトしました", ["タイムアウト"]), ["タイムアウト"], "日本語は語の一部でも合う")
        XCTAssertEqual(marked("null と null", ["null"]), ["null", "null"], "何度出ても全部")
        XCTAssertEqual(marked("undefined is not", ["undefined is", "is not"]), ["undefined is not"], "重なりはまとめる")
        XCTAssertEqual(marked("abc", ["x", ""]), [])
        XCTAssertEqual(marked("", ["a"]), [])
        XCTAssertEqual(marked("👍🏽 ok", ["ok"]), ["ok"], "合成された絵文字の後ろでもずれない")
    }

    func testWordsFromFieldAndCap() {
        let h = TextHighlight(field: "keywords")
        XCTAssertEqual(h.resolvedWords(fieldValue: "TypeError, undefined、タイムアウト\nnull，null ,  "),
                       ["TypeError", "undefined", "タイムアウト", "null"], "「,」「、」「，」改行で区切り・空と重複は捨てる")
        XCTAssertEqual(h.resolvedWords(fieldValue: nil), [])
        let many = (1...30).map { "w\($0)" }.joined(separator: ",")
        XCTAssertEqual(h.resolvedWords(fieldValue: many).count, TextHighlight.maxWords, "語は 20 個まで")
        XCTAssertEqual(TextHighlight(words: ["期限"]).resolvedWords(fieldValue: "無視"), ["期限"])
    }

    func testDecodes() throws {
        let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"text","text":"x","highlight":{"field":"kw","style":"bold"}}"#.utf8))
        XCTAssertEqual(c.highlight, TextHighlight(field: "kw", style: "bold"))
        let bad = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"text","text":"x","highlight":"oops"}"#.utf8))
        XCTAssertNil(bad.highlight, "形が違えば印なしで描く")
        XCTAssertEqual(bad.text, "x")
    }

    func testAttributedMarksOnlyMatches() {
        let s = TextHighlightStyle.attributed("期限は明日", words: ["期限"], style: nil)
        XCTAssertEqual(String(s.characters), "期限は明日", "文そのものは変えない(VoiceOver はそのまま読む)")
        let runs = s.runs.map { (String(s[$0.range].characters), $0.inlinePresentationIntent, $0.backgroundColor) }
        XCTAssertEqual(runs.count, 2)
        XCTAssertEqual(runs[0].0, "期限")
        XCTAssertEqual(runs[0].1, .stronglyEmphasized)
        XCTAssertNotNil(runs[0].2)
        XCTAssertNil(runs[1].1)
        XCTAssertNil(runs[1].2)
        let bold = TextHighlightStyle.attributed("期限は明日", words: ["期限"], style: "bold")
        XCTAssertNil(bold.runs.first?.backgroundColor, "bold は背景を付けない")
        XCTAssertEqual(bold.runs.first?.inlinePresentationIntent, .stronglyEmphasized)
    }

    @MainActor func testTextShowsRecordValue() throws {
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"error","name":"エラー","fields":[{"id":"message","name":"m","type":"text"},{"id":"keywords","name":"k","type":"string"}]}
        """#.utf8))
        let form = FormDraft(entity: entity)
        form.values["message"] = "TypeError: x is undefined"
        let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"text","text":"エラー: {{message}}","highlight":{"field":"keywords"}}"#.utf8))
        let ev = { ExprEvaluator(entities: [entity], records: { _ in [] }) }
        XCTAssertEqual(TextContent.resolve(c.text ?? "", form: form, evaluator: ev), "エラー: TypeError: x is undefined")
        XCTAssertEqual(TextContent.resolve(c.text ?? "", form: nil, evaluator: ev), "エラー: {{message}}", "フォームの外は文字のまま")
        form.values["keywords"] = "typeerror, undefined"
        for v in [AnyView(TextLeaf(c: c, ctx: RenderContext(route: Route(screen: "form"), form: form))), AnyView(TextLeaf(c: c))] {
            XCTAssertNotNil(ImageRenderer(content: v.frame(width: 320, height: 120)).uiImage, "フォームの外(プレビュー)でも描ける")
        }
    }

    /// 知らない名前・写真・hidden の `{{x}}` は書いたままの文字(hidden の値は決して出さない)。
    @MainActor func testOnlyVisibleScalarFieldsSubstitute() throws {
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"m","name":"m","fields":[{"id":"body","name":"b","type":"text"},{"id":"pic","name":"p","type":"photo"},
          {"id":"secret","name":"s","type":"string","hidden":true},{"id":"n","name":"n","type":"integer"}]}
        """#.utf8))
        let form = FormDraft(entity: entity)
        form.values["body"] = "本文"
        form.values["pic"] = "photo:abc"
        form.values["secret"] = "ひみつの答え"
        form.values["n"] = 3
        let ev = { ExprEvaluator(entities: [entity], records: { _ in [] }) }
        let out = TextContent.resolve("{{body}}|{{n}}|{{zzz}}|{{pic}}|{{secret}}", form: form, evaluator: ev)
        XCTAssertEqual(out, "本文|3|{{zzz}}|{{pic}}|{{secret}}")
        XCTAssertFalse(out.contains("ひみつ"), "hidden の値は出さない")
        XCTAssertTrue(TextHighlight.substitutes(entity, "body"))
        XCTAssertFalse(TextHighlight.substitutes(entity, "secret"))
        XCTAssertFalse(TextHighlight.substitutes(nil, "body"))
    }
}
