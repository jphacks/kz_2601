import XCTest
import MetamonDSL
@testable import MetamonServices

/// #277 変身一覧の整理: 名前の検査・複製名・並べ替え・自分で並べた順の表示・インメモリのクライアント。
final class AppListEditTests: XCTestCase {
    private let apps = [
        AppSummary(id: "a", name: "家計簿", currentVersion: 3, updatedAt: "2026-10-24T09:00:00Z"),
        AppSummary(id: "b", name: "買い物", currentVersion: 2, updatedAt: "2026-10-25T09:00:00Z"),
        AppSummary(id: "c", name: "日記", currentVersion: 4, updatedAt: "2026-10-23T09:00:00Z"),
    ]

    // MARK: names

    func testNormalizedName() {
        XCTAssertEqual(AppListEdit.normalizedName("  仕事の家計簿 \n"), "仕事の家計簿")
        XCTAssertNil(AppListEdit.normalizedName("   "))
        XCTAssertEqual(AppListEdit.normalizedName(String(repeating: "あ", count: 40))?.count, 40)
        XCTAssertNil(AppListEdit.normalizedName(String(repeating: "あ", count: 41)))
    }

    func testCopyNameFitsLimit() {
        XCTAssertEqual(AppListEdit.copyName(of: "家計簿") { "\($0)のコピー" }, "家計簿のコピー")
        let long = AppListEdit.copyName(of: String(repeating: "あ", count: 40)) { "\($0)のコピー" }
        XCTAssertEqual(long.count, AppListEdit.maxNameLength)
        XCTAssertTrue(long.hasSuffix("のコピー"))
    }

    // MARK: order

    func testMoved() {
        XCTAssertEqual(AppListEdit.moved(["a", "b", "c"], id: "c", to: 0), ["c", "a", "b"])
        XCTAssertEqual(AppListEdit.moved(["a", "b", "c"], id: "a", to: 1), ["b", "a", "c"])
        XCTAssertEqual(AppListEdit.moved(["a", "b", "c"], id: "a", to: 99), ["b", "c", "a"])
        XCTAssertEqual(AppListEdit.moved(["a", "b"], id: "x", to: 0), ["a", "b"])
    }

    func testVisibleWithoutManualOrderIsCurrentThenNewest() {
        XCTAssertEqual(AppSummary.visible(apps, current: "c").map(\.id), ["c", "b", "a"])
    }

    func testVisibleFollowsManualOrderAndPutsNewAppsFirst() {
        var list = AppListEdit.applyingOrder(["c", "a", "b"], to: apps)
        XCTAssertEqual(AppSummary.visible(list, current: "b").map(\.id), ["c", "a", "b"], "並べた順(使用中を先頭に寄せない)")
        list.append(AppSummary(id: "new", name: "新しい", currentVersion: 2, updatedAt: "2026-10-20T09:00:00Z"))
        XCTAssertEqual(AppSummary.visible(list, current: "b").map(\.id), ["new", "c", "a", "b"], "並べた後に増えたアプリは先頭")
    }

    func testSortIndexDecodesAndIsOptional() throws {
        let json = #"[{"id":"a","name":"x","currentVersion":2,"status":"active","sortIndex":3},{"id":"b","name":"y","currentVersion":2,"status":"active"}]"#
        let decoded = try JSONDecoder().decode([AppSummary].self, from: Data(json.utf8))
        XCTAssertEqual(decoded.map(\.sortIndex), [3, nil])
    }

    // MARK: in-memory client

    func testInMemoryRenameDuplicateReorder() async throws {
        let client = InMemoryAppConfigClient()
        await client.setApps(apps)

        let renamed = try await client.renameApp(appId: "a", name: "仕事の家計簿")
        XCTAssertEqual(renamed.name, "仕事の家計簿")

        let copy = try await client.duplicateApp(appId: "a", name: "仕事の家計簿のコピー")
        XCTAssertNotEqual(copy.id, "a")
        XCTAssertFalse(copy.isBlank, "複製は未生成扱いにしない(一覧に出る)")

        try await client.reorderApps(ids: [copy.id, "c", "a", "b"])
        let listed = try await client.listApps()
        XCTAssertEqual(AppSummary.visible(listed, current: nil).map(\.id), [copy.id, "c", "a", "b"])
    }

    func testDefaultClientIsUnsupported() async {
        struct Bare: AppConfigClient {
            func fetchDocument(appId: String) async throws -> AppDocument { throw AppListEdit.unsupported }
            func listVersions(appId: String) async throws -> [VersionSummary] { [] }
            func restore(appId: String, toVersion version: Int) async throws -> AppDocument { throw AppListEdit.unsupported }
            func list(appId: String, entity: String) async throws -> [Record] { [] }
            func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record { throw AppListEdit.unsupported }
            func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record { throw AppListEdit.unsupported }
            func delete(appId: String, entity: String, id: String) async throws {}
        }
        do {
            _ = try await Bare().renameApp(appId: "a", name: "x")
            XCTFail("既定は unsupported を投げる")
        } catch {
            XCTAssertTrue(error is AppListEdit.Unsupported)
        }
    }
}
