import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #1323: Mac の部品の「この部品について話す」の文脈(どの画面のどの部品か)と、どの部品に右クリック・VoiceOver の操作を出すか。
final class ComponentTalkTests: XCTestCase {
    private func dsl() throws -> AppDSL {
        let json = """
        {"schemaVersion":1,"app":{"name":"買い物"},"initialScreen":"home","entities":[],
         "screens":[{"id":"home","title":"買い物","components":[]},{"id":"detail","title":"  ","components":[]}]}
        """
        return try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    private func components(_ json: String) throws -> [Component] {
        try JSONDecoder().decode([Component].self, from: Data(json.utf8))
    }

    func testTargetNamesScreenAndComponent() throws {
        let c = try components(#"[{"id":"items","type":"list","label":"買うもの"}]"#)[0]
        let target = ComponentTalk.target(c, route: Route(screen: "home"))
        XCTAssertEqual(target.screen, "home")
        XCTAssertEqual(target.component, AIButtonComponent(id: "items", label: "買うもの"))
        XCTAssertEqual(target.screenTitle(in: try dsl()), "買い物")
        XCTAssertFalse(target.isInModal)
    }

    func testComponentLabelFallsBackToTitleThenType() throws {
        let cs = try components(#"[{"id":"a","type":"section","title":"今週"},{"id":"b","type":"chart"}]"#)
        XCTAssertEqual(cs.map { ComponentTalk.target($0, route: Route(screen: "home")).component.label }, ["今週", "chart"])
    }

    func testScreenTitleFallsBackToId() throws {
        let c = try components(#"[{"id":"t","type":"text","text":"x"}]"#)[0]
        let dsl = try dsl()
        // title が空白だけ・画面が見つからない時は id
        XCTAssertEqual(ComponentTalk.target(c, route: Route(screen: "detail")).screenTitle(in: dsl), "detail")
        XCTAssertEqual(ComponentTalk.target(c, route: Route(screen: "gone")).screenTitle(in: dsl), "gone")
    }

    func testModalStacks() throws {
        let c = try components(#"[{"id":"t","type":"text","text":"x"}]"#)[0]
        XCTAssertTrue(ComponentTalk.target(c, route: Route(screen: "detail", stack: "sheet")).isInModal)
        XCTAssertTrue(ComponentTalk.target(c, route: Route(screen: "detail", stack: "full")).isInModal)
        XCTAssertFalse(ComponentTalk.target(c, route: Route(screen: "detail", stack: Route.tabStack("home"))).isInModal)
    }

    func testWhichComponentsOfferMenuAndAction() {
        // 表示・ボタン・一覧などの部品は右クリックと VoiceOver の両方
        for type in [ComponentType.text, .button, .list, .qrCode, .chart, .image, .toggle, .chatThread, .unknown] {
            XCTAssertTrue(ComponentTalk.offersMenu(for: type), "\(type)")
            XCTAssertTrue(ComponentTalk.offersAction(for: type), "\(type)")
        }
        // 文字の入力欄は右クリックを OS の標準に残し、VoiceOver の操作だけ
        for type in [ComponentType.textField, .numberField, .textEditor, .secureField, .searchBar, .transcribeField] {
            XCTAssertFalse(ComponentTalk.offersMenu(for: type), "\(type)")
            XCTAssertTrue(ComponentTalk.offersAction(for: type), "\(type)")
        }
        // 入れ物・余白・区切り線は出さない(中の部品と重ねない)
        for type in [ComponentType.vstack, .hstack, .section, .card, .disclosure, .carousel, .grid, .spacer, .divider] {
            XCTAssertFalse(ComponentTalk.offersMenu(for: type), "\(type)")
            XCTAssertFalse(ComponentTalk.offersAction(for: type), "\(type)")
        }
    }
}
