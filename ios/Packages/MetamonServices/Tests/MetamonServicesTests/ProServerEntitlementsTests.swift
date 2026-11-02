// 審査用ビルド(APP_STORE_REVIEW・#450 G1)には課金が無い。
#if !APP_STORE_REVIEW
import XCTest
@testable import MetamonServices

/// #314: 検証済みの取引をサーバーに送る(重複は送らない・失敗は送り直す)と appAccountToken。
@MainActor
final class ProServerEntitlementsTests: XCTestCase {
    actor FakeClient: EntitlementClient {
        var received: [String] = []
        var failNext = false

        func setFailNext(_ v: Bool) { failNext = v }

        func submitAppleTransaction(_ signedTransaction: String) async throws -> ServerEntitlements {
            if failNext {
                failNext = false
                throw URLError(.notConnectedToInternet)
            }
            received.append(signedTransaction)
            return ServerEntitlements(
                isPro: true,
                pro: .init(
                    productId: ProProducts.monthly, environment: "Sandbox",
                    expiresAt: "2026-12-01T00:00:00Z"))
        }
    }

    func testAccountTokenMatchesBackend() {
        // backend internal/entitlement TestAccountToken と同じ値
        XCTAssertEqual(
            ProAccountToken.uuid(for: "firebase-uid-123").uuidString.lowercased(),
            "578b5ddb-6720-8e0a-a511-88fa2b02abdb")
        XCTAssertNotEqual(ProAccountToken.uuid(for: "a"), ProAccountToken.uuid(for: "b"))
    }

    func testSendsEachTransactionOncePerUID() async {
        let client = FakeClient()
        let store = ProEntitlementStore(mode: .storeKit)
        var uid = "alice"
        store.server = client
        store.accountUID = { [uid] in uid }

        store.sendToServer(["jws-1"])
        await store.serverSyncTask?.value
        store.sendToServer(["jws-1"])  // 起動のたびに同じ取引が来ても送らない
        await store.serverSyncTask?.value
        var received = await client.received
        XCTAssertEqual(received, ["jws-1"])
        XCTAssertEqual(store.serverEntitlements?.isPro, true)

        uid = "bob"  // 別のアカウントでサインインし直したら、その uid でもう一度送る
        store.accountUID = { [uid] in uid }
        store.sendToServer(["jws-1"])
        await store.serverSyncTask?.value
        received = await client.received
        XCTAssertEqual(received, ["jws-1", "jws-1"])
    }

    func testFailedSendIsRetried() async {
        let client = FakeClient()
        await client.setFailNext(true)
        let store = ProEntitlementStore(mode: .storeKit)
        store.server = client
        store.accountUID = { "alice" }

        store.sendToServer(["jws-2"])
        await store.serverSyncTask?.value
        var received = await client.received
        XCTAssertEqual(received, [])
        XCTAssertNil(store.serverEntitlements)

        store.sendToServer(["jws-2"])
        await store.serverSyncTask?.value
        received = await client.received
        XCTAssertEqual(received, ["jws-2"])
    }

    func testNoServerSendsNothing() async {
        let store = ProEntitlementStore(mode: .storeKit)
        store.sendToServer(["jws-3"])
        XCTAssertNil(store.serverSyncTask)
    }
}
#endif
