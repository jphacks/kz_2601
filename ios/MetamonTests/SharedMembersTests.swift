import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 外す・抜けるの成否を切り替えられる参加者 API(#254)。
private final class FakeMembers: MembershipClient, @unchecked Sendable {
    private let lock = NSLock()
    var shared = true
    var list = SharedMemberList(items: [
        SharedMember(uid: "o", role: .owner, you: true),
        SharedMember(uid: "m1", displayName: "父"),
        SharedMember(uid: "m2", displayName: "たろう"),
    ], role: .owner)
    var listError: Error?
    var writeError: Error?
    private(set) var removed: [String] = []
    private(set) var left = 0

    func previewShare(token: String) async throws -> ShareLinkPreview { ShareLinkPreview(token: token, name: "n", sharedData: true) }
    func importShare(token: String, displayName: String) async throws -> InstalledApp { InstalledApp(appId: "a", name: "n", member: true) }
    func hasMembersOnlyData(appId: String) async throws -> Bool { shared }
    func members(appId: String) async throws -> SharedMemberList {
        if let listError { throw listError }
        return list
    }
    func removeMember(appId: String, uid: String) async throws {
        if let writeError { throw writeError }
        lock.withLock { removed.append(uid) }
    }
    func leave(appId: String) async throws {
        if let writeError { throw writeError }
        lock.withLock { left += 1 }
    }
}

@MainActor
final class SharedMembersTests: XCTestCase {
    private let notMember = APIError.server(status: 403, code: "forbidden", message: "join this app with a share link to see its shared data",
                                            details: ["reason": .string("not_member")])

    private func loaded(_ fake: FakeMembers) async -> SharedMembersModel {
        let model = SharedMembersModel(appId: "app1", client: fake)
        await model.load()
        return model
    }

    func testLoadShowsMembersOnlyForSharedApps() async {
        let fake = FakeMembers()
        let model = await loaded(fake)
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.list?.items.count, 3)

        let privateApp = FakeMembers()
        privateApp.shared = false
        let hidden = await loaded(privateApp)
        XCTAssertEqual(hidden.phase, .hidden, "共有の記録が無いアプリでは参加者の欄を出さない")
    }

    func testLoadNotMember() async {
        let fake = FakeMembers()
        fake.listError = notMember
        let model = await loaded(fake)
        XCTAssertEqual(model.phase, .notMember)
    }

    func testRemoveIsOptimisticAndSuggestsRevokingLink() async {
        let fake = FakeMembers()
        let model = await loaded(fake)
        let dad = fake.list.items[1]
        let error = await model.remove(dad)
        XCTAssertNil(error)
        XCTAssertEqual(model.list?.items.map(\.uid), ["o", "m2"])
        XCTAssertEqual(model.justRemoved, dad, "外した後はリンクの取り消しへ誘導する")
        XCTAssertEqual(fake.removed, ["m1"])
    }

    func testRemoveFailureRestoresSamePosition() async {
        let fake = FakeMembers()
        let model = await loaded(fake)
        fake.writeError = URLError(.notConnectedToInternet)
        let dad = fake.list.items[1]
        let error = await model.remove(dad)
        XCTAssertNotNil(error, "失敗したら知らせる")
        XCTAssertEqual(model.list?.items.map(\.uid), ["o", "m1", "m2"], "元の位置に戻す")
        XCTAssertNil(model.justRemoved)
    }

    func testRemoveAlreadyGoneCountsAsSuccess() async {
        let fake = FakeMembers()
        let model = await loaded(fake)
        fake.writeError = APIError.server(status: 404, code: "not_found", message: "member not found")
        let error = await model.remove(fake.list.items[2])
        XCTAssertNil(error)
        XCTAssertEqual(model.list?.items.map(\.uid), ["o", "m1"])
    }

    func testLeaveOptimisticAndRollback() async {
        let fake = FakeMembers()
        fake.list = SharedMemberList(items: [SharedMember(uid: "o", role: .owner), SharedMember(uid: "me", you: true)], role: .member)
        let model = await loaded(fake)
        let ok = await model.leave()
        XCTAssertNil(ok)
        XCTAssertEqual(model.phase, .left)
        XCTAssertEqual(fake.left, 1)

        let failing = FakeMembers()
        failing.list = fake.list
        let model2 = await loaded(failing)
        failing.writeError = URLError(.timedOut)
        let err = await model2.leave()
        XCTAssertNotNil(err)
        XCTAssertEqual(model2.phase, .loaded, "失敗したら一覧に戻す")
    }

    func testMemberLabel() {
        XCTAssertEqual(SharedMember(uid: "o", role: .owner).label, L10n.tr("作成者"))
        XCTAssertEqual(SharedMember(uid: "m").label, L10n.tr("名前なし"))
        XCTAssertEqual(SharedMember(uid: "m", displayName: "父").label, "父")
    }

    // MARK: - 403 の文言

    func testDenialMessagesAreDistinctFromAuthorOnly() throws {
        let messages = [SharedAccessDenial.notMember, .inviterLeft, .membersFull].map(\.message)
        XCTAssertEqual(Set(messages).count, 3)
        XCTAssertFalse(messages.contains { $0.contains("作った人だけ") }, "#378 の文言と混ぜない")

        let leftError = APIError.server(status: 403, code: "forbidden", message: "the person who sent this link no longer participates in the app",
                                        details: ["reason": .string("inviter_left")])
        XCTAssertEqual(AppRuntime.importFailureMessage(leftError), SharedAccessDenial.inviterLeft.message)
        XCTAssertEqual(AppRuntime.importFailureMessage(APIError.server(status: 404, code: "not_found", message: "")),
                       L10n.tr("このシェアリンクは取り消されたか、存在しません"))

        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: BuildConfig.make(environment: .mock))
        XCTAssertEqual(rt.message(for: notMember, entity: "expense"), SharedAccessDenial.notMember.message, "records の 403 not_member")
    }
}
