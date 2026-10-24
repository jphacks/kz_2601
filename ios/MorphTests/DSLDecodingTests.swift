import XCTest
@testable import Morph

final class DSLDecodingTests: XCTestCase {
    func testBundledSamplesDecode() throws {
        let initial = try AppDocument.bundled("app_initial")
        XCTAssertTrue(initial.isVoiceOnly)

        let kakeibo = try AppDocument.bundled("app_kakeibo")
        XCTAssertEqual(kakeibo.navigation.type, .tab)
        XCTAssertEqual(kakeibo.navigation.tabs.count, 2)
        XCTAssertNotNil(kakeibo.screens["home"])
        XCTAssertFalse(kakeibo.isVoiceOnly)
    }

    func testUnknownComponentIsIgnoredNotFatal() throws {
        let json = #"{"type":"vstack","children":[{"type":"text","text":"a"},{"type":"hologram"},"garbage",{"type":"text"}]}"#
        let node = try JSONDecoder().decode(Node.self, from: Data(json.utf8))
        guard case .stack(let props) = node.component else { return XCTFail("expected stack") }
        // text / unknown / (garbage は skip) / invalid text(props不足)
        XCTAssertEqual(props.children.count, 3)
        guard case .unknown(let type) = props.children[1].component else { return XCTFail("expected unknown") }
        XCTAssertEqual(type, "hologram")
        guard case .invalid(let badType, _) = props.children[2].component else { return XCTFail("expected invalid") }
        XCTAssertEqual(badType, "text")
    }

    func testUnknownActionAndLenientEnum() throws {
        let json = #"{"type":"button","label":"x","style":"neon","action":{"type":"teleport"}}"#
        let node = try JSONDecoder().decode(Node.self, from: Data(json.utf8))
        guard case .button(let props) = node.component else { return XCTFail("expected button") }
        XCTAssertEqual(props.style, .primary) // 未知 style は fallback
        guard case .unknown(let type)? = props.action else { return XCTFail("expected unknown action") }
        XCTAssertEqual(type, "teleport")
    }

    func testInterpolation() {
        let state: JSONValue = ["todos": [["title": "a"], ["title": "b"]], "name": "Kota"]
        let scope = Interpolator.Scope(state: state, user: .null, ctx: RenderContext(params: ["id": "42"]))

        XCTAssertEqual(Interpolator.resolve("{{state.todos.count}}", in: scope), .number(2))
        XCTAssertEqual(Interpolator.resolve("{{todos.1.title}}", in: scope), .string("b"))
        XCTAssertEqual(Interpolator.resolve("こんにちは {{state.name}} さん ({{params.id}})", in: scope), .string("こんにちは Kota さん (42)"))
        XCTAssertEqual(Interpolator.resolve("plain", in: scope), .string("plain"))
        XCTAssertEqual(Interpolator.resolve("{{missing}}", in: scope), .null)

        let itemScope = Interpolator.Scope(state: state, user: .null, ctx: RenderContext().with(item: ["id": "x"], index: 3))
        XCTAssertEqual(Interpolator.resolve("{{item.id}}-{{index}}", in: itemScope), .string("x-3"))
    }

    func testJSONValueSetPath() {
        var v: JSONValue = .object([:])
        v.set(path: "draft.title", to: "hello")
        v.set(path: "list.1", to: 5)
        XCTAssertEqual(v[path: "draft.title"], "hello")
        XCTAssertEqual(v[path: "list.1"], 5)
        XCTAssertEqual(v[path: "list.0"], .null)
    }
}
