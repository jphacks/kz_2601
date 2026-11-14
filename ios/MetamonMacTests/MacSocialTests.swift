#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// 人とつながる部品(#963): 会話の送信・印・通報 / ブロック・参加者・仲間探しを、iOS と同じ Services のクライアントで動かす
@MainActor
final class MacSocialTests: XCTestCase {
    private final class FakeSocial: MembershipClient, ModerationClient, MatchingClient, @unchecked Sendable {
        var fail = false
        var calls: [String] = []
        var ageConfirmed = false
        var incoming: [MatchInterest] = [MatchInterest(uid: "u-taro", profileRecordId: "r-taro")]

        private func log(_ s: String) throws {
            calls.append(s)
            if fail { throw APIError.server(status: 500, code: "internal", message: "boom") }
        }

        // MembershipClient
        func previewShare(token: String) async throws -> ShareLinkPreview { ShareLinkPreview(token: token, name: "") }
        func importShare(token: String, displayName: String) async throws -> InstalledApp { fatalError("unused") }
        func hasMembersOnlyData(appId: String) async throws -> Bool { true }
        func members(appId: String) async throws -> SharedMemberList {
            try log("members")
            return SharedMemberList(items: [SharedMember(uid: "u-me", role: .owner, you: true),
                                            SharedMember(uid: "u-hanako", displayName: "はなこ")], role: .owner)
        }
        func removeMember(appId: String, uid: String) async throws { try log("remove \(uid)") }
        func leave(appId: String) async throws { try log("leave") }
        func setAdmin(appId: String, uid: String, admin: Bool) async throws { try log("admin \(uid) \(admin)") }

        // ModerationClient
        func report(_ target: ReportTarget, reason: ReportReason) async throws { try log("report \(reason.rawValue)") }
        func blockedUsers() async throws -> [BlockedUser] {
            try log("blocked")
            return [BlockedUser(uid: "u-old", name: "前の人")]
        }
        func block(uid: String, name: String?) async throws -> BlockedUser {
            try log("block \(uid)")
            return BlockedUser(uid: uid, name: name ?? "")
        }
        func block(listingId: String) async throws -> BlockedUser { fatalError("unused") }
        func unblock(uid: String) async throws { try log("unblock \(uid)") }

        // MatchingClient
        func sendInterest(appId: String, recordId: String, fromRecordId: String, ageConfirmed: Bool) async throws -> MatchInterest {
            try log("send \(recordId)")
            return MatchInterest(uid: "u-hanako", profileRecordId: recordId)
        }
        func list(appId: String, box: MatchBox) async throws -> [MatchInterest] {
            try log("list \(box.rawValue)")
            return box == .incoming ? incoming : []
        }
        func accept(appId: String, uid: String, ageConfirmed: Bool) async throws -> MatchInterest {
            try log("accept \(uid)")
            return MatchInterest(uid: uid, status: "matched")
        }
        func decline(appId: String, uid: String) async throws { try log("decline \(uid)") }
        func unmatch(appId: String, uid: String) async throws { try log("unmatch \(uid)") }
        func ageConfirmation() async throws -> Bool { ageConfirmed }
        func confirmAge() async throws { try log("put age") }
    }

    private static let json = """
    {"schemaVersion":1,"app":{"name":"つながる"},"initialScreen":"home",
     "entities":[{"id":"msg","name":"会話","scope":"shared","marks":[{"id":"like","icon":"heart","label":"いいね"}],
                  "deleteBy":"author","fields":[{"id":"text","name":"本文","type":"string"}]},
                 {"id":"memo","name":"メモ","fields":[{"id":"text","name":"本文","type":"string"}]},
                 {"id":"profile","name":"プロフィール","scope":"public","match":"profile","fields":[{"id":"hobby","name":"趣味","type":"string"}]}],
     "screens":[{"id":"home","title":"会話","components":[{"type":"list","entity":"msg","item":{"title":"text"}}]}]}
    """

    private let hanako = Record(id: "r-hanako", data: ["text": "こんにちは"], author: "u-hanako", authorName: "はなこ")
    private var defaults: UserDefaults!

    override func setUp() {
        super.setUp()
        defaults = UserDefaults(suiteName: "MacSocialTests")
        defaults.removePersistentDomain(forName: "MacSocialTests")
    }

    private func runtime(_ fake: FakeSocial?, server: InMemoryAppConfigClient = InMemoryAppConfigClient(),
                         uid: String? = "u-me", online: Bool = true) async throws -> MacAppRuntime {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(Self.json.utf8))
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("mac-social-\(UUID().uuidString)")
        addTeardownBlock { try? FileManager.default.removeItem(at: dir) }
        let store = RecordStore(directory: dir)
        let engine = RecordSyncEngine(store: store, queue: SyncQueue(directory: nil, store: store, client: server),
                                      client: server, connectivity: ManualConnectivity(online: online))
        let rt = MacAppRuntime(appId: "social-\(UUID().uuidString.prefix(6))", dsl: dsl, recordSync: engine,
                               ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
        rt.social = MacSocial(currentUserId: { uid }, members: fake, moderation: fake, matching: fake,
                              ageGate: MatchingAgeGate(defaults: defaults))
        return rt
    }

    // MARK: 会話(chatThread の送信)

    func testChatPostOnSharedEntityCarriesMyUID() async throws {
        // オンラインだと送信後にモックサーバーの行(作者は別の uid)へ置き換わりうる。ローカルの作者を見るのでオフラインで作る
        let rt = try await runtime(FakeSocial(), online: false)
        XCTAssertNotNil((rt as any DSLRuntime) as? any RecordPosting, "chatThread が送信の口として見つける")
        let ok = await rt.postRecord(entity: "msg", data: ["text": "やあ"])
        XCTAssertTrue(ok)
        XCTAssertEqual(rt.records(for: "msg").first?.author, "u-me", "自分の投稿として出す(吹き出しの右側)")
    }

    func testPrivateEntityAndUnknownEntity() async throws {
        let rt = try await runtime(FakeSocial())
        _ = await rt.postRecord(entity: "memo", data: ["text": "x"])
        XCTAssertNil(rt.records(for: "memo").first?.author, "private の entity には作者を付けない")
        let unknown = await rt.postRecord(entity: "nope", data: ["text": "x"])
        XCTAssertFalse(unknown)
    }

    // MARK: 印

    func testMarkToggleConfirmsWithServer() async throws {
        let server = InMemoryAppConfigClient(seed: ["msg": [hanako]])
        let rt = try await runtime(FakeSocial(), server: server)
        await rt.load(entities: ["msg"], force: true)
        let state = await rt.toggleMark(entity: "msg", recordId: "r-hanako", mark: "like")
        XCTAssertEqual(state?.mine, true)
        XCTAssertEqual(state?.count, 1)
        XCTAssertEqual(rt.records(for: "msg").first?.markState("like").mine, true)
        XCTAssertNil(rt.toast)
    }

    func testMarkToggleRollsBackOnFailure() async throws {
        let server = InMemoryAppConfigClient(seed: ["msg": [hanako]])
        let rt = try await runtime(FakeSocial(), server: server)
        await rt.load(entities: ["msg"], force: true)
        try await server.delete(appId: rt.appId, entity: "msg", id: "r-hanako") // サーバーで消えた → 404
        let state = await rt.toggleMark(entity: "msg", recordId: "r-hanako", mark: "like")
        XCTAssertNil(state)
        XCTAssertEqual(rt.records(for: "msg").first?.markState("like").mine, false, "元に戻す")
        XCTAssertNotNil(rt.toast)
    }

    func testMarkUnsentPostTellsWhy() async throws {
        // オンラインだと作成直後に同期キューが裏で流れ、仮 id の行が先に送信済みへ置き換わることがある(不安定の原因)。
        // 未送信のまま確かめるためオフラインで作る(未送信の判定はオフラインより先)
        let rt = try await runtime(FakeSocial(), online: false)
        let created = await rt.createRecordId(entity: "msg", data: ["text": "送信前"])
        let id = try XCTUnwrap(created)
        let state = await rt.toggleMark(entity: "msg", recordId: id, mark: "like")
        XCTAssertNil(state)
        XCTAssertEqual(rt.toast, MacAppRuntime.markErrorText(MarkError.notSynced))
        let unknownMark = await rt.toggleMark(entity: "memo", recordId: id, mark: "like")
        XCTAssertNil(unknownMark, "印の無い entity には付けない")
    }

    // MARK: 通報・ブロック

    func testCanReportOnlyOthersSharedRecords() async throws {
        let rt = try await runtime(FakeSocial())
        XCTAssertTrue(rt.canReport(entity: "msg", record: hanako))
        XCTAssertFalse(rt.canReport(entity: "msg", record: Record(id: "r1", data: [:], author: "u-me")), "自分の投稿")
        XCTAssertFalse(rt.canReport(entity: "memo", record: hanako), "private の entity")
        let mock = try await runtime(nil)
        XCTAssertFalse(mock.canReport(entity: "msg", record: hanako), "モック(クライアント無し)ではメニューを出さない")
        let signedOut = try await runtime(FakeSocial(), uid: nil)
        XCTAssertFalse(signedOut.canReport(entity: "msg", record: hanako))
    }

    func testReportSuccessAndFailure() async throws {
        let fake = FakeSocial()
        let rt = try await runtime(fake)
        rt.reportRecord(entity: "msg", record: hanako)
        let req = try XCTUnwrap(rt.social.reporting)
        XCTAssertEqual(req.target, .record(appId: rt.appId, entityId: "msg", recordId: "r-hanako"))
        await rt.submitReport(req, reason: .spam)
        XCTAssertEqual(fake.calls, ["report spam"])
        XCTAssertEqual(rt.social.notice?.title, L10n.tr("通報しました"))
        XCTAssertNil(rt.social.reporting)

        fake.fail = true
        await rt.submitReport(req, reason: .spam)
        XCTAssertEqual(rt.social.notice?.title, L10n.tr("通報できませんでした"))
    }

    func testBlockHidesPostsAndRollsBackOnFailure() async throws {
        let fake = FakeSocial()
        let server = InMemoryAppConfigClient(seed: ["msg": [hanako]])
        let rt = try await runtime(fake, server: server)
        await rt.load(entities: ["msg"], force: true)
        XCTAssertEqual(rt.records(for: "msg").count, 1)

        fake.fail = true
        rt.blockAuthor(entity: "msg", record: hanako)
        await rt.confirmBlock(try XCTUnwrap(rt.social.blocking))
        XCTAssertEqual(rt.records(for: "msg").count, 1, "失敗したら戻す")
        XCTAssertEqual(rt.social.notice?.title, L10n.tr("ブロックできませんでした"))

        fake.fail = false
        rt.blockAuthor(entity: "msg", record: hanako)
        await rt.confirmBlock(try XCTUnwrap(rt.social.blocking))
        XCTAssertTrue(rt.records(for: "msg").isEmpty, "ブロックした人の投稿は隠す")
        XCTAssertEqual(rt.social.blocked["u-hanako"], "はなこ")

        await rt.unblock(BlockedUser(uid: "u-hanako", name: "はなこ"))
        XCTAssertEqual(rt.records(for: "msg").count, 1)
    }

    func testLoadBlockedUsersOncePerAccount() async throws {
        let fake = FakeSocial()
        let rt = try await runtime(fake)
        await rt.loadBlockedUsers()
        await rt.loadBlockedUsers()
        XCTAssertEqual(fake.calls, ["blocked"])
        XCTAssertEqual(rt.social.blocked["u-old"], "前の人")
    }

    // MARK: 参加者

    func testMembersLoadAndChanges() async throws {
        let fake = FakeSocial()
        let rt = try await runtime(fake)
        XCTAssertTrue(MacAppRuntime.usesMembers(rt.dsl), "deleteBy があるので参加者を読む")
        XCTAssertNil(rt.isAppAdmin, "読む前は分からない")
        await rt.reloadAppMembers()
        XCTAssertEqual(rt.appMembers.map(\.uid), ["u-me", "u-hanako"])
        XCTAssertEqual(rt.isAppAdmin, true)
        XCTAssertEqual(rt.memberName(.string("u-hanako")), "はなこ")

        let promoted = await rt.setMemberAdmin(uid: "u-hanako", admin: true)
        let removed = await rt.removeMember(uid: "u-hanako")
        XCTAssertTrue(promoted && removed)
        XCTAssertEqual(fake.calls, ["members", "admin u-hanako true", "members", "remove u-hanako", "members"], "変えたら読み直す")

        let left = await rt.leaveApp()
        XCTAssertTrue(left)
        XCTAssertTrue(rt.appMembers.isEmpty)
        XCTAssertEqual(rt.isAppAdmin, false)
    }

    func testMembersFailureShowsToast() async throws {
        let fake = FakeSocial()
        fake.fail = true
        let rt = try await runtime(fake)
        await rt.reloadAppMembers()
        XCTAssertNil(rt.isAppAdmin, "読めなければ分からないまま")
        let removed = await rt.removeMember(uid: "u-hanako")
        XCTAssertFalse(removed)
        XCTAssertEqual(rt.toast, L10n.tr("参加者を外せませんでした"))
        let left = await rt.leaveApp()
        XCTAssertFalse(left)
        let mock = try await runtime(nil)
        let mockLeft = await mock.leaveApp()
        XCTAssertFalse(mockLeft, "モックでは何もしない")
    }

    // MARK: 仲間探し

    private var taroProfile: Record { Record(id: "r-taro", data: ["hobby": "将棋"], author: "u-taro", authorName: "たろう") }

    func testInterestAsksAgeFirstThenSends() async throws {
        let fake = FakeSocial()
        // オンラインだと裏の同期でプロフィールがモックサーバーの行(作者 mock の uid)に置き換わり、
        // 「自分のプロフィール」が見つからなくなる(不安定の原因)。仲間探しは同期と無関係なのでオフラインで作る
        let rt = try await runtime(fake, online: false)
        _ = await rt.postRecord(entity: "profile", data: ["hobby": "囲碁"])
        XCTAssertEqual(rt.interestState(entity: "profile", record: taroProfile), InterestState.none)
        XCTAssertNil(rt.interestState(entity: "profile", record: Record(id: "x", data: [:], author: "u-me")), "自分の行には出さない")

        await rt.sendInterest(entity: "profile", record: taroProfile)
        guard case .send? = rt.social.askingAge else { return XCTFail("初回は 18 歳以上の確認を出す") }
        XCTAssertTrue(fake.calls.isEmpty)

        await rt.confirmAdult(try XCTUnwrap(rt.social.askingAge))
        XCTAssertTrue(fake.calls.contains("put age"))
        XCTAssertTrue(fake.calls.contains("send r-taro"))
        XCTAssertEqual(rt.interestState(entity: "profile", record: taroProfile), .sent)
    }

    func testInterestNeedsOwnProfile() async throws {
        let rt = try await runtime(FakeSocial())
        await rt.sendInterest(entity: "profile", record: taroProfile)
        XCTAssertEqual(rt.toast, L10n.tr("先に自分のプロフィールを書いてください"))
    }

    func testAcceptAndFailureRollsBack() async throws {
        let fake = FakeSocial()
        let rt = try await runtime(fake)
        rt.social.ageGate.confirm()
        await rt.refreshMatches([.incoming])
        let item = try XCTUnwrap(rt.matchItems(.incoming)?.first)

        fake.fail = true
        await rt.acceptInterest(item)
        XCTAssertEqual(rt.matchItems(.incoming)?.count, 1, "失敗したら一覧を戻す")
        XCTAssertEqual(rt.toast, L10n.tr("うまくいきませんでした。もう一度お試しください"))

        fake.fail = false
        await rt.acceptInterest(item)
        XCTAssertEqual(rt.interestState(entity: "profile", record: taroProfile), .matched)
        XCTAssertEqual(rt.toast, L10n.tr("つながりました。連絡先が見られます"))
    }

    func testDeclineAdultAndMockHidesButton() async throws {
        let rt = try await runtime(FakeSocial())
        rt.social.askingAge = .refresh([.incoming])
        rt.declineAdult()
        XCTAssertNil(rt.social.askingAge)
        XCTAssertTrue(rt.social.declinedAge)
        let mock = try await runtime(nil)
        XCTAssertNil(mock.interestState(entity: "profile", record: taroProfile), "モックではボタンを出さない")
        XCTAssertNil(mock.matchItems(.incoming))
    }

    // MARK: 後始末

    func testWipeClearsSocialState() async throws {
        let fake = FakeSocial()
        let rt = try await runtime(fake)
        rt.social.ageGate.confirm()
        await rt.reloadAppMembers()
        await rt.loadBlockedUsers()
        await rt.refreshMatches([.incoming])
        rt.social.ageGate.markSent(uid: "u-me")
        rt.reportRecord(entity: "msg", record: hanako)

        rt.social.wipe()
        XCTAssertTrue(rt.social.blocked.isEmpty)
        XCTAssertTrue(rt.appMembers.isEmpty)
        XCTAssertNil(rt.social.reporting)
        XCTAssertNil(rt.matchItems(.incoming), "前のアカウントの一覧は捨てる")
        XCTAssertFalse(rt.social.ageGate.isSent(uid: "u-me"))
        await rt.loadBlockedUsers()
        XCTAssertEqual(fake.calls.filter { $0 == "blocked" }.count, 2, "サインインし直したら読み直す")
    }
}
#endif
