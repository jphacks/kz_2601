#if METAMON_STG
import XCTest
import MetamonServices
@testable import Metamon

/// #2083: 開発用メニューの「サーバーの応答」の見本が、それぞれの案内(FailureKind / ExtDenial / AppLimit / CoinDenial)に届く
final class DebugFaultPresetsTests: XCTestCase {
    private func preset(_ id: String) throws -> DebugFaultPreset {
        try XCTUnwrap(DebugFaultPreset.all.first { $0.id == id }, id)
    }

    private func error(_ id: String) throws -> Error {
        try XCTUnwrap(try preset(id).outcome.expectedError, id)
    }

    func testIdsAreUniqueAndGrouped() {
        let ids = DebugFaultPreset.all.map(\.id)
        XCTAssertEqual(Set(ids).count, ids.count)
        for p in DebugFaultPreset.all { XCTAssertTrue(DebugFaultPreset.groups.contains(p.group), p.id) }
    }

    func testProRequired() throws {
        let mine = try XCTUnwrap(ProRequired(try error("pro")))
        XCTAssertFalse(mine.creatorNeedsPro)
        XCTAssertEqual(mine.features.first?.id, "sharedData")
        XCTAssertTrue(try XCTUnwrap(ProRequired(try error("proCreator"))).creatorNeedsPro)
    }

    func testAppLimit() throws {
        let buy = try XCTUnwrap(AppLimit(try error("appLimitBuy")))
        XCTAssertTrue(buy.canBuySlot)
        XCTAssertEqual(buy.slotCoins, 100)
        XCTAssertEqual(buy.apps?.count, 3)
        XCTAssertEqual(buy.apps?.filter(\.mine).count, 2)
        XCTAssertEqual(FailureKind.classify(try error("appLimitBuy")), .appLimit(buy))
        let noBuy = try XCTUnwrap(AppLimit(try error("appLimitNoBuy")))
        XCTAssertFalse(noBuy.canBuySlot)
        XCTAssertNil(noBuy.slotCoins)
    }

    func testCoins() throws {
        XCTAssertEqual(CoinDenialRoute.of(try XCTUnwrap(CoinDenial(try error("coinsSelf")))), .shop)
        XCTAssertEqual(CoinDenialRoute.of(try XCTUnwrap(CoinDenial(try error("coinsCreator")))), .explainOnly)
        XCTAssertEqual(CoinDenialRoute.of(try XCTUnwrap(CoinDenial(try error("allowance")))), .settings)
        XCTAssertEqual(CoinDenialRoute.of(try XCTUnwrap(CoinDenial(try error("overflowCap")))), .settings)
    }

    func testExtDenials() throws {
        func route(_ id: String) throws -> ExtDenialRoute {
            ExtDenialRoute.of(try XCTUnwrap(ExtDenial(try error(id)), id), isPro: false)
        }
        XCTAssertEqual(try route("extNotConnected"), .connect(provider: DebugFaultPreset.extProvider))
        XCTAssertEqual(try route("extUnavailable"), .toast(ExtDenialCopy.unavailable))
        XCTAssertEqual(try route("extBusy"), .toast(ExtDenialCopy.busy))
        guard case .paywall = try route("extPro") else { return XCTFail("課金画面のはず") }
        guard case .coins(let c) = try route("extCoins") else { return XCTFail("コインのはず") }
        XCTAssertEqual(CoinDenialRoute.of(c), .shop)
        XCTAssertNil(ExtDenial(try error("extRate")), "429 は外の API の断りではなく、ふつうのエラーの扱い")
    }

    func testExtPresetsTargetTheExtPath() throws {
        var req = URLRequest(url: URL(string: "https://appconfig.example.test/v1/apps/a1/ext/fakeOauth/listItems")!)
        req.httpMethod = "POST"
        for p in DebugFaultPreset.external { XCTAssertTrue(p.match.matches(req), p.id) }
    }

    func testBusyAndSignIn() throws {
        guard case .rateLimited(let user) = FailureKind.classify(try error("rateUser")) else { return XCTFail() }
        XCTAssertEqual(user.scope, .user)
        guard case .rateLimited(let ip) = FailureKind.classify(try error("rateNoHeader")) else { return XCTFail() }
        XCTAssertTrue(ip.isAnonymous)
        XCTAssertEqual(FailureKind.classify(try error("rateClaude")), .busy(retryAfter: 60))
        XCTAssertEqual(FailureKind.classify(try error("unauthenticated")), .sessionExpired)
        XCTAssertEqual(FailureKind.classify(try error("internal")), .busy(retryAfter: nil))
        XCTAssertEqual(FailureKind.classify(try error("unavailable")), .busy(retryAfter: 5))
    }

    func testTransport() throws {
        XCTAssertEqual(FailureKind.classify(try error("offline")), .offline)
        XCTAssertNil(try preset("slow").outcome.expectedError, "遅らせて本物へ通す")
        XCTAssertNil(try preset("streamCut").outcome.expectedError, "切れたら通常の呼び方に戻る")
        XCTAssertEqual(try preset("streamCut").followUp, 0, "通常の呼び方は本物へ通す")
        guard case .rateLimited(let g) = FailureKind.classify(try error("streamError")) else { return XCTFail() }
        XCTAssertEqual(g.scope, .global)
        XCTAssertEqual(FailureKind.classify(try error("streamFailed")), .busy(retryAfter: nil))
    }

    func testStreamPresetsOnlyMatchTheStream() throws {
        var plain = URLRequest(url: URL(string: "https://ai.example.test/v1/generate")!)
        plain.httpMethod = "POST"
        var stream = URLRequest(url: URL(string: "https://ai.example.test/v1/generate?stream=1")!)
        stream.httpMethod = "POST"
        for id in ["streamCut", "streamError", "streamFailed"] {
            XCTAssertTrue(try preset(id).match.matches(stream), id)
            XCTAssertFalse(try preset(id).match.matches(plain), id)
        }
    }

    func testTargetOverridesAndTimes() throws {
        let p = try preset("pro")
        let once = p.rule(always: false, target: .preset)
        XCTAssertEqual(once.match, DebugFaultPreset.generate)
        XCTAssertEqual(once.times, .once(followUp: 20))
        let everywhere = p.rule(always: true, target: .all)
        XCTAssertEqual(everywhere.match, .all)
        XCTAssertEqual(everywhere.times, .always)
        XCTAssertTrue(everywhere.label.contains("全部の通信"))
    }
}
#endif
