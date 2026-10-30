#if !APP_STORE_REVIEW
import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer

@MainActor
@Observable
private final class PassbyRuntime: DSLRuntime {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    init(dsl: AppDSL) { self.dsl = dsl }
    func records(for entity: String) -> [Record] { [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
}

/// すれ違い通信(#425): nearbyShare mode:passby が専用の表示になり、表示中だけホストに attach する。
final class NearbyPassbyRenderTests: XCTestCase {
    private let json = """
    {"schemaVersion":1,"app":{"name":"すれ違い","voiceEntry":"none"},"initialScreen":"home",
     "entities":[{"id":"profile","name":"プロフィール","fields":[{"id":"name","name":"名前","type":"string"},{"id":"hobby","name":"趣味","type":"string"}]}],
     "screens":[{"id":"home","title":"すれ違い","components":[
       {"type":"nearbyShare","id":"pb","label":"すれ違い通信","entity":"profile","mode":"passby","fields":["name","hobby"],"distance":true}]}]}
    """

    @MainActor func testPassbyRendersAndAttachesWhileVisible() throws {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let c = try XCTUnwrap(dsl.screens.first?.components.first)
        let config = try XCTUnwrap(NearbyPassby.Config(c))
        let host = StaticNearbyPassbyController()
        host.agree()
        host.setOptIn(true, config: config)
        host.distanceSupport = .supported
        host.peers = [PassbyPeer(id: "s2", name: "花子", distance: 1.2, azimuth: 0.3)]
        let met = Date(timeIntervalSince1970: 1_800_000_000)
        host.encounters = [.init(id: "s2", name: "花子", firstMet: met, lastMet: met, times: 1, records: 1)]
        let previous = NearbyPassbyHost.controller
        NearbyPassbyHost.controller = host
        defer { NearbyPassbyHost.controller = previous }

        let view = NearbyShareLeaf(c: c, ctx: RenderContext(route: Route(screen: "home", stack: "main")))
            .environment(\.dslRuntime, PassbyRuntime(dsl: dsl))
            .environment(\.scenePhase, .active)
            .frame(width: 360, height: 480)
        let window = UIWindow(frame: CGRect(x: 0, y: 0, width: 360, height: 480))
        let vc = UIHostingController(rootView: view)
        window.rootViewController = vc
        window.makeKeyAndVisible()
        vc.view.layoutIfNeeded()

        let attached = expectation(description: "attach")
        Task { @MainActor in
            for _ in 0..<50 where host.attached == nil { try? await Task.sleep(for: .milliseconds(20)) }
            attached.fulfill()
        }
        wait(for: [attached], timeout: 3)
        XCTAssertEqual(host.attached, "pb", "表示中は探す")
        XCTAssertTrue(host.isAdvertising)
        XCTAssertGreaterThan(vc.sizeThatFits(in: CGSize(width: 360, height: 480)).height, 0)

        window.rootViewController = nil
        window.isHidden = true
        let detached = expectation(description: "detach")
        Task { @MainActor in
            for _ in 0..<50 where host.attached != nil { try? await Task.sleep(for: .milliseconds(20)) }
            detached.fulfill()
        }
        wait(for: [detached], timeout: 3)
        XCTAssertNil(host.attached, "画面を離れたら止める")
        XCTAssertFalse(host.isAdvertising)
    }

    @MainActor func testStaticControllerNeedsOptInAndAttach() throws {
        let config = NearbyPassby.Config(componentId: "pb", entityId: "profile", label: "", fields: ["name"])
        let host = StaticNearbyPassbyController()
        host.attach(config)
        XCTAssertFalse(host.isAdvertising, "オンにするまで探さない")
        host.setOptIn(true, config: config)
        XCTAssertTrue(host.isAdvertising)
        host.detach(NearbyPassby.Config(componentId: "other", entityId: "profile", label: "", fields: []))
        XCTAssertTrue(host.isAdvertising, "別の部品の detach では止めない")
        host.detach(config)
        XCTAssertFalse(host.isAdvertising)
    }
}
#endif
