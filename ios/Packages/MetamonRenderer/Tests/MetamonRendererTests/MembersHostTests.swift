import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer

@MainActor
@Observable
private final class MembersRuntime: DSLRuntime, MembersHost {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    var appMembers: [AppMember] = []
    var isAppAdmin: Bool?

    init(dsl: AppDSL) { self.dsl = dsl }

    func records(for entity: String) -> [Record] { [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
}

/// 参加者を持たないホスト(プレビュー・テスト)。
@MainActor
@Observable
private final class PlainRuntime: DSLRuntime {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []

    init(dsl: AppDSL) { self.dsl = dsl }

    func records(for entity: String) -> [Record] { [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
}

/// #271 / #272: type member / members の値(uid)を表示名にする。
@MainActor
final class MembersHostTests: XCTestCase {
    private var savedLanguage: AppLanguage!

    override func setUp() {
        super.setUp()
        savedLanguage = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() {
        LanguagePreference.shared.setting = savedLanguage
        super.tearDown()
    }

    private func dsl() throws -> AppDSL {
        try JSONDecoder().decode(AppDSL.self, from: Data(#"""
        {"schemaVersion":1,"app":{"name":"当番"},"initialScreen":"home","entities":[],
         "screens":[{"id":"home","title":"ホーム","components":[]}]}
        """#.utf8))
    }

    private func runtime() throws -> MembersRuntime {
        let rt = MembersRuntime(dsl: try dsl())
        rt.appMembers = [
            AppMember(uid: "u1", displayName: "花子", isAdmin: true),
            AppMember(uid: "u2", displayName: "太郎"),
            AppMember(uid: "u3", displayName: ""),
        ]
        return rt
    }

    func testMemberNameKnownUnknownAndEmpty() throws {
        let rt = try runtime()
        XCTAssertEqual(rt.memberName(.string("u1")), "花子")
        XCTAssertEqual(rt.memberName(.string("u3")), "参加者", "表示名が空なら「参加者」")
        XCTAssertEqual(rt.memberName(.string("gone")), "参加者", "一覧に無い uid")
        XCTAssertEqual(rt.memberName(.string("")), "", "未選択は空")
        XCTAssertEqual(rt.memberName(.null), "")
        XCTAssertEqual(rt.memberName(.number(1)), "", "文字でない値は未選択と同じ")
    }

    func testMemberNamesListsInOrder() throws {
        let rt = try runtime()
        XCTAssertEqual(rt.memberNames(.array([.string("u1")])), "花子")
        XCTAssertEqual(rt.memberNames(.array([.string("u2"), .string("u1")])), "太郎、花子", "値の順に並べる")
        let mixed = rt.memberNames(.array([.string("u1"), .string(""), .string("gone")]))
        XCTAssertTrue(mixed.contains("花子") && mixed.contains("参加者"), mixed)
        XCTAssertEqual(rt.memberNames(.string("u2")), "太郎", "1 つの uid の文字も読む")
    }

    func testMemberNamesEmpty() throws {
        let rt = try runtime()
        XCTAssertEqual(rt.memberNames(.array([])), "")
        XCTAssertEqual(rt.memberNames(.array([.string("")])), "")
        XCTAssertEqual(rt.memberNames(.null), "")
        XCTAssertEqual(rt.memberNames(.string("")), "")
    }

    func testHostWithoutMembers() throws {
        let rt = PlainRuntime(dsl: try dsl())
        XCTAssertNil(rt.membersHost)
        XCTAssertNil(rt.viewerIsAdmin)
        XCTAssertEqual(rt.memberName(.string("u1")), "参加者", "参加者を持たないホストは名前が分からない")
        XCTAssertEqual(rt.memberName(.string("")), "")
    }

    func testViewerIsAdminFollowsHost() throws {
        let rt = try runtime()
        XCTAssertNil(rt.viewerIsAdmin, "分からない間は nil")
        rt.isAppAdmin = true
        XCTAssertEqual(rt.viewerIsAdmin, true)
        rt.isAppAdmin = false
        XCTAssertEqual(rt.viewerIsAdmin, false)
    }
}
