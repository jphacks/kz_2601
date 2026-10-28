import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer

@MainActor
@Observable
private final class DriveRuntime: DSLRuntime {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    var store: [String: [Record]]

    init(dsl: AppDSL, store: [String: [Record]]) {
        self.dsl = dsl
        self.store = store
    }

    func records(for entity: String) -> [Record] { store[entity] ?? [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
}

/// `routeGuide`(#411): 登録・読み上げの文・矢印・描画(ホームの候補から選ぶ形 / フォームの今のレコード / 固定の住所 / 目的地なし)。
final class RouteGuideLeafTests: XCTestCase {
    private func drive() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/drive.json"))
    }

    private let spots = [
        Record(id: "s1", data: ["name": "兼六園", "place": "36.562128,136.662652"]),
        Record(id: "s2", data: ["name": "近江町市場", "place": "36.571476,136.656650"]),
    ]

    override func tearDown() {
        LanguagePreference.shared.setting = .system
        super.tearDown()
    }

    @MainActor func testRegistered() throws {
        XCTAssertNotNil(ComponentRegistry.standard.builder(for: "routeGuide"))
        let json = #"{"type":"routeGuide","id":"g","entity":"spot","locationField":"place","titleField":"name","mode":"walking"}"#
        let c = try JSONDecoder().decode(Component.self, from: Data(json.utf8))
        XCTAssertEqual(c.type, .routeGuide)
        XCTAssertTrue(c.rendersOwnSurface, "section の中で二重カードにしない")
    }

    @MainActor func testSpokenTextJapanese() {
        LanguagePreference.shared.setting = .ja
        let next = RouteGuideSession.text(.next(distance: 320, instruction: "右折して百万石通りに入る"))
        XCTAssertTrue(next.hasPrefix("300"), next)
        XCTAssertTrue(next.hasSuffix("メートル先、右折して百万石通りに入る"), next)
        XCTAssertTrue(RouteGuideSession.text(.approach(distance: 1_240, instruction: nil)).contains("1.2"))
        XCTAssertTrue(RouteGuideSession.text(.approach(distance: 1_240, instruction: nil)).hasSuffix("キロメートル先、目的地です"))
        XCTAssertEqual(RouteGuideSession.text(.now(instruction: "左折")), "まもなく、左折")
        XCTAssertEqual(RouteGuideSession.text(.now(instruction: nil)), "まもなく目的地です")
        XCTAssertEqual(RouteGuideSession.text(.arrived), "目的地に到着しました。案内を終了します")
    }

    @MainActor func testSpokenTextEnglish() {
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(RouteGuideSession.text(.next(distance: 320, instruction: "Turn right onto Main St")), "In 300 meters, Turn right onto Main St")
        XCTAssertEqual(RouteGuideSession.text(.arrived), "You have arrived. Ending guidance.")
        XCTAssertEqual(RouteGuideLeaf.duration(125 * 60), "2 h 5 min left")
    }

    func testManeuverIcon() {
        XCTAssertEqual(RouteGuideLeaf.maneuverIcon("右折して国道8号線に入る"), "arrow.turn.up.right")
        XCTAssertEqual(RouteGuideLeaf.maneuverIcon("Turn left onto Main St"), "arrow.turn.up.left")
        XCTAssertEqual(RouteGuideLeaf.maneuverIcon("Uターンする"), "arrow.uturn.down")
        XCTAssertEqual(RouteGuideLeaf.maneuverIcon("目的地は右側です"), "arrow.turn.up.right")
        XCTAssertEqual(RouteGuideLeaf.maneuverIcon("国道8号線を進む"), "arrow.up")
        XCTAssertEqual(RouteGuideLeaf.maneuverIcon(nil), "flag.checkered")
    }

    /// 実際に SwiftUI で描いて落ちないこと・大きさが出ること(地図は MapKit のまま)。
    @MainActor func testRendersEachDestinationKind() throws {
        let dsl = try drive()
        let rt = DriveRuntime(dsl: dsl, store: ["spot": spots])
        let home = try XCTUnwrap(dsl.screen("home")?.components.first { $0.type == .routeGuide })
        let formScreen = try XCTUnwrap(dsl.screen("spot_form"))
        let onForm = try XCTUnwrap(formScreen.components.first { $0.type == .routeGuide })
        let draft = FormDraft(entity: try XCTUnwrap(dsl.entity("spot")), record: spots[0])
        let fixed = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"routeGuide","address":"金沢駅","mode":"walking"}"#.utf8))
        let broken = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"routeGuide","locationField":"place"}"#.utf8))

        let cases: [(String, Component, RenderContext)] = [
            ("home", home, RenderContext(route: Route(screen: "home"))),
            ("form", onForm, RenderContext(route: Route(screen: "spot_form", recordId: "s1"), form: draft)),
            ("address", fixed, RenderContext(route: Route(screen: "home"))),
            ("no destination", broken, RenderContext(route: Route(screen: "home"))),
        ]
        for (name, c, ctx) in cases {
            let host = UIHostingController(rootView: RouteGuideLeaf(c: c, ctx: ctx)
                .environment(\.dslRuntime, rt)
                .frame(width: 360, height: 420))
            let window = UIWindow(frame: CGRect(x: 0, y: 0, width: 390, height: 844))
            window.rootViewController = host
            window.makeKeyAndVisible()
            host.view.setNeedsLayout()
            host.view.layoutIfNeeded()
            let size = host.sizeThatFits(in: CGSize(width: 360, height: 420))
            XCTAssertGreaterThan(size.width, 0, name)
            XCTAssertGreaterThan(size.height, 0, name)
            window.isHidden = true
        }
    }
}
