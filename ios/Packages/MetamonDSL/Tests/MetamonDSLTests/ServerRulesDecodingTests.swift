import XCTest
@testable import MetamonDSL

/// #340: サーバーの自動の動き(トップの serverRules)は iOS では描画しない。キーがあっても今までどおりデコードできること。
final class ServerRulesDecodingTests: XCTestCase {
    /// backend の Validate を通る例(backend/pkg/dsl/testdata/server_rules.json)をそのまま読む。
    func testBackendExampleDecodes() throws {
        let data = try Data(contentsOf: ExamplesFixture.repoRoot.appending(path: "backend/pkg/dsl/testdata/server_rules.json"))
        let dsl = try JSONDecoder().decode(AppDSL.self, from: data)
        XCTAssertEqual(dsl.entities.map(\.id), ["events", "rsvps", "messages", "log"])
        XCTAssertEqual(dsl.screens.map(\.id), ["home"])
        XCTAssertEqual(dsl.initialScreen, "home")
        XCTAssertGreaterThan(dsl.serverRuleCount, 0) // #347: 設定の「自動の動き」を出す
    }

    /// 封筒(AppDocument)の中にあっても落ちない・未知の形でも無視する。
    func testUnknownServerRulesShapeIsIgnored() throws {
        let json = #"""
        {"appId":"a","version":3,"dsl":{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"home","entities":[],
         "screens":[{"id":"home","title":"x","components":[{"type":"text","text":"x"}]}],
         "serverRules":[{"id":"r","on":{"record":"created","entity":"e"},"do":[{"future":{"x":1}}]}]}}
        """#
        let doc = try JSONDecoder().decode(AppDocument.self, from: Data(json.utf8))
        XCTAssertEqual(doc.version, 3)
        XCTAssertEqual(doc.dsl.screens.count, 1)
        XCTAssertEqual(doc.dsl.serverRuleCount, 1)
        XCTAssertEqual(AppDSL.fallback.serverRuleCount, 0)
    }
}
