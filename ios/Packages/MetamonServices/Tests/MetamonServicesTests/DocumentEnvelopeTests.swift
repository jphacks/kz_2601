import XCTest
import MetamonDSL
@testable import MetamonServices

/// #1285 Mac の複製: 封筒を新しい appId・版 1・名前で包み直す(画面の定義はそのまま)。
final class DocumentEnvelopeTests: XCTestCase {
    private let dsl = Data(#"""
    {"schemaVersion":1,"app":{"name":"家計簿","voiceEntry":"none"},"initialScreen":"home","entities":[],
     "screens":[{"id":"home","title":"ホーム","components":[]}]}
    """#.utf8)

    func testDuplicatedHasNewIdVersionAndName() throws {
        let source = try XCTUnwrap(DocumentEnvelope.make(appId: "a", version: 7, dsl: dsl))
        let copy = try XCTUnwrap(DocumentEnvelope.duplicated(source, appId: "b", name: "家計簿のコピー"))
        let doc = try AppDocument.decode(copy)
        XCTAssertEqual(doc.appId, "b")
        XCTAssertEqual(doc.version, 1)
        XCTAssertEqual(doc.dsl.app.name, "家計簿のコピー")
        XCTAssertEqual(doc.dsl.screens.map(\.id), ["home"])
        XCTAssertEqual(try AppDocument.decode(source).appId, "a", "元の封筒は変えない")
    }

    func testDuplicatedRejectsBrokenEnvelope() {
        XCTAssertNil(DocumentEnvelope.duplicated(Data("not json".utf8), appId: "b", name: "x"))
    }
}
