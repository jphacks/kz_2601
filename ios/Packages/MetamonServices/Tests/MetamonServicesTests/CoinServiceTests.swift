#if !APP_STORE_REVIEW // 審査用ビルドはコイン(App 内課金)を持たない(#450)
import XCTest
import MetamonDSL
@testable import MetamonServices

/// #630: コインの口。契約が固まる前なので、形が少し違っても落ちずに読めること・送る形。
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

    // MARK: - 読み取り(寛容に)

    func testWalletNestedBalance() throws {
        let json = #"""
        {"balance":{"purchased":120,"grant":40,"grantPeriodEnd":"2026-12-01T00:00:00.123Z"},
         "allowance":{"runs":{"used":320,"limit":1000},"writes":{"used":5}},
         "overflow":{"enabled":true,"monthlyCap":200,"spent":30},
         "estimates":{"create":30,"ai_button":1,"edit":{"coins":10}},
         "unknown":{"x":1}}
        """#
        let w = try CoinWallet.decode(Data(json.utf8))
        XCTAssertEqual(w.balance.purchased, 120)
        XCTAssertEqual(w.balance.grant, 40)
        XCTAssertEqual(w.balance.total, 160)
        XCTAssertNotNil(w.balance.grantPeriodEnd, "小数秒つきの日時も読む")
        XCTAssertEqual(w.allowance["runs"], UsageMeter(used: 320, limit: 1000))
        XCTAssertEqual(w.allowance["writes"]?.limit, nil, "上限が無い = 数えない")
        XCTAssertEqual(w.overflow, CoinOverflow(enabled: true, monthlyCap: 200, spent: 30))
        XCTAssertEqual(w.estimates.coins(for: .create), 30)
        XCTAssertEqual(w.estimates.coins(for: .edit), 10, "{coins: N} の形も読む")
        XCTAssertEqual(w.estimates.coins(for: .aiButton), 1, "snake_case のキーも引ける")
        XCTAssertNil(w.estimates.coins(for: .chat))
    }

    func testWalletFlatBalanceAndWrongTypes() throws {
        let json = #"{"purchased":"7","grant":null,"overflow":{"enabled":"yes","spentThisMonth":3},"estimates":{"create":0}}"#
        let w = try CoinWallet.decode(Data(json.utf8))
        XCTAssertEqual(w.balance.purchased, 7, "文字列の数も読む")
        XCTAssertEqual(w.balance.grant, 0)
        XCTAssertFalse(w.overflow.enabled, "型が違えば既定(OFF)")
        XCTAssertEqual(w.overflow.spent, 3)
        XCTAssertNil(w.estimates.coins(for: .create), "0 は出さない")
    }

    func testWalletRejectsNonObject() {
        XCTAssertThrowsError(try CoinWallet.decode(Data("[]".utf8)))
    }

    func testLedgerSkipsBrokenRows() throws {
        let json = #"""
        {"items":[
          {"id":"a","at":"2026-11-10T01:00:00Z","kind":"spend","operation":"create","appName":"家計簿","delta":-30},
          {"id":"b","kind":"purchase"},
          {"createdAt":"2026-11-09T01:00:00Z","type":"purchase","amount":100}
        ],"nextCursor":"c2"}
        """#
        let page = try CoinLedgerPage.decode(Data(json.utf8))
        XCTAssertEqual(page.entries.count, 2, "増減の無い行だけ捨てる")
        XCTAssertEqual(page.entries[0], CoinLedgerEntry(id: "a", at: page.entries[0].at, kind: "spend", operation: "create", appName: "家計簿", delta: -30))
        XCTAssertNotNil(page.entries[0].at)
        XCTAssertEqual(page.entries[1].kind, "purchase")
        XCTAssertEqual(page.entries[1].delta, 100)
        XCTAssertEqual(page.nextCursor, "c2")
    }

    func testConfigFlagAndPacks() throws {
        let json = #"""
        {"mode":"enforce",
         "packs":[{"productId":"p.s","coins":100},{"productId":"p.m","coins":300,"bonusCoins":30},{"productId":"","coins":5},{"productId":"p.x"}],
         "estimates":{"create":30},
         "plans":{"free":{"allowance":{}},"pro":{"monthlyCoins":100,"allowance":{"runs":1000,"writes":{"limit":10000}}}},
         "overflow":{"blockCoins":10,"blockRuns":100}}
        """#
        let c = try CoinPlanConfig.decode(Data(json.utf8))
        XCTAssertTrue(c.isEnabled, "mode=enforce で有効")
        XCTAssertEqual(c.packs.map(\.productId), ["p.s", "p.m"], "id・枚数の無いパックは捨てる")
        XCTAssertEqual(c.pack("p.m")?.totalCoins, 330)
        XCTAssertEqual(c.pro.monthlyCoins, 100)
        XCTAssertEqual(c.pro.allowance, ["runs": 1000, "writes": 10000])
        XCTAssertNil(c.free.monthlyCoins)
        XCTAssertEqual(c.overflowBlockCoins, 10)
        XCTAssertEqual(c.overflowBlockRuns, 100)
    }

    func testConfigOffByDefault() throws {
        XCTAssertFalse(try CoinPlanConfig.decode(Data(#"{"mode":"log"}"#.utf8)).isEnabled, "log の段では出さない")
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
        StubURLProtocol.reset([.init(status: 200, body: #"{"balance":{"purchased":1}}"#),
                               .init(status: 200, body: #"{"items":[]}"#)])
        _ = try await client.wallet()
        XCTAssertEqual(lastRequest()?.url?.path, "/v1/me/coins")
        XCTAssertEqual(lastRequest()?.value(forHTTPHeaderField: "Authorization"), "Bearer u1")
        _ = try await client.ledger(cursor: "a b/c")
        let url = try XCTUnwrap(lastRequest()?.url)
        XCTAssertEqual(url.path, "/v1/me/coins/ledger")
        XCTAssertEqual(URLComponents(url: url, resolvingAgainstBaseURL: true)?.queryItems?.first, URLQueryItem(name: "cursor", value: "a b/c"))
    }

    func testCreditSendsSignedTransaction() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"balance":{"purchased":100}}"#)])
        let w = try await client.creditApplePurchase(signedTransaction: "jws.abc")
        XCTAssertEqual(w.balance.purchased, 100)
        let req = lastRequest()
        XCTAssertEqual(req?.httpMethod, "POST")
        XCTAssertEqual(req?.url?.path, "/v1/me/coins/apple")
        let b = try XCTUnwrap(body(req))
        XCTAssertEqual(b["signedTransaction"] as? String, "jws.abc")
        XCTAssertEqual(Set(b.keys), ["signedTransaction"])
    }

    func testSettingsBody() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"balance":{"purchased":0},"overflow":{"enabled":true,"monthlyCap":50}}"#)])
        let w = try await client.updateSettings(CoinSettingsUpdate(overflowEnabled: true, monthlyCap: -5))
        XCTAssertTrue(w.overflow.enabled)
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
        XCTAssertNil(off.estimate(.create))
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
        XCTAssertEqual(store.estimate(.create), 30)
        await store.refreshWallet()
        XCTAssertEqual(store.wallet?.balance.total, 160)
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

    func testMockPurchaseCreditsBonus() async throws {
        let store = CoinStore(mode: .mock, service: MockCoinService())
        await store.refreshConfig()
        await store.refreshWallet()
        let outcome = try await store.purchase(productID: MockCoinService.samplePackIDs[1])
        XCTAssertEqual(outcome, .credited)
        XCTAssertEqual(store.wallet?.balance.purchased, 120 + 330)
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
        XCTAssertEqual(store.wallet?.overflow.enabled, true)
        XCTAssertEqual(store.wallet?.overflow.monthlyCap, 80)
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
