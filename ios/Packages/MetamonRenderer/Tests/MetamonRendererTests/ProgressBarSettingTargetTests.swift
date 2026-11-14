import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// progressBar の target に `{"setting": …}`: 利用者が変える目標(1 日の歩数の目標)でバーが動く。目標が空・0 以下は「—」で 0 で割らない。
final class ProgressBarSettingTargetTests: XCTestCase {
    private let json = #"""
    {"schemaVersion":1,"app":{"name":"歩数","voiceEntry":"floating"},"initialScreen":"home",
     "entities":[{"id":"walk","name":"歩いた記録","fields":[{"id":"steps","name":"歩数","type":"integer"}]},
                 {"id":"settings","name":"設定","single":true,"fields":[{"id":"daily_goal","name":"目標","type":"integer","defaultValue":8000}]}],
     "screens":[{"id":"home","title":"h","components":[
       {"type":"progressBar","label":"今日の歩数","entity":"walk","field":"steps","target":{"setting":"daily_goal"},"tone":"positive"}]}]}
    """#

    @MainActor func testSettingTargetFollowsTheGoal() throws {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let bar = try XCTUnwrap(dsl.screens.first?.components.first)
        XCTAssertNil(bar.target)
        XCTAssertEqual(bar.targetExpr, .setting("daily_goal"))

        let rt = ConformanceRuntime(dsl: dsl, store: ["walk": [Record(id: "w1", data: ["steps": .number(4000)])]], currentUserId: "me")
        // 保存前は defaultValue 8000
        XCTAssertEqual(ProgressBarLeaf.ratio(bar, rt: rt), 0.5, accuracy: 1e-9)
        XCTAssertFalse(ProgressBarLeaf.missingTarget(bar, rt: rt))
        // 目標を 5000 に変えるとバーが動く
        rt.store["settings"] = [Record(id: "single", data: ["daily_goal": .number(5000)])]
        XCTAssertEqual(ProgressBarLeaf.ratio(bar, rt: rt), 0.8, accuracy: 1e-9)
        // 0 にされたら「—」・比率は 0(0 で割らない)
        rt.store["settings"] = [Record(id: "single", data: ["daily_goal": .number(0)])]
        XCTAssertEqual(ProgressBarLeaf.ratio(bar, rt: rt), 0)
        XCTAssertTrue(ProgressBarLeaf.missingTarget(bar, rt: rt))
        withExtendedLifetime(rt) {}
    }

    func testNumberTargetStaysLiteral() throws {
        let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"progressBar","entity":"walk","target":10}"#.utf8))
        XCTAssertEqual(c.target, 10)
        XCTAssertNil(c.targetExpr)
    }
}
