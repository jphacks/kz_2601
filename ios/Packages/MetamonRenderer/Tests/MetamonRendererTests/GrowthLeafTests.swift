import SwiftUI
import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// `growth`(数値で育つ絵): 実の数・育ち切り・数値の出どころ・例の DSL(歩数で木に実がなる)が描けること。
final class GrowthLeafTests: XCTestCase {
    private func stepsTree() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/steps_tree.json"))
    }

    // MARK: 模型

    func testOneFruitPerStepUntilTarget() {
        let m = GrowthModel(value: 3214, target: 8000, step: 1000)
        XCTAssertEqual(m.capacity, 8)
        XCTAssertEqual(m.items, 3)
        XCTAssertEqual(m.nextAt, 4000)
        XCTAssertFalse(m.isComplete)
        XCTAssertEqual(GrowthModel(value: 3999.9, target: 8000, step: 1000).items, 3)
        XCTAssertEqual(GrowthModel(value: 4000, target: 8000, step: 1000).items, 4, "ちょうど step の倍数で 1 つ増える")
        let done = GrowthModel(value: 9100, target: 8000, step: 1000)
        XCTAssertTrue(done.isComplete)
        XCTAssertEqual(done.items, 8)
        XCTAssertNil(done.nextAt)
        XCTAssertEqual(done.progress, 1)
    }

    func testDefaultsAndBadInput() {
        XCTAssertEqual(GrowthModel(value: 0, target: 8000, step: nil).capacity, 10, "step 省略は target の 1/10")
        XCTAssertEqual(GrowthModel(value: 50, target: 100, step: 1).capacity, GrowthModel.maxItems, "小さすぎる step は 30 個に収める")
        XCTAssertEqual(GrowthModel(value: 10, target: 0, step: nil).target, 1, "0 で割らない")
        XCTAssertEqual(GrowthModel(value: -5, target: 10, step: nil).items, 0)
        XCTAssertEqual(GrowthModel(value: .nan, target: 10, step: .infinity).items, 0)
        XCTAssertEqual(GrowthModel(value: 8500, target: 8000, step: 3000).capacity, 3, "端数は切り上げ(3000・6000・8000)")
        XCTAssertEqual(GrowthModel(value: 8000, target: 8000, step: 3000).items, 3, "target に届いたら全部")
        XCTAssertEqual(GrowthModel.Kind("flower"), .flower)
        XCTAssertEqual(GrowthModel.Kind("cactus"), .tree, "未知は tree")
        XCTAssertEqual(GrowthModel.Kind(nil), .tree)
    }

    func testSlotsAreStableAndInside() {
        for kind in GrowthModel.Kind.allCases {
            let a = GrowthModel.slots(kind, count: 8)
            XCTAssertEqual(a.count, 8)
            XCTAssertEqual(a, GrowthModel.slots(kind, count: 8), "同じ数なら同じ位置(増えても前の実は動かない)")
            for p in a where kind != .plant {
                XCTAssertTrue((0...1).contains(p.x) && (0...1).contains(p.y), "\(kind) \(p)")
            }
        }
    }

    func testBodyGrowsWithProgress() {
        XCTAssertEqual(GrowthModel(value: 0, target: 100, step: nil).bodyScale, 0.55, accuracy: 1e-9)
        XCTAssertEqual(GrowthModel(value: 100, target: 100, step: nil).bodyScale, 1, accuracy: 1e-9)
        XCTAssertLessThan(GrowthModel(value: 20, target: 100, step: nil).bodyScale, GrowthModel(value: 40, target: 100, step: nil).bodyScale)
    }

    /// 開いている間は歩数を減らさない(取り直しの遅れで実が消えない)。日付が変わったら新しい値。
    func testLiveStepsNeverGoBackWithinADay() {
        XCTAssertEqual(GrowthLeaf.merged(current: 3214, next: 3200, sameDay: true), 3214)
        XCTAssertEqual(GrowthLeaf.merged(current: 3214, next: 3230, sameDay: true), 3230)
        XCTAssertEqual(GrowthLeaf.merged(current: 3214, next: 12, sameDay: false), 12)
        XCTAssertEqual(GrowthLeaf.merged(current: nil, next: 12, sameDay: true), 12)
    }

    /// 目標が無い(設定が空・0)時は「収穫!」にせず育てない(target 1 として扱うと 1 歩で収穫になっていた)。
    func testMissingTargetDoesNotHarvest() {
        let m = GrowthLeaf.model(raw: 3214, target: nil, step: 1000)
        XCTAssertFalse(m.isComplete)
        XCTAssertEqual(m.items, 0)
        XCTAssertEqual(GrowthLeaf.model(raw: 3214, target: 8000, step: 1000), GrowthModel(value: 3214, target: 8000, step: 1000))
    }

    /// 樹冠は円の和集合: 重なった所も 1 つの輪郭で、円の外は含まない。
    func testCanopyIsOneSilhouette() {
        let blobs: [CanopyShape.Blob] = [.init(x: 0.3, y: 0.5, r: 0.2), .init(x: 0.6, y: 0.5, r: 0.2)]
        let path = CanopyShape(blobs: blobs).path(in: CGRect(x: 0, y: 0, width: 100, height: 100))
        XCTAssertTrue(path.contains(CGPoint(x: 45, y: 50)), "重なり")
        XCTAssertTrue(path.contains(CGPoint(x: 15, y: 50)))
        XCTAssertFalse(path.contains(CGPoint(x: 45, y: 10)))
        XCTAssertEqual(path.boundingRect.minX, 10, accuracy: 0.5)
        XCTAssertEqual(path.boundingRect.maxX, 80, accuracy: 0.5)
    }

    /// 日本語の文言(シミュレータが英語でも ja で引く)
    @MainActor func testJapaneseWords() {
        let saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
        defer { LanguagePreference.shared.setting = saved }
        XCTAssertEqual(GrowthLeaf.itemName(.tree), "実")
        XCTAssertEqual(GrowthLeaf.itemName(.flower), "花")
        XCTAssertEqual(GrowthLeaf.completeText(.tree), "収穫!")
        XCTAssertEqual(L("目標を決めると育ちます"), "目標を決めると育ちます")
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(L("目標を決めると育ちます"), "Set a goal to make it grow")
    }

    func testNumberText() {
        XCTAssertEqual(GrowthLeaf.numberText(8000), 8000.formatted(.number.grouping(.automatic)))
        XCTAssertEqual(GrowthLeaf.numberText(.nan), "—")
    }

    // MARK: 登録・デコード

    @MainActor func testRegisteredAndDeclared() throws {
        XCTAssertNotNil(ComponentRegistry.standard.builder(for: "growth"))
        XCTAssertTrue(ClientCapabilities.allComponents.contains("growth"))
        XCTAssertFalse(ClientCapabilities.reviewExcludedComponents.contains(.growth))
        XCTAssertTrue(ClientCapabilities.macUnavailableComponents.contains(.growth), "Mac はまだ描かない(枠で受ける)")
        XCTAssertFalse(ClientCapabilities.allowedComponents(macOS: true, review: false).contains("growth"))
        XCTAssertTrue(ClientCapabilities.allowedComponents(macOS: false, review: false).contains("growth"))
        XCTAssertTrue(DSLExamples.names.contains("steps_tree"))
    }

    func testExampleDecodes() throws {
        let c = try XCTUnwrap(stepsTree().screens.first?.components.first)
        XCTAssertEqual(c.type, .growth)
        XCTAssertEqual(c.kind, "tree")
        XCTAssertEqual(c.metric, "steps")
        XCTAssertEqual(c.live, true)
        XCTAssertEqual(c.target, 8000)
        XCTAssertEqual(c.step, 1000)
        XCTAssertEqual(c.area, "A1:F10")
    }

    // MARK: 数値の出どころ

    @MainActor func testEntityAndExpressionSources() throws {
        let json = #"""
        {"schemaVersion":1,"app":{"name":"貯金","voiceEntry":"floating"},"initialScreen":"home",
         "entities":[{"id":"saving","name":"貯金","fields":[{"id":"amount","name":"金額","type":"currency"}]},
                     {"id":"settings","name":"設定","single":true,"fields":[{"id":"goal","name":"目標","type":"integer","defaultValue":100000},{"id":"count","name":"回数","type":"integer","defaultValue":3}]}],
         "screens":[{"id":"home","title":"h","components":[
           {"type":"growth","id":"sum","label":"貯金の木","entity":"saving","field":"amount","target":{"setting":"goal"},"unit":"円"},
           {"type":"growth","id":"n","label":"回数","kind":"flower","entity":"saving","aggregate":"count","target":5},
           {"type":"growth","id":"expr","label":"カウンタ","kind":"plant","value":{"setting":"count"},"target":10}]}]}
        """#
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let cs = try XCTUnwrap(dsl.screens.first?.components)
        let rt = ConformanceRuntime(dsl: dsl, store: ["saving": [
            Record(id: "a", data: ["amount": .number(30000)]), Record(id: "b", data: ["amount": .number(25000)]),
        ]], currentUserId: "me")
        XCTAssertEqual(GrowthLeaf.recordValue(cs[0], rt: rt), 55000)
        XCTAssertEqual(GrowthLeaf.target(cs[0], rt: rt), 100000, "目標は設定の既定値")
        XCTAssertEqual(GrowthLeaf.recordValue(cs[1], rt: rt), 2)
        XCTAssertEqual(GrowthLeaf.target(cs[1], rt: rt), 5)
        XCTAssertEqual(GrowthLeaf.recordValue(cs[2], rt: rt), 3)
        rt.store["settings"] = [Record(id: "single", data: ["goal": .number(50000), "count": .number(7)])]
        XCTAssertEqual(GrowthLeaf.target(cs[0], rt: rt), 50000)
        XCTAssertEqual(GrowthLeaf.recordValue(cs[2], rt: rt), 7)
        withExtendedLifetime(rt) {}
    }

    // MARK: 描画

    /// 3 種の絵 × 0・途中・育ち切り・許可なし・使えない が落ちずに描ける(ImageRenderer では .task は走らないので初期値で描く)。
    @MainActor func testRendersEveryKindAndState() throws {
        let base = try XCTUnwrap(stepsTree().screens.first?.components.first)
        for kind in ["tree", "flower", "plant"] {
            var c = base
            c.kind = kind
            let states: [(String, GrowthLeaf.StepsPhase)] = [
                ("loading", .loading), ("zero", .value(0)), ("mid", .value(3214)), ("done", .value(8000)),
                ("denied", .denied), ("failed", .failed("この端末ではヘルスデータを使えません")),
            ]
            for (name, phase) in states {
                let r = ImageRenderer(content: GrowthLeaf(c: c, steps: phase).frame(width: 360, height: 480))
                r.scale = 1
                let img = try XCTUnwrap(r.platformImage, "\(kind) \(name)")
                XCTAssertEqual(img.size.width, 360, "\(kind) \(name)")
            }
        }
    }

    /// 目で確かめる用: GROWTH_SNAPSHOT_DIR を渡すと PNG を書き出す(既定では書かない)。
    @MainActor func testWritesSnapshotsWhenAsked() throws {
        guard let dir = ProcessInfo.processInfo.environment["GROWTH_SNAPSHOT_DIR"] ?? ProcessInfo.processInfo.environment["SIMCTL_CHILD_GROWTH_SNAPSHOT_DIR"] else {
            throw XCTSkip("GROWTH_SNAPSHOT_DIR が無い")
        }
        let base = try XCTUnwrap(stepsTree().screens.first?.components.first)
        // 日本語で描く(「歩」「実」の崩れを見る)。GROWTH_SNAPSHOT_LANG=en で英語
        let lang = ProcessInfo.processInfo.environment["GROWTH_SNAPSHOT_LANG"] ?? ProcessInfo.processInfo.environment["SIMCTL_CHILD_GROWTH_SNAPSHOT_LANG"] ?? "ja"
        let saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = lang == "en" ? .en : .ja
        defer { LanguagePreference.shared.setting = saved }
        for kind in ["tree", "flower", "plant"] {
            for (name, v) in [("0", 0.0), ("3214", 3214), ("8000", 8000)] {
                var c = base
                c.kind = kind
                let view = GrowthLeaf(c: c, steps: .value(v)).frame(width: 360, height: 480).background(Color.white)
                    .environment(\.locale, LanguagePreference.shared.locale)
                let r = ImageRenderer(content: view)
                r.scale = 2
                #if os(iOS)
                let data = try XCTUnwrap(r.uiImage?.pngData())
                try data.write(to: URL(fileURLWithPath: dir).appending(path: "growth-\(kind)-\(name).png"))
                #endif
            }
        }
    }
}
