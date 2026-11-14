#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// 近くの人に送る(#964): Mac も MultipeerConnectivity で送り合い、すれ違い通信(passby)は「この Mac では使えません」。
/// 通信はしない(待ち受けの判定・送るデータ・取り込み・申告・Info.plist を確かめる)。
@MainActor
final class MacNearbyTests: XCTestCase {
    private let json = """
    {"schemaVersion":1,"app":{"name":"名刺","voiceEntry":"none"},"initialScreen":"home",
     "entities":[{"id":"card","name":"名刺","fields":[{"id":"name","name":"名前","type":"string"},{"id":"age","name":"年齢","type":"integer"}]}],
     "screens":[{"id":"home","title":"名刺","components":[
       {"type":"nearbyShare","id":"send","label":"近くの人に送る","entity":"card"},
       {"type":"nearbyShare","id":"pb","label":"すれ違い通信","entity":"card","mode":"passby","fields":["name"]}]}]}
    """

    private func dsl(_ text: String? = nil) throws -> AppDSL {
        try JSONDecoder().decode(AppDSL.self, from: Data((text ?? json).utf8))
    }

    private func runtime(_ dsl: AppDSL, appId: String = "test-mac-nearby-\(UUID().uuidString.lowercased())") -> MacAppRuntime {
        MacAppRuntime(appId: appId, dsl: dsl, ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    override func tearDown() {
        MacNearby.shared.request = nil
        super.tearDown()
    }

    // MARK: - 申告と描き分け

    /// nearbyShare は Mac でも作らせる(申告から外さない・Renderer は本物の部品で受ける)
    func testNearbyShareStaysAllowedOnMac() {
        XCTAssertFalse(ClientCapabilities.macUnavailableComponents.contains(.nearbyShare))
        XCTAssertTrue(ClientCapabilities.allowedComponents?.contains(ComponentType.nearbyShare.rawValue) == true)
        XCTAssertFalse(ComponentRegistry.standard.unavailableTypes.contains(ComponentType.nearbyShare.rawValue))
        XCTAssertNotNil(ComponentRegistry.standard.builder(for: ComponentType.nearbyShare.rawValue))
    }

    /// 送る方はボタン、すれ違い通信は「この Mac では使えません」の枠
    func testPassbyIsUnavailableOnMac() throws {
        let components = try dsl().screens[0].components
        XCTAssertEqual(NearbyShareLeaf.kind(of: components[0]), .send)
        XCTAssertEqual(NearbyShareLeaf.kind(of: components[1]), .unavailable(feature: "すれ違い通信"))
    }

    /// 頼み方のコツは「近くの人に送る」を隠さない(ヘルスケア・道のりの記録は隠したまま)
    func testPromptTipsNoLongerHideNearby() {
        XCTAssertFalse(PromptTipsList.unavailableOnMac.contains(.nearby))
        XCTAssertTrue(PromptTipsList.unavailableOnMac.contains(.health))
    }

    // MARK: - Info.plist・サービス名

    func testInfoPlistDeclaresLocalNetworkAndBonjour() throws {
        let usage = try XCTUnwrap(Bundle.main.object(forInfoDictionaryKey: "NSLocalNetworkUsageDescription") as? String)
        XCTAssertFalse(usage.isEmpty)
        let services = try XCTUnwrap(Bundle.main.object(forInfoDictionaryKey: "NSBonjourServices") as? [String])
        XCTAssertEqual(Set(services), ["_\(NearbyService.serviceType)._tcp", "_\(NearbyService.serviceType)._udp"])
        XCTAssertFalse(services.contains { $0.hasPrefix("_tailorit-pb") }, "すれ違い通信のサービスは Mac では使わない")
    }

    // MARK: - 待ち受け

    func testAdvertisesOnlyWhileActiveAndAppUsesNearby() throws {
        let nearbyApp = try dsl()
        XCTAssertTrue(nearbyApp.usesNearbyReceive)
        XCTAssertTrue(MacNearby.shouldAdvertise(appIsActive: true, dsl: nearbyApp))
        XCTAssertFalse(MacNearby.shouldAdvertise(appIsActive: false, dsl: nearbyApp), "裏に回ったら待ち受けない")
        XCTAssertFalse(MacNearby.shouldAdvertise(appIsActive: true, dsl: nil), "前面のウインドウが無い")

        let passbyOnly = try dsl(json.replacingOccurrences(of: #"{"type":"nearbyShare","id":"send","label":"近くの人に送る","entity":"card"},"#, with: ""))
        XCTAssertFalse(passbyOnly.usesNearbyReceive, "すれ違い通信だけのアプリでは待ち受けない")
        XCTAssertFalse(MacNearby.shouldAdvertise(appIsActive: true, dsl: passbyOnly))
    }

    func testPublicNameFollowsAnonymousSetting() {
        let defaults = UserDefaults(suiteName: "mac-nearby-\(UUID().uuidString)")!
        let nearby = MacNearby(defaults: defaults)
        nearby.signedInName = { "山田" }
        XCTAssertEqual(nearby.publicName, "山田")
        nearby.isAnonymous = true
        XCTAssertEqual(nearby.publicName, NearbyLimits.anonymousName)
        XCTAssertTrue(defaults.bool(forKey: MacNearby.anonymousKey), "iOS と同じキーに残す")
        nearby.signedInName = { nil }
        nearby.isAnonymous = false
        XCTAssertFalse(nearby.publicName.isEmpty, "サインインしていなければ Mac の名前")
    }

    // MARK: - 送る

    func testShareNearbyOpensSendSheetForThisWindow() throws {
        let rt = runtime(try dsl())
        XCTAssertNotNil(rt.nearby, "Mac のランタイムも近距離共有のホスト")
        rt.shareNearby(entity: "card", records: [["name": .string("花子"), "age": .number(20), "extra": .string("x")]])
        let req = try XCTUnwrap(MacNearby.shared.request)
        XCTAssertEqual(req.owner, ObjectIdentifier(rt))
        XCTAssertEqual(req.payload.header.count, 1)
        XCTAssertEqual(req.payload.header.entityId, "card")
        XCTAssertEqual(req.payload.records.first?["extra"], nil, "保存しない項目は送らない")
    }

    func testShareNearbyWithNothingToSendShowsToast() throws {
        let rt = runtime(try dsl())
        rt.shareNearby(entity: "card", records: [])
        XCTAssertNil(MacNearby.shared.request)
        XCTAssertEqual(rt.toast, L10n.tr("送る記録がありません"))
    }

    // MARK: - 受け取る

    func testImportAddsRecordsFromSameApp() async throws {
        let app = try dsl()
        let rt = runtime(app, appId: "test-mac-nearby-import")
        let entity = try XCTUnwrap(app.entity("card"))
        let payload = NearbyShare.makePayload(sender: "花子", appId: "test-mac-nearby-import", appName: "名刺", entity: entity,
                                              records: [["name": .string("太郎"), "age": .number(30)], ["name": .string("次郎")]])
        let ack = await rt.importNearby(payload)
        XCTAssertEqual(ack, .init(ok: true, added: 2))
        XCTAssertEqual(Set(rt.records(for: "card").compactMap { $0.data["name"]?.stringValue }), ["太郎", "次郎"])
        XCTAssertNotNil(rt.toast)
    }

    func testImportRejectsUnknownEntity() async throws {
        let rt = runtime(try dsl())
        let other = try dsl(json.replacingOccurrences(of: #""id":"card""#, with: #""id":"memo""#))
        let payload = NearbyShare.makePayload(sender: "花子", appId: "other", appName: "メモ", entity: try XCTUnwrap(other.entity("memo")),
                                              records: [["name": .string("太郎")]])
        let ack = await rt.importNearby(payload)
        XCTAssertFalse(ack.ok)
        XCTAssertEqual(ack.added, 0)
        XCTAssertTrue(rt.records(for: "card").isEmpty)
    }
}
#endif
