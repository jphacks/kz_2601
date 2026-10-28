import XCTest
import SwiftUI
import UIKit
import MetamonDSL
@testable import MetamonRenderer

/// #417 aiButton generate: どこで書くか(端末内 / オンライン)、依頼の組み立て、結果の描画。
final class AiGenerateTests: XCTestCase {
    private func ideas() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/ai_ideas.json"))
    }

    private func component(_ json: String) throws -> Component {
        try JSONDecoder().decode(Component.self, from: Data(json.utf8))
    }

    // MARK: どこで書くか

    func testRoutePrefersOnDeviceThenOnline() {
        let off = AIAvailability.unavailable(.deviceNotEligible)
        XCTAssertEqual(AIGenerateRoute.choose(availability: .available, hostOnline: true, mock: false), .onDevice, "端末内 AI が使えれば無料のそちら")
        XCTAssertEqual(AIGenerateRoute.choose(availability: off, hostOnline: true, mock: false), .online, "無ければオンライン(Pro)")
        XCTAssertEqual(AIGenerateRoute.choose(availability: off, hostOnline: false, mock: false),
                       .unavailable(off.message!), "プレビュー等オンラインの無いホストは理由を出して無効")
        XCTAssertEqual(AIGenerateRoute.choose(availability: .available, hostOnline: true, mock: true), .online, "モックは決まった文を返すホストへ")
        XCTAssertEqual(AIGenerateRoute.choose(availability: .available, hostOnline: false, mock: true), .onDevice)
    }

    // MARK: 依頼

    func testListAndFieldsRequestsFromExample() throws {
        let dsl = try ideas()
        let batsu = try XCTUnwrap(dsl.screen("home")?.components.first { $0.id == "batsu" })
        let req = try AIGenerateBuilder.build(batsu, form: nil, values: [:], sourceEntity: nil, records: [], locale: "ja")
        XCTAssertEqual(req.shape, .list)
        XCTAssertEqual(req.count, 5)
        XCTAssertNil(req.items)
        XCTAssertNil(req.input, "材料なし")
        XCTAssertTrue(req.instruction.hasPrefix("飲み会や家族"))

        let menu = try XCTUnwrap(dsl.screen("fridge")?.components.first { $0.id == "menu" })
        let food = try XCTUnwrap(dsl.entity("food"))
        XCTAssertThrowsError(try AIGenerateBuilder.build(menu, form: nil, values: [:], sourceEntity: food, records: [], locale: "ja")) {
            XCTAssertEqual($0 as? AIGenerateBuilder.BuildError, .noRecords)
        }
        let records = (0..<30).map { i in
            Record(id: "r\(i)", data: ["name": .string("食材\(i)"), "amount": "1 個"], updatedAt: String(format: "2026-10-%02dT00:00:00Z", i % 28 + 1))
        }
        let fields = try AIGenerateBuilder.build(menu, form: nil, values: [:], sourceEntity: food, records: records, locale: "ja")
        XCTAssertEqual(fields.shape, .fields)
        XCTAssertEqual(fields.items, ["主菜", "副菜", "汁物"])
        XCTAssertNil(fields.count)
        XCTAssertEqual(fields.input?.split(separator: "\n").count, AITextLimits.maxRecords, "最近の 20 件まで")
        XCTAssertTrue(fields.instruction.contains("下の材料"), "{{input}} は目印にして材料は input で渡す")
        XCTAssertFalse(fields.instruction.contains("食材0"))
    }

    func testInputIsClippedAndFormPlaceholdersExpand() throws {
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"""
            {"id":"d","name":"日記","fields":[{"id":"body","name":"本文","type":"text"},{"id":"reply","name":"返事","type":"string"}]}
            """#.utf8))
        let c = try component(#"{"type":"aiButton","task":"generate","prompt":"{{body}} に返事を","input":"body","output":"reply"}"#)
        let long = String(repeating: "あ", count: 3000) // 9000 バイト
        let req = try AIGenerateBuilder.build(c, form: entity, values: ["body": .string(long)], sourceEntity: nil, records: [], locale: "en")
        XCTAssertLessThanOrEqual(req.input?.utf8.count ?? 0, AITextLimits.maxInputBytes)
        XCTAssertEqual(req.instruction.count, AITextLimits.maxInstruction, "置換後も 500 字に切る")
        XCTAssertEqual(req.shape, .line)
        XCTAssertEqual(req.locale, "en")

        let empty = try component(#"{"type":"aiButton","task":"generate","prompt":"  "}"#)
        XCTAssertThrowsError(try AIGenerateBuilder.build(empty, form: nil, values: [:], sourceEntity: nil, records: [], locale: "ja"))
    }

    func testPromptDescribesShape() {
        let list = AITextRequest(instruction: "案", shape: .list, count: 4)
        XCTAssertTrue(AIGenerateBuilder.prompt(list).contains("4 個"))
        let fields = AITextRequest(instruction: "献立", shape: .fields, items: ["主菜", "汁物"], input: "- 鮭")
        XCTAssertTrue(AIGenerateBuilder.prompt(fields).contains("主菜 / 汁物"))
        XCTAssertTrue(AIGenerateBuilder.prompt(fields).contains("材料:\n- 鮭"))
        XCTAssertTrue(AIGenerateBuilder.instructions(locale: "en").contains("Write in English."))
    }

    // MARK: 描画

    @MainActor func testResultViewRendersEveryShape() {
        let results = [
            AITextResult(shape: .line, text: "今日もおつかれさま"),
            AITextResult(shape: .list, text: "・a\n・b", items: ["変顔で自撮り", "好きな歌を一節", "モノマネ"]),
            AITextResult(shape: .fields, text: "主菜: 鮭", fields: [AITextField(name: "主菜", value: "鮭の塩焼き"), AITextField(name: "汁物", value: "味噌汁")]),
        ]
        var heights: [CGFloat] = []
        for r in results {
            let host = UIHostingController(rootView: AITextResultView(result: r).frame(width: 320))
            let size = host.sizeThatFits(in: CGSize(width: 320, height: CGFloat.greatestFiniteMagnitude))
            XCTAssertGreaterThan(size.height, 20, "\(r.shape)")
            heights.append(size.height)
        }
        XCTAssertGreaterThan(heights[1], heights[0], "3 行の箇条書きは 1 行より高い")
    }

    /// フォームの無い画面でも「フォームが必要」にならず、ボタンが描ける(generate は screen.entity 不要)。
    @MainActor func testGenerateButtonRendersWithoutForm() throws {
        let dsl = try ideas()
        let batsu = try XCTUnwrap(dsl.screen("home")?.components.first { $0.id == "batsu" })
        let view = AiButtonLeaf(c: batsu, ctx: RenderContext(route: Route(screen: "home")))
        let host = UIHostingController(rootView: view.frame(width: 320))
        let size = host.sizeThatFits(in: CGSize(width: 320, height: 600))
        XCTAssertGreaterThan(size.height, 30)
    }
}
