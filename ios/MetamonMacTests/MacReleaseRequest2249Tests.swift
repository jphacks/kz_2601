import XCTest
import MetamonServices
@testable import Metamon

/// Mac のリリース申請のシート(#2249): 送る・必須が空・申請中の表示
@MainActor
final class MacReleaseRequest2249Tests: XCTestCase {
    private actor FakeClient: ReleaseRequestClient {
        var mine: [ReleaseRequest] = []
        var failure: Error?
        private(set) var sent: [(appId: String, purpose: String, contact: String?)] = []

        func setMine(_ items: [ReleaseRequest]) { mine = items }
        func setFailure(_ error: Error?) { failure = error }

        func request(appId: String, purpose: String, contact: String?) async throws -> ReleaseRequest {
            if let failure { throw failure }
            sent.append((appId, purpose, contact))
            return ReleaseRequest(id: "r1", appId: appId, purpose: purpose, contact: contact ?? "")
        }

        func myRequests() async throws -> [ReleaseRequest] { mine }
    }

    private func model(_ client: FakeClient?, store: MacReleaseRequests? = nil) -> MacReleaseRequestModel {
        MacReleaseRequestModel(appId: "app-1", appName: "家計簿", client: client, store: store ?? MacReleaseRequests())
    }

    func testSendTrimsAndShowsReceived() async {
        let fake = FakeClient()
        let store = MacReleaseRequests()
        let m = model(fake, store: store)
        m.purpose = "  毎朝の記録に  "
        m.contact = "   "
        XCTAssertNil(m.open, "申請前はフォーム")
        let ok = await m.send()
        XCTAssertTrue(ok)
        let sent = await fake.sent
        XCTAssertEqual(sent.count, 1)
        XCTAssertEqual(sent.first?.purpose, "毎朝の記録に", "前後の空白を外す")
        XCTAssertNil(sent.first?.contact ?? nil, "空の連絡先は送らない")
        XCTAssertTrue(m.justSent, "「受け付けました」")
        XCTAssertEqual(m.open?.kind, .received, "送った後はフォームの代わりに状況")
        XCTAssertEqual(store.byApp["app-1"]?.id, "r1", "バッジにもすぐ出す")
    }

    func testEmptyPurposeCannotSend() async {
        let fake = FakeClient()
        let m = model(fake)
        m.purpose = " \n "
        XCTAssertFalse(m.canSend)
        let ok = await m.send()
        XCTAssertFalse(ok)
        let sent = await fake.sent
        XCTAssertTrue(sent.isEmpty, "必須が空なら送らない")
    }

    func testOpenRequestShowsStatusInsteadOfForm() async {
        let fake = FakeClient()
        await fake.setMine([ReleaseRequest(id: "r2", appId: "app-1", status: "reviewing"),
                            ReleaseRequest(id: "r1", appId: "app-1", status: "declined")])
        let m = model(fake)
        await m.load()
        XCTAssertEqual(m.open?.kind, .reviewing, "申請中は状況を出す")
    }

    func testDeclinedShowsFormAgain() async {
        let fake = FakeClient()
        await fake.setMine([ReleaseRequest(id: "r1", appId: "app-1", status: "declined")])
        let m = model(fake)
        await m.load()
        XCTAssertNil(m.open, "見送りなら出し直せる")
    }

    func testFailureKeepsFormWithMessage() async {
        let fake = FakeClient()
        await fake.setFailure(URLError(.notConnectedToInternet))
        let m = model(fake)
        m.purpose = "用途"
        let ok = await m.send()
        XCTAssertFalse(ok)
        XCTAssertNotNil(m.error)
        XCTAssertNil(m.open)
        XCTAssertFalse(m.busy)
    }

    func testNoClientSaysServerOnly() async {
        let m = model(nil)
        m.purpose = "用途"
        let ok = await m.send()
        XCTAssertFalse(ok)
        XCTAssertNotNil(m.error)
    }

    func testMenuOnlyForOwnServerApps() {
        XCTAssertTrue(MacReleaseRequestModel.canRequest(isCreated: true, isOnServer: true, isSharedWithMe: false, signedIn: true, available: true))
        XCTAssertFalse(MacReleaseRequestModel.canRequest(isCreated: true, isOnServer: false, isSharedWithMe: false, signedIn: true, available: true))
        XCTAssertFalse(MacReleaseRequestModel.canRequest(isCreated: true, isOnServer: true, isSharedWithMe: true, signedIn: true, available: true))
        XCTAssertFalse(MacReleaseRequestModel.canRequest(isCreated: true, isOnServer: true, isSharedWithMe: false, signedIn: false, available: true))
        XCTAssertFalse(MacReleaseRequestModel.canRequest(isCreated: false, isOnServer: true, isSharedWithMe: false, signedIn: true, available: false))
    }

    func testFormRules() {
        XCTAssertEqual(ReleaseRequestForm.clipped(String(repeating: "あ", count: 250), max: ReleaseRequestForm.maxPurpose).count, 200)
        XCTAssertFalse(ReleaseRequestForm.canSend(purpose: "用途", busy: true))
        XCTAssertEqual(ReleaseRequestForm.values(purpose: " a ", contact: " b ").contact, "b")
    }
}
