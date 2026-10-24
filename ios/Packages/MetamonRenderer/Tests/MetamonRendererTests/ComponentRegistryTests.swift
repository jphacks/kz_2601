import XCTest
import SwiftUI
import MetamonDSL
@testable import MetamonRenderer

/// 「崩れない仕組み」: schema(docs/dsl/dsl.schema.json)の Leaf/Container 全 `type` が
/// `ComponentRegistry.standard` と `ComponentType` の両方に存在することを強制する。抜けたら赤。
final class ComponentRegistryTests: XCTestCase {
    /// このファイル → MetamonRendererTests → Tests → MetamonRenderer → Packages → ios → リポジトリ root
    private static let repoRoot: URL = {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return url
    }()

    /// schema の `$defs.Container.properties.type.enum` + `$defs.Leaf.anyOf[].$ref` 先の `properties.type.const`。
    private func schemaComponentTypes() throws -> Set<String> {
        let data = try Data(contentsOf: Self.repoRoot.appending(path: "docs/dsl/dsl.schema.json"))
        let root = try XCTUnwrap(try JSONSerialization.jsonObject(with: data) as? [String: Any])
        let defs = try XCTUnwrap(root["$defs"] as? [String: Any])

        var types = Set<String>()
        let container = try XCTUnwrap(defs["Container"] as? [String: Any])
        let containerType = try XCTUnwrap((container["properties"] as? [String: Any])?["type"] as? [String: Any])
        types.formUnion(try XCTUnwrap(containerType["enum"] as? [String]))

        let leaf = try XCTUnwrap(defs["Leaf"] as? [String: Any])
        for branch in try XCTUnwrap(leaf["anyOf"] as? [[String: Any]]) {
            let ref = try XCTUnwrap(branch["$ref"] as? String)
            let name = String(ref.split(separator: "/").last!)
            let def = try XCTUnwrap(defs[name] as? [String: Any], "missing $defs.\(name)")
            let typeProp = try XCTUnwrap((def["properties"] as? [String: Any])?["type"] as? [String: Any], "\(name).properties.type")
            types.insert(try XCTUnwrap(typeProp["const"] as? String, "\(name).type.const"))
        }
        XCTAssertGreaterThanOrEqual(types.count, 18, "schema から type を拾えていない")
        return types
    }

    func testEverySchemaTypeIsRegistered() throws {
        let schema = try schemaComponentTypes()
        let registered = ComponentRegistry.standard.registeredTypes
        XCTAssertEqual(schema.subtracting(registered), [], "schema にあるのに ComponentRegistry.standard に無い type")
        XCTAssertEqual(registered.subtracting(schema), [], "ComponentRegistry.standard にあるのに schema に無い type(schema を更新するか登録を消す)")
    }

    func testEverySchemaTypeHasEnumCase() throws {
        let schema = try schemaComponentTypes()
        let cases = Set(ComponentType.allCases.map(\.rawValue)).subtracting(["unknown"])
        XCTAssertEqual(schema.subtracting(cases), [], "schema にあるのに ComponentType に無い case")
        XCTAssertEqual(cases.subtracting(schema), [], "ComponentType にあるのに schema に無い case")
    }

    @MainActor func testRegistryOverrideAndLookup() throws {
        let base = ComponentRegistry.standard
        XCTAssertNotNil(base.builder(for: "text"))
        XCTAssertNil(base.builder(for: "hologram"))

        let custom = base.registering(.voiceInput) { _, _ in Text("host") }
        XCTAssertEqual(custom.registeredTypes, base.registeredTypes)
        XCTAssertNotNil(custom.builder(for: ComponentType.voiceInput.rawValue))

        var extended = base
        extended.register("hologram") { _, _ in Text("x") }
        XCTAssertNotNil(extended.builder(for: "hologram"))
        XCTAssertNil(base.builder(for: "hologram"), "value semantics: 元のレジストリは変わらない")
    }

    func testRouteAndContext() {
        let r = Route(screen: "home", recordId: "1", stack: Route.tabStack("home"))
        XCTAssertEqual(r.id, "tab:home/home/1")
        XCTAssertEqual(Route(screen: "a").stack, "main")
        let ctx = RenderContext(route: r).actionContext(recordId: "9")
        XCTAssertEqual(ctx.recordId, "9")
        XCTAssertEqual(ctx.route, r)
    }

    func testColorHex() {
        XCTAssertNotNil(Color(hex: "#FF8800"))
        XCTAssertNotNil(Color(hex: "FF880080"))
        XCTAssertNil(Color(hex: "#FFF"))
        XCTAssertNil(Color(hex: nil))
    }
}
