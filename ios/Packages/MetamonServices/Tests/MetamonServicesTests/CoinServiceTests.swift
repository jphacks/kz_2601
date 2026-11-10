#if !APP_STORE_REVIEW // 審査用ビルドはコイン(App 内課金)を持たない(#450)
import XCTest
import MetamonDSL
@testable import MetamonServices

/// #630: コインの口。サーバーの形を読むこと・形が少し違っても落ちないこと・送る形。
final class CoinServiceTests: XCTestCase {
    private var savedSession: URLSession!
    private let base = URL(string: "https://appconfig.test/")!
    private var client: HTTPCoinClient { HTTPCoinClient(baseURL: base, token: StaticTokenProvider("u1")) }

    override func setUp() {
        super.setUp()
        savedSession = HTTP.session
        HTTP.session = HTTP.makeSession(protocolClasses: [StubURLProtocol.self])
    }

    override func tearDown() {
        HTTP.session = savedSession
        super.tearDown()
    }

    private func lastRequest() -> URLRequest? { StubURLProtocol.lock.withLock { StubURLProtocol.requests.last } }

    private func body(_ req: URLRequest?) -> [String: Any]? {
        guard let req else { return nil }
        var data = req.httpBody
        if data == nil, let stream = req.httpBodyStream {
            stream.open()
            defer { stream.close() }
            var buf = [UInt8](repeating: 0, count: 65536)
            var out = Data()
            while stream.hasBytesAvailable {
                let n = stream.read(&buf, maxLength: buf.count)
                if n <= 0 { break }
                out.append(buf, count: n)
            }
            data = out
        }
        return data.flatMap { try? JSONSerialization.jsonObject(with: $0) as? [String: Any] }
    }

    // MARK: - 読み取り(サーバーの形・docs/backend/openapi-appconfig.yaml の coins)

    func testWalletServerShape() throws {
        let json = #"""
        {"mode":"enforce","total":175,"available":170,"purchased":120,"grant":40,
         "grantPeriodEnd":"2026-12-01T00:00:00.123Z","initial":10,"reserved":5,
         "overflow":{"enabled":true,"monthlyCap":200,"spent":30}}
        """#
        let w = try CoinWallet.decode(Data(json.utf8))
        XCTAssertEqual(w.mode, "enforce")
        XCTAssertEqual(w.balance, CoinBalance(purchased: 120, grant: 40, grantPeriodEnd: w.balance.grantPeriodEnd, initial: 10, reserved: 5))
        XCTAssertEqual(w.balance.available, 170, "使える量 = 買った分 + 毎月の分 + 最初の分")
        XCTAssertEqual(w.balance.total, 175, "押さえている分も足すとサーバーの total")
        XCTAssertNotNil(w.balance.grantPeriodEnd, "小数秒つきの日時も読む")
        XCTAssertEqual(w.overflow, CoinOverflow(enabled: true, monthlyCap: 200, spent: 30))
        XCTAssertTrue(w.allowance.isEmpty, "サーバーは枠の使った量をまだ返さない")
    }

    func testWalletTolerant() throws {
        let json = #"""
        {"balance":{"purchased":"7","grant":null},"allowance":{"runs":{"used":3,"limit":10},"writes":{"limit":5}},
         "overflow":{"enabled":"yes"},"estimates":{"generate":30,"ai_button":1,"edit":{"coins":10},"chat":0},"unknown":{"x":1}}
        """#
        let w = try CoinWallet.decode(Data(json.utf8))
        XCTAssertEqual(w.balance.purchased, 7, "文字列の数・balance の中の形も読む")
        XCTAssertEqual(w.balance.grant, 0)
        XCTAssertEqual(w.allowance, ["runs": UsageMeter(used: 3, limit: 10)], "used の無い枠は捨てる")
        XCTAssertEqual(w.overflow?.enabled, false, "型が違えば既定(OFF)")
        XCTAssertEqual(w.estimates.coins(for: .generate), 30)
        XCTAssertEqual(w.estimates.coins(for: .edit), 10, "{coins: N} の形も読む")
        XCTAssertEqual(w.estimates.coins(for: .text), 1, "古い名前・snake_case のキーも引ける")
        XCTAssertNil(w.estimates.coins(for: .chat), "0 は出さない")
        XCTAssertNil(try CoinWallet.decode(Data(#"{"purchased":1}"#.utf8)).overflow, "overflow が無ければ設定を出さない")
    }

    func testWalletRejectsNonObject() {
        XCTAssertThrowsError(try CoinWallet.decode(Data("[]".utf8)))
    }

    func testLedgerServerShape() throws {
        let json = #"""
        {"entries":[
          {"id":"a","kind":"spend_ai","amount":-30,"op":"generate","appId":"app1","at":"2026-11-10T01:00:00Z"},
          {"id":"b","kind":"purchase"},
          {"id":"c","kind":"purchase","amount":100,"transactionId":"t1","productId":"p.s","at":"2026-11-09T01:00:00.5Z"}
        ],"nextCursor":"c2"}
        """#
        let page = try CoinLedgerPage.decode(Data(json.utf8))
        XCTAssertEqual(page.entries.count, 2, "増減の無い行だけ捨てる")
        XCTAssertEqual(page.entries[0], CoinLedgerEntry(id: "a", at: page.entries[0].at, kind: "spend_ai", operation: "generate", appId: "app1", delta: -30))
        XCTAssertNotNil(page.entries[0].at)
        XCTAssertEqual(page.entries[1].delta, 100)
        XCTAssertNotNil(page.entries[1].at)
        XCTAssertEqual(page.nextCursor, "c2")
        XCTAssertNil(try CoinLedgerPage.decode(Data(#"{"entries":[],"nextCursor":""}"#.utf8)).nextCursor, "空 = 続き無し")
    }

    func testConfigServerShape() throws {
        let json = #"""
        {"mode":"enforce",
         "packs":[{"productId":"p.s","coins":100},{"productId":"p.m","coins":300},{"productId":"","coins":5},{"productId":"p.x"}],
         "estimates":{"generate":30,"text":2},
         "initialGrant":50,"proMonthly":100,
         "allowance":{"runs":1000,"recipients":500,"writes":10000}}
        """#
        let c = try CoinPlanConfig.decode(Data(json.utf8))
        XCTAssertTrue(c.isEnabled, "mode=enforce で有効")
        XCTAssertEqual(c.packs.map(\.productId), ["p.s", "p.m"], "id・枚数の無いパックは捨てる")
        XCTAssertEqual(c.pack("p.m")?.totalCoins, 300)
        XCTAssertEqual(c.pack("p.m")?.bonusCoins, 0, "サーバーはおまけを返さない")
        XCTAssertEqual(c.estimates.coins(for: .generate), 30)
        XCTAssertEqual(c.estimates.coins(for: .text), 2)
        XCTAssertEqual(c.free, CoinPlanNumbers(oneTimeCoins: 50))
        XCTAssertEqual(c.pro, CoinPlanNumbers(monthlyCoins: 100, allowance: ["runs": 1000, "recipients": 500, "writes": 10000]))
        XCTAssertNil(c.overflowBlockCoins, "まとまりの数はサーバーがまだ返さない")
    }

    func testConfigZerosAreHidden() throws {
        let c = try CoinPlanConfig.decode(Data(#"{"mode":"enforce","packs":[],"estimates":{},"initialGrant":0,"proMonthly":0,"allowance":{"runs":0}}"#.utf8))
        XCTAssertEqual(c.free, CoinPlanNumbers())
        XCTAssertEqual(c.pro, CoinPlanNumbers())
    }

    func testConfigOffByDefault() throws {
        XCTAssertFalse(try CoinPlanConfig.decode(Data(#"{"mode":"log"}"#.utf8)).isEnabled, "log の段では出さない")
        XCTAssertFalse(try CoinPlanConfig.decode(Data(#"{"mode":"off"}"#.utf8)).isEnabled)
        XCTAssertFalse(try CoinPlanConfig.decode(Data("{}".utf8)).isEnabled)
        XCTAssertFalse(try CoinPlanConfig.decode(Data(#"{"enabled":false,"mode":"enforce"}"#.utf8)).isEnabled, "enabled が優先")
    }

    // MARK: - 送る形

    func testConfigRequestAnd404IsDisabled() async throws {
        StubURLProtocol.reset([.init(status: 404, body: #"{"error":{"code":"not_found","message":"x"}}"#)])
        let c = try await client.planConfig()
        XCTAssertEqual(c, .disabled, "古いサーバー = 出さない")
        let req = lastRequest()
        XCTAssertEqual(req?.httpMethod, "GET")
        XCTAssertEqual(req?.url?.path, "/v1/coins/config")
    }

    func testWalletAndLedgerRequests() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"mode":"enforce","purchased":1}"#),
                               .init(status: 200, body: #"{"entries":[],"nextCursor":""}"#)])
        _ = try await client.wallet()
        XCTAssertEqual(lastRequest()?.url?.path, "/v1/me/coins")
        XCTAssertEqual(lastRequest()?.value(forHTTPHeaderField: "Authorization"), "Bearer u1")
        _ = try await client.ledger(cursor: "a b/c")
        let url = try XCTUnwrap(lastRequest()?.url)
        XCTAssertEqual(url.path, "/v1/me/coins/ledger")
        XCTAssertEqual(URLComponents(url: url, resolvingAgainstBaseURL: true)?.queryItems?.first, URLQueryItem(name: "cursor", value: "a b/c"))
    }

    func testCreditSendsSignedTransactionAndReadsWallet() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"""
        {"credited":true,"transactionId":"t1","productId":"p.s","coins":100,
         "wallet":{"mode":"enforce","total":110,"available":110,"purchased":100,"grant":0,"initial":10,"reserved":0,
                   "overflow":{"enabled":false,"monthlyCap":0,"spent":0}}}
        """#)])
        let w = try await client.creditApplePurchase(signedTransaction: "jws.abc")
        XCTAssertEqual(w.balance.purchased, 100, "応答の wallet を読む")
        XCTAssertEqual(w.balance.available, 110)
        let req = lastRequest()
        XCTAssertEqual(req?.httpMethod, "POST")
        XCTAssertEqual(req?.url?.path, "/v1/me/coins/apple")
        let b = try XCTUnwrap(body(req))
        XCTAssertEqual(b["signedTransaction"] as? String, "jws.abc")
        XCTAssertEqual(Set(b.keys), ["signedTransaction"])
    }

    func testSettingsBody() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"mode":"enforce","purchased":0,"overflow":{"enabled":true,"monthlyCap":50,"spent":0}}"#)])
        let w = try await client.updateSettings(CoinSettingsUpdate(overflowEnabled: true, monthlyCap: -5))
        XCTAssertEqual(w.overflow, CoinOverflow(enabled: true, monthlyCap: 50, spent: 0))
        let req = lastRequest()
        XCTAssertEqual(req?.httpMethod, "PUT")
        XCTAssertEqual(req?.url?.path, "/v1/me/coins/settings")
        let ov = try XCTUnwrap(body(req)?["overflow"] as? [String: Any])
        XCTAssertEqual(ov["enabled"] as? Bool, true)
        XCTAssertEqual(ov["monthlyCap"] as? Int, 0, "負の上限は 0 にする")
    }

    // MARK: - 402

    func testDenialParsing() async {
        StubURLProtocol.reset([.init(status: 402, body: #"{"error":{"code":"coins_insufficient","message":"m","details":{"required":30,"balance":12,"payer":"creator"}}}"#)])
        do {
            _ = try await client.wallet()
            XCTFail("402 must throw")
        } catch {
            let d = try? XCTUnwrap(CoinDenial(error))
            XCTAssertEqual(d, CoinDenial(kind: .coinsInsufficient, required: 30, balance: 12, payer: "creator"))
            XCTAssertEqual(d?.payerIsSelf, false)
        }
        XCTAssertEqual(CoinDenial(APIError.server(status: 402, code: "allowance_exhausted", message: ""))?.kind, .allowanceExhausted)
        XCTAssertEqual(CoinDenial(APIError.server(status: 402, code: "overflow_cap_reached", message: ""))?.kind, .overflowCapReached)
        XCTAssertNil(CoinDenial(APIError.server(status: 402, code: "pro_required", message: "")), "Pro の 402 は別")
        XCTAssertNil(CoinDenial(APIError.server(status: 403, code: "coins_insufficient", message: "")))
    }
}

/// #630: コインの状態。フラグが OFF の間は何も出さない・サーバーが足してから取引を終わらせる。
@MainActor
final class CoinStoreTests: XCTestCase {
    func testDisabledWithoutServiceOrFlag() async {
        let none = CoinStore(mode: .mock)
        await none.refreshConfig()
        XCTAssertFalse(none.isEnabled, "口が無ければ出さない")

        let off = CoinStore(mode: .mock, service: MockCoinService(config: .disabled))
        await off.refreshConfig()
        XCTAssertFalse(off.isEnabled, "サーバーが OFF なら出さない")
        XCTAssertNil(off.estimate(.generate))
        XCTAssertTrue(off.offers.isEmpty)
        let w = await off.refreshWallet()
        XCTAssertNil(w, "OFF の間は残高も読まない")
    }

    func testEnabledMockOffersAndEstimate() async {
        let store = CoinStore(mode: .mock, service: MockCoinService())
        await store.refreshConfig()
        XCTAssertTrue(store.isEnabled)
        XCTAssertEqual(store.offers.map(\.id), MockCoinService.samplePackIDs)
        XCTAssertNil(store.offers.first?.displayPrice, "モックは値段を持たない")
        XCTAssertEqual(store.estimate(.generate), 30)
        await store.refreshWallet()
        XCTAssertEqual(store.wallet?.balance.available, 160)
    }

    func testFinishOnlyAfterServerCredits() async {
        let service = MockCoinService()
        let store = CoinStore(mode: .mock, service: service)
        await store.refreshConfig()
        let finished = FinishCounter()

        await service.setFailCredit(true)
        let failed = await store.creditThenFinish(signedTransaction: "mock:\(MockCoinService.samplePackIDs[0]):t1") { await finished.hit() }
        XCTAssertFalse(failed)
        let afterFail = await finished.count
        XCTAssertEqual(afterFail, 0, "サーバーが足せなかった取引は終わらせない")

        await service.setFailCredit(false)
        let ok = await store.creditThenFinish(signedTransaction: "mock:\(MockCoinService.samplePackIDs[0]):t1") { await finished.hit() }
        XCTAssertTrue(ok)
        let afterOK = await finished.count
        XCTAssertEqual(afterOK, 1)
        XCTAssertEqual(store.wallet?.balance.purchased, 220)

        // 同じ取引を送り直しても 1 回分(サーバーが冪等)
        await store.creditThenFinish(signedTransaction: "mock:\(MockCoinService.samplePackIDs[0]):t1") { await finished.hit() }
        XCTAssertEqual(store.wallet?.balance.purchased, 220)
    }

    func testMockPurchaseCreditsPack() async throws {
        let store = CoinStore(mode: .mock, service: MockCoinService())
        await store.refreshConfig()
        await store.refreshWallet()
        let outcome = try await store.purchase(productID: MockCoinService.samplePackIDs[1])
        XCTAssertEqual(outcome, .credited)
        XCTAssertEqual(store.wallet?.balance.purchased, 120 + 300)
        do {
            _ = try await store.purchase(productID: "unknown")
            XCTFail("知らない商品は買わない")
        } catch {
            XCTAssertEqual(error as? CoinStore.StoreError, .productNotFound)
        }
    }

    func testOverflowSettingsAndClear() async throws {
        let store = CoinStore(mode: .mock, service: MockCoinService())
        await store.refreshConfig()
        try await store.updateOverflow(enabled: true, monthlyCap: 80)
        XCTAssertEqual(store.wallet?.overflow?.enabled, true)
        XCTAssertEqual(store.wallet?.overflow?.monthlyCap, 80)
        store.clear()
        XCTAssertNil(store.wallet, "サインアウトで前のアカウントの残高を消す")
        XCTAssertTrue(store.isEnabled, "公開の設定は残す")
    }
}

private actor FinishCounter {
    private(set) var count = 0
    func hit() { count += 1 }
}
#endif
