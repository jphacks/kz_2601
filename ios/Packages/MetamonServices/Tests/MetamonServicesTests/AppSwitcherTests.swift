import XCTest
@testable import MetamonServices

/// #51 複数アプリ切替: 発話の意図判定・名前照合・一覧の絞り込み・オフラインキャッシュ。
final class AppSwitcherTests: XCTestCase {
    // MARK: VoiceIntent

    func testSwitchAppIntent() {
        XCTAssertEqual(VoiceIntent.detect("家計簿に切り替えて"), .switchApp(name: "家計簿"))
        XCTAssertEqual(VoiceIntent.detect("家計簿アプリに切り替えて。"), .switchApp(name: "家計簿"))
        XCTAssertEqual(VoiceIntent.detect("筋トレ を 開いて"), .switchApp(name: "筋トレ"))
        XCTAssertEqual(VoiceIntent.detect("じゃあ散歩メモに変身して"), .switchApp(name: "散歩メモ"))
        XCTAssertEqual(VoiceIntent.detect("家計簿の方に切替"), .switchApp(name: "家計簿"))
    }

    func testListAppsIntent() {
        XCTAssertEqual(VoiceIntent.detect("アプリ一覧"), .listApps)
        XCTAssertEqual(VoiceIntent.detect("変身一覧を見せて"), .listApps)
        XCTAssertEqual(VoiceIntent.detect("アプリのリストを出して"), .listApps)
        // 名前の無い切替は一覧
        XCTAssertEqual(VoiceIntent.detect("別のアプリに切り替えて"), .listApps)
        XCTAssertEqual(VoiceIntent.detect("他のアプリを開いて"), .listApps)
        XCTAssertEqual(VoiceIntent.detect("アプリを切り替えて"), .listApps)
    }

    func testExistingIntentsUnchanged() {
        XCTAssertEqual(VoiceIntent.detect("元に戻して"), .undo)
        XCTAssertEqual(VoiceIntent.detect("前の状態に戻して"), .undo)
        XCTAssertEqual(VoiceIntent.detect("別のアプリ。家計簿が欲しい"), .newApp)
        XCTAssertEqual(VoiceIntent.detect("新しいアプリに切り替えて"), .newApp)
        XCTAssertEqual(VoiceIntent.detect("ダークモードにして"), .morph)
        XCTAssertEqual(VoiceIntent.detect("ボタンを右に戻して"), .morph)
        XCTAssertEqual(VoiceIntent.detect("一覧画面にグラフを足して"), .morph)
    }

    // MARK: AppNameMatch

    func testNameMatchTiers() {
        let names = ["家計簿", "筋トレ記録", "散歩の写真メモ", "家計簿(旅行用)"]
        XCTAssertEqual(AppNameMatch.match("家計簿", in: names), .one(0))              // 完全一致が前方一致より優先
        XCTAssertEqual(AppNameMatch.match("筋トレ", in: names), .one(1))              // 前方一致
        XCTAssertEqual(AppNameMatch.match("写真メモ", in: names), .one(2))            // 部分一致
        XCTAssertEqual(AppNameMatch.match("家計簿アプリ", in: names), .one(0))        // 「アプリ」は無視
        XCTAssertEqual(AppNameMatch.match("家計", in: names), .ambiguous([0, 3]))    // 曖昧
        XCTAssertEqual(AppNameMatch.match("日記", in: names), .none)
        XCTAssertEqual(AppNameMatch.match("", in: names), .none)
    }

    // MARK: AppSummary.visible

    func testVisibleFiltersArchivedAndBlank() {
        let apps = [
            AppSummary(id: "a", name: "家計簿", currentVersion: 3, updatedAt: "2026-10-24T01:00:00Z"),
            AppSummary(id: "b", name: "筋トレ", currentVersion: 5, updatedAt: "2026-10-24T03:00:00Z"),
            AppSummary(id: "c", name: "古いやつ", currentVersion: 2, status: "archived", updatedAt: "2026-10-24T04:00:00Z"),
            AppSummary(id: "d", name: "はじめてのアプリ", currentVersion: 1, updatedAt: "2026-10-24T05:00:00Z"),
            AppSummary(id: "a", name: "家計簿(重複)", currentVersion: 3),
        ]
        let visible = AppSummary.visible(apps, current: "a")
        XCTAssertEqual(visible.map(\.id), ["a", "b"], "現在のアプリが先頭、archived と未生成(v1)は除外、重複は 1 つ")

        // 現在のアプリが未生成(v1)でも一覧には残す
        let withBlankCurrent = AppSummary.visible(apps, current: "d")
        XCTAssertEqual(withBlankCurrent.map(\.id), ["d", "b", "a"])
    }

    // MARK: AppListCache

    func testAppListCacheRoundTrip() throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-test-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: dir) }
        let cache = AppListCache(directory: dir)
        XCTAssertEqual(cache.load(), [])
        let apps = [AppSummary(id: "x", name: "家計簿", currentVersion: 4, updatedAt: "2026-10-24T01:02:03.456Z")]
        cache.store(apps)
        XCTAssertEqual(cache.load(), apps)
        XCTAssertNotNil(cache.load().first?.updatedDate, "小数秒付き ISO8601 も読める")
        cache.clear()
        XCTAssertEqual(cache.load(), [])
    }
}
