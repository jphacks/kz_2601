import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// #277 変身一覧の整理(AppRuntime): 先に一覧へ反映し、サーバーが失敗したら戻す。
@MainActor
final class AppListManageTests: XCTestCase {
    private let apps = [
        AppSummary(id: "a", name: "家計簿", currentVersion: 3, updatedAt: "2026-10-24T09:00:00Z"),
        AppSummary(id: "b", name: "買い物", currentVersion: 2, updatedAt: "2026-10-25T09:00:00Z"),
        AppSummary(id: "c", name: "日記", currentVersion: 4, updatedAt: "2026-10-23T09:00:00Z"),
    ]

    private struct Boom: Error {}

    /// 一覧の書き換えだけ失敗するクライアント
    private actor FailingClient: AppConfigClient {
        func fetchDocument(appId: String) async throws -> AppDocument { try DSLExamples.document(DSLExamples.initial) }
        func listVersions(appId: String) async throws -> [VersionSummary] { [] }
        func restore(appId: String, toVersion version: Int) async throws -> AppDocument { throw Boom() }
        func list(appId: String, entity: String) async throws -> [Record] { [] }
        func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record { throw Boom() }
        func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record { throw Boom() }
        func delete(appId: String, entity: String, id: String) async throws {}
        func renameApp(appId: String, name: String) async throws -> AppSummary { throw Boom() }
        func duplicateApp(appId: String, name: String) async throws -> AppSummary { throw Boom() }
        func reorderApps(ids: [String]) async throws { throw Boom() }
    }

    private func runtime(failing: Bool = false) async throws -> AppRuntime {
        var services = BuildConfig.make(environment: .mock)
        if failing {
            services.appConfig = FailingClient()
        } else if let mem = services.appConfig as? InMemoryAppConfigClient {
            await mem.setApps(apps)
        }
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 3), services: services)
        rt.apps = apps
        return rt
    }

    func testRenameUpdatesList() async throws {
        let rt = try await runtime()
        await rt.renameApp(apps[0], to: "  仕事の家計簿 ")
        XCTAssertEqual(rt.apps.first { $0.id == "a" }?.name, "仕事の家計簿")
        XCTAssertNil(rt.toast)
    }

    func testRenameRejectsEmptyName() async throws {
        let rt = try await runtime()
        await rt.renameApp(apps[0], to: "   ")
        XCTAssertEqual(rt.apps.first { $0.id == "a" }?.name, "家計簿")
        XCTAssertNotNil(rt.toast)
    }

    func testRenameRollsBackOnFailure() async throws {
        let rt = try await runtime(failing: true)
        await rt.renameApp(apps[0], to: "仕事の家計簿")
        XCTAssertEqual(rt.apps.first { $0.id == "a" }?.name, "家計簿", "失敗したら元の名前に戻す")
        XCTAssertNotNil(rt.toast)
    }

    func testDuplicateAddsCopyWithoutSwitching() async throws {
        let rt = try await runtime()
        let current = rt.document.appId
        await rt.duplicateApp(apps[0])
        let copy = try XCTUnwrap(rt.apps.first { !["a", "b", "c"].contains($0.id) })
        XCTAssertNotEqual(copy.id, "a")
        XCTAssertTrue(rt.visibleApps.contains(copy))
        XCTAssertEqual(rt.document.appId, current, "複製しても切り替えない")
    }

    func testDuplicateGoesRightAfterSourceWhenManuallyOrdered() async throws {
        let rt = try await runtime()
        await rt.moveApp("c", to: 0)
        await rt.duplicateApp(apps[0])
        let ids = rt.visibleApps.map(\.id).filter { $0 != rt.document.appId }
        let copy = try XCTUnwrap(rt.apps.first { !["a", "b", "c"].contains($0.id) }?.id)
        XCTAssertEqual(ids.firstIndex(of: copy), ids.firstIndex(of: "a").map { $0 + 1 }, "\(ids)")
    }

    func testMoveReordersAndPersistsSortIndex() async throws {
        let rt = try await runtime()
        let before = rt.visibleApps.map(\.id)
        let last = try XCTUnwrap(before.last)
        await rt.moveApp(last, to: 0)
        XCTAssertEqual(rt.visibleApps.first?.id, last)
        let listed = try await rt.services.appConfig.listApps()
        XCTAssertEqual(listed.first { $0.id == last }?.sortIndex, 1, "サーバー(モック)に順番が残る")
    }

    func testMoveRollsBackOnFailure() async throws {
        let rt = try await runtime(failing: true)
        let before = rt.visibleApps.map(\.id)
        await rt.moveApp(before.last!, to: 0)
        XCTAssertEqual(rt.visibleApps.map(\.id), before, "失敗したら元の順に戻す")
        XCTAssertNotNil(rt.toast)
    }
}
