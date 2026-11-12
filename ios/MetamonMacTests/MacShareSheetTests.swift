import XCTest
import AppKit
import MetamonServices
@testable import MetamonRenderer
@testable import Metamon

/// 共有シートの QR・送り先・参加している人(#974)と、グラフのホバー(#945)
@MainActor
final class MacShareSheetTests: XCTestCase {
    private final class FakeMembers: MembershipClient, @unchecked Sendable {
        var fail = false
        var failStatus = 500
        var shared = true
        var role: SharedMember.Role = .owner
        var calls: [String] = []
        /// 一覧を返す直前に呼ぶ(読み込み中のサインアウトを再現する)
        var beforeMembers: (@Sendable () async -> Void)?

        private func log(_ s: String) throws {
            calls.append(s)
            if fail { throw APIError.server(status: failStatus, code: "x", message: "boom") }
        }

        func previewShare(token: String) async throws -> ShareLinkPreview { ShareLinkPreview(token: token, name: "") }
        func importShare(token: String, displayName: String) async throws -> InstalledApp { fatalError("unused") }
        func hasMembersOnlyData(appId: String) async throws -> Bool { shared }
        func members(appId: String) async throws -> SharedMemberList {
            calls.append("members")
            await beforeMembers?()
            return SharedMemberList(items: [SharedMember(uid: "u-me", role: role, you: true),
                                            SharedMember(uid: "u-hanako", displayName: "はなこ"),
                                            SharedMember(uid: "u-owner", role: role == .owner ? .member : .owner)],
                                    role: role)
        }
        func removeMember(appId: String, uid: String) async throws { try log("remove \(uid)") }
        func leave(appId: String) async throws { try log("leave") }
        func setAdmin(appId: String, uid: String, admin: Bool) async throws { try log("admin \(uid) \(admin)") }
    }

    private let link = URL(string: "https://tailorit.app/s/abc")!

    // MARK: - 送り先

    func testLineOpensShareScreenWithMessageAndLink() throws {
        let url = try XCTUnwrap(MacShareTarget.line.urls(appName: "家計簿", link: link).first)
        XCTAssertEqual(url.host, "line.me")
        XCTAssertEqual(url.path, "/R/share")
        let text = try XCTUnwrap(URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first { $0.name == "text" }?.value)
        XCTAssertTrue(text.contains("家計簿"))
        XCTAssertTrue(text.hasSuffix(link.absoluteString))
        XCTAssertFalse(MacShareTarget.line.copiesText)
    }

    func testSlackAndDiscordPreferAppThenWeb() {
        XCTAssertEqual(MacShareTarget.slack.urls(appName: "a", link: link).map(\.absoluteString), ["slack://open", "https://slack.com/"])
        XCTAssertEqual(MacShareTarget.discord.urls(appName: "a", link: link).map(\.absoluteString), ["discord://", "https://discord.com/app"])
        XCTAssertTrue(MacShareTarget.canOpen(URL(string: "https://slack.com/")!), "https はブラウザで開ける")
    }

    func testSlackCopiesTextAndFallsBackToWeb() {
        let board = NSPasteboard(name: .init("MacShareSheetTests.\(UUID().uuidString)"))
        defer { board.releaseGlobally() }
        var opened: [URL] = []
        let url = MacShareTarget.slack.send(appName: "家計簿", link: link, board: board,
                                            canOpen: { $0.scheme == "https" }, open: { opened.append($0) })
        XCTAssertEqual(url?.absoluteString, "https://slack.com/", "アプリが無ければ Web")
        XCTAssertEqual(opened, [url].compactMap { $0 })
        let copied = board.string(forType: .string) ?? ""
        XCTAssertTrue(copied.contains(link.absoluteString))
        XCTAssertTrue(copied.hasPrefix("💬 "), "チャンネル向けの整形(iOS の shareButton と同じ)")
        XCTAssertNotNil(MacShareTarget.slack.sentNotice)
    }

    func testDiscordOpensAppWhenInstalled() {
        let board = NSPasteboard(name: .init("MacShareSheetTests.\(UUID().uuidString)"))
        defer { board.releaseGlobally() }
        let url = MacShareTarget.discord.send(appName: "a", link: link, board: board, canOpen: { _ in true }, open: { _ in })
        XCTAssertEqual(url?.absoluteString, "discord://")
    }

    func testLineDoesNotTouchPasteboard() {
        let board = NSPasteboard(name: .init("MacShareSheetTests.\(UUID().uuidString)"))
        defer { board.releaseGlobally() }
        board.clearContents()
        board.setString("keep", forType: .string)
        _ = MacShareTarget.line.send(appName: "a", link: link, board: board, canOpen: { _ in true }, open: { _ in })
        XCTAssertEqual(board.string(forType: .string), "keep")
    }

    // MARK: - QR

    func testQRCodeForShareLink() {
        let info = ShareInfo(token: "abc", appId: "a", name: "n", dslVersion: 1, url: "metamon://share/abc", webUrl: "https://tailorit.app/s/abc")
        XCTAssertEqual(MacShareAppModel.qrText(info), "metamon://share/abc", "iOS と同じくアプリのリンク")
        XCTAssertNotNil(QRCodeImage.image(for: MacShareAppModel.qrText(info), scale: 6))
    }

    // MARK: - 参加している人

    private func loadedModel(_ fake: FakeMembers, social: MacSocial) async -> MacSharedMembersModel {
        let model = MacSharedMembersModel(appId: "app-1", client: fake, social: social)
        await model.load()
        return model
    }

    func testHiddenWithoutSharedData() async {
        let fake = FakeMembers()
        fake.shared = false
        let model = await loadedModel(fake, social: MacSocial())
        XCTAssertEqual(model.phase, .hidden)
        XCTAssertFalse(model.isVisible)
        XCTAssertFalse(fake.calls.contains("members"), "共有の記録が無いアプリでは一覧を読まない")
    }

    func testLoadSyncsRuntimeMembers() async {
        let social = MacSocial()
        let model = await loadedModel(FakeMembers(), social: social)
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(social.memberEntries["app-1"]?.members.map(\.uid), ["u-me", "u-hanako", "u-owner"])
        XCTAssertEqual(social.memberEntries["app-1"]?.isAdmin, true)
    }

    func testOwnerPermissions() async throws {
        let model = await loadedModel(FakeMembers(), social: MacSocial())
        let items = try XCTUnwrap(model.list?.items)
        XCTAssertFalse(model.canRemove(items[0]), "自分は外せない")
        XCTAssertTrue(model.canRemove(items[1]))
        XCTAssertTrue(model.canSetAdmin(items[1]))
        XCTAssertFalse(model.canLeave, "作成者は抜けない")
    }

    func testMemberCanOnlyLeave() async throws {
        let fake = FakeMembers()
        fake.role = .member
        let model = await loadedModel(fake, social: MacSocial())
        let items = try XCTUnwrap(model.list?.items)
        XCTAssertFalse(items.contains { model.canRemove($0) || model.canSetAdmin($0) })
        XCTAssertTrue(model.canLeave)
    }

    func testRemoveUpdatesListAndRuntime() async throws {
        let fake = FakeMembers()
        let social = MacSocial()
        let model = await loadedModel(fake, social: social)
        let hanako = try XCTUnwrap(model.list?.items[1])
        let error = await model.remove(hanako)
        XCTAssertNil(error)
        XCTAssertEqual(fake.calls.last, "remove u-hanako")
        XCTAssertEqual(model.list?.items.map(\.uid), ["u-me", "u-owner"])
        XCTAssertEqual(model.justRemoved, hanako, "リンクの取り消しへ誘導")
        XCTAssertEqual(social.memberEntries["app-1"]?.members.map(\.uid), ["u-me", "u-owner"])
    }

    func testRemoveFailureRollsBack() async throws {
        let fake = FakeMembers()
        let model = await loadedModel(fake, social: MacSocial())
        let hanako = try XCTUnwrap(model.list?.items[1])
        fake.fail = true
        let error = await model.remove(hanako)
        XCTAssertNotNil(error)
        XCTAssertEqual(model.list?.items.map(\.uid), ["u-me", "u-hanako", "u-owner"], "元の位置に戻す")
        XCTAssertNil(model.justRemoved)
    }

    func testRemoveAlreadyGoneIsSuccess() async throws {
        let fake = FakeMembers()
        let model = await loadedModel(fake, social: MacSocial())
        let hanako = try XCTUnwrap(model.list?.items[1])
        fake.fail = true
        fake.failStatus = 404
        let error = await model.remove(hanako)
        XCTAssertNil(error)
        XCTAssertEqual(model.list?.items.count, 2)
    }

    func testSetAdminAndRollback() async throws {
        let fake = FakeMembers()
        let social = MacSocial()
        let model = await loadedModel(fake, social: social)
        let hanako = try XCTUnwrap(model.list?.items[1])
        let ok = await model.setAdmin(hanako, true)
        XCTAssertNil(ok)
        XCTAssertEqual(fake.calls.last, "admin u-hanako true")
        XCTAssertEqual(model.list?.items[1].isAdmin, true)
        XCTAssertEqual(social.memberEntries["app-1"]?.members[1].isAdmin, true)

        fake.fail = true
        let promoted = try XCTUnwrap(model.list?.items[1])
        let failed = await model.setAdmin(promoted, false)
        XCTAssertNotNil(failed)
        XCTAssertEqual(model.list?.items[1].isAdmin, true, "失敗したら戻す")
    }

    func testLeave() async {
        let fake = FakeMembers()
        fake.role = .member
        let social = MacSocial()
        let model = await loadedModel(fake, social: social)
        let error = await model.leave()
        XCTAssertNil(error)
        XCTAssertEqual(model.phase, .left)
        XCTAssertEqual(social.memberEntries["app-1"]?.members.count, 0)
        XCTAssertEqual(social.memberEntries["app-1"]?.isAdmin, false)
    }

    func testLeaveFailureRestoresPhase() async {
        let fake = FakeMembers()
        fake.role = .member
        let model = await loadedModel(fake, social: MacSocial())
        fake.fail = true
        let error = await model.leave()
        XCTAssertNotNil(error)
        XCTAssertEqual(model.phase, .loaded)
    }

    /// 読み込み中にサインアウトでシートを閉じたら、届いた一覧を書き戻さない
    func testCloseDuringLoadDoesNotWriteMembersBack() async throws {
        let social = MacSocial()
        let fake = FakeMembers()
        let share = MacShareAppModel(appId: "app-1", appName: "n", sharing: nil, membership: fake, social: social)
        let model = try XCTUnwrap(share.members)
        fake.beforeMembers = { await MainActor.run { share.close() } }
        await model.load()
        XCTAssertNil(model.list)
        XCTAssertFalse(model.isVisible)
        XCTAssertNil(social.memberEntries["app-1"])
    }

    func testShareModelCreatesMembersOnlyWithClient() {
        XCTAssertNil(MacShareAppModel(appId: "a", appName: "n", sharing: nil).members)
        XCTAssertNotNil(MacShareAppModel(appId: "a", appName: "n", sharing: nil, membership: FakeMembers()).members)
    }

    // MARK: - グラフのホバー(#945)

    func testNearestBarOrPoint() {
        let xs: [CGFloat?] = [10, 50, nil, 90]
        XCTAssertEqual(ChartReadout.nearestIndex(toX: 0, positions: xs), 0)
        XCTAssertEqual(ChartReadout.nearestIndex(toX: 31, positions: xs), 1)
        XCTAssertEqual(ChartReadout.nearestIndex(toX: 75, positions: xs), 3, "描けない見出しは飛ばす")
        XCTAssertEqual(ChartReadout.nearestIndex(toX: 500, positions: xs), 3)
        XCTAssertNil(ChartReadout.nearestIndex(toX: 0, positions: [nil]))
    }

    func testPieHoverPicksSlice() throws {
        let center = CGPoint(x: 100, y: 100)
        let values = [1.0, 1.0, 2.0] // 0〜90°・90〜180°・180〜360°
        func slice(_ p: CGPoint) -> Int? {
            ChartReadout.cumulative(at: p, center: center, radius: 80, total: 4).flatMap { ChartReadout.index(atCumulative: $0, values: values) }
        }
        XCTAssertEqual(slice(CGPoint(x: 140, y: 60)), 0, "右上")
        XCTAssertEqual(slice(CGPoint(x: 140, y: 140)), 1, "右下")
        XCTAssertEqual(slice(CGPoint(x: 60, y: 140)), 2, "左下")
        XCTAssertEqual(slice(CGPoint(x: 60, y: 60)), 2, "左上")
        XCTAssertNil(slice(CGPoint(x: 100, y: 0)), "円の外")
        XCTAssertNil(ChartReadout.cumulative(at: center, center: center, radius: 80, total: 4))
        XCTAssertNil(ChartReadout.cumulative(at: CGPoint(x: 120, y: 100), center: center, radius: 80, total: 0))
    }

    func testReadoutFormatMatchesIOS() {
        let ja = Locale(identifier: "ja_JP")
        XCTAssertEqual(ChartReadout.line(label: "3月", value: 1234, locale: ja), "3月: 1,234")
        XCTAssertEqual(ChartReadout.format(2.26, locale: Locale(identifier: "de_DE")), "2,3")
    }
}
