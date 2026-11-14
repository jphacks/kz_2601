#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
import MetamonRenderer
@testable import Metamon

/// 近距離共有(#36)もすれ違い通信(#425)の「名前を出さない」に従う: MCPeerID と Header の送り主が「近くの人」になる。
@MainActor
final class NearbyShareAnonymousTests: XCTestCase {
    private final class FakeTransport: PassbyTransport {
        var onPayload: ((NearbyPassby.Payload) -> Void)?
        var onPeers: (([PassbyPeer]) -> Void)?
        var onNotice: ((String?) -> Void)?
        var distanceSupport: PassbyDistanceSupport { .unsupported("x") }
        func start(_ start: PassbyStart) {}
        func stop() {}
    }

    private let json = """
    {"schemaVersion":1,"app":{"name":"出席","voiceEntry":"none"},"initialScreen":"home",
     "entities":[{"id":"members","name":"メンバー","fields":[{"id":"name","name":"名前","type":"string","required":true}]}],
     "screens":[{"id":"home","title":"出席","components":[
       {"type":"nearbyShare","id":"ns","label":"近くの人に送る","entity":"members"}]}]}
    """

    func testAnonymousSettingAppliesToPeerNameAndHeader() async throws {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let rt = AppRuntime(document: AppDocument(appId: "nearby-anon-\(UUID().uuidString)", version: 1, dsl: dsl),
                            services: BuildConfig.make(environment: .mock))
        let suite = "nearby-anon-\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        let savedController = NearbyPassbyHost.controller
        let savedService = NearbyUI.shared.service
        defer {
            defaults.removePersistentDomain(forName: suite)
            NearbyPassbyHost.controller = savedController
            if let savedService { NearbyUI.shared.install(savedService) }
            NearbyUI.shared.request = nil
        }
        let c = PassbyController(transport: FakeTransport(), defaults: defaults)
        c.runtime = rt
        NearbyPassbyHost.controller = c
        let realName = NearbyLimits.peerName(rt.nearbyDisplayName, anonymous: false)
        XCTAssertNotEqual(realName, NearbyLimits.anonymousName)

        // オン: 待ち受けの名前(MCPeerID)と Header の送り主が「近くの人」
        c.setAnonymous(true)
        XCTAssertEqual(rt.nearbyPublicName, NearbyLimits.anonymousName)
        let service = NearbyService(displayName: rt.nearbyPublicName)
        NearbyUI.shared.install(service)
        XCTAssertEqual(service.myName, NearbyLimits.anonymousName)
        _ = await rt.createRecord(entity: "members", data: ["name": "太郎"])
        rt.shareNearby(entity: "members", records: NearbyShare.recordsToSend(from: rt.records(for: "members")))
        XCTAssertEqual(NearbyUI.shared.request?.payload.header.sender, NearbyLimits.anonymousName)

        // オフ: 切り替えると近距離共有の名前も本名に戻る(設定はすれ違い通信と同じ 1 つ)
        c.setAnonymous(false)
        XCTAssertFalse(defaults.bool(forKey: "passby.anonymous"))
        XCTAssertEqual(service.myName, realName, "切り替えたら MCPeerID を作り直す")
        rt.shareNearby(entity: "members", records: NearbyShare.recordsToSend(from: rt.records(for: "members")))
        XCTAssertEqual(NearbyUI.shared.request?.payload.header.sender, realName)

        // 再びオン: 同じ設定キーに保存され、名前も「近くの人」へ
        c.setAnonymous(true)
        XCTAssertTrue(defaults.bool(forKey: "passby.anonymous"))
        XCTAssertEqual(service.myName, NearbyLimits.anonymousName)
    }

    /// 送るシートの「Android とはつながりません」に英訳がある(Android の nearby_ios_not_supported と対)
    func testSendSheetAndroidNoteHasEnglish() throws {
        let path = try XCTUnwrap(Bundle(for: AppRuntime.self).path(forResource: "en", ofType: "lproj"))
        let en = try XCTUnwrap(Bundle(path: path))
        XCTAssertEqual(en.localizedString(forKey: "Android の端末とはつながりません(iPhone どうしで送り合えます)", value: nil, table: nil),
                       "Can't connect to Android devices (works between iPhones)")
    }
}
#endif
