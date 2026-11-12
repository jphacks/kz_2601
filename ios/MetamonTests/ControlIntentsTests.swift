import XCTest
import MetamonDSL
import MetamonRenderer
@testable import Metamon

/// コントロール(iOS 18・#1993)を押した時の本体の動き・文言。
@MainActor
final class ControlIntentsTests: XCTestCase {
    private let json = """
    {"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home",
     "entities":[{"id":"expense","name":"支出","fields":[{"id":"amount","name":"金額","type":"number"}]},
                 {"id":"memo","name":"メモ","fields":[{"id":"text","name":"内容","type":"string"}]}],
     "screens":[{"id":"home","title":"家計簿","components":[
                  {"type":"list","entity":"expense","item":{"title":"amount"}},
                  {"type":"button","label":"追加","action":{"type":"navigate","screen":"add","presentation":"sheet"}}]},
                {"id":"add","title":"支出を追加","entity":"expense","components":[{"type":"numberField","field":"amount"}]}]}
    """

    private func runtime() throws -> AppRuntime {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let services = BuildConfig.make(environment: .mock)
        return AppRuntime(document: AppDocument(appId: "kakeibo-\(UUID().uuidString.prefix(6))", version: 1, dsl: dsl), services: services)
    }

    func testFormScreenIsTheEntitysForm() throws {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        XCTAssertEqual(AppRuntime.formScreen(for: "expense", in: dsl), "add")
        XCTAssertNil(AppRuntime.formScreen(for: "memo", in: dsl)) // フォーム画面の無い entity はアプリを開くだけ
    }

    func testRecordControlOpensTheForm() async throws {
        let rt = try runtime()
        await rt.openFromControl(appId: rt.document.appId, entityId: "expense")
        XCTAssertEqual(NavigationResolver.top(rt.navigationState, dsl: rt.dsl).screen, "add")
        XCTAssertFalse(rt.showAppSwitcher)
    }

    func testOpenAppControlOnCurrentAppStaysOnRoot() async throws {
        let rt = try runtime()
        await rt.openFromControl(appId: rt.document.appId, entityId: nil)
        XCTAssertNil(rt.sheet)
        XCTAssertEqual(NavigationResolver.top(rt.navigationState, dsl: rt.dsl).screen, "home")
    }

    /// 未選択・消したアプリは一覧を開く(落ちない)
    func testMissingAppOpensSwitcher() async throws {
        let rt = try runtime()
        await rt.openFromControl(appId: nil, entityId: "expense")
        let opened = expectation(description: "switcher")
        Task {
            for _ in 0..<50 where !rt.showAppSwitcher { try? await Task.sleep(for: .milliseconds(20)) }
            opened.fulfill()
        }
        await fulfillment(of: [opened], timeout: 3)
        XCTAssertTrue(rt.showAppSwitcher)
        XCTAssertNil(rt.sheet)
    }

    func testIntentsCarryIds() {
        XCTAssertEqual(OpenAppFromControlIntent(appId: "a1").appId, "a1")
        let record = OpenRecordFromControlIntent(appId: "a1", entityId: "expense")
        XCTAssertEqual(record.appId, "a1")
        XCTAssertEqual(record.entityId, "expense")
        XCTAssertTrue(OpenAppFromControlIntent.openAppWhenRun)
        XCTAssertTrue(OpenRecordFromControlIntent.openAppWhenRun)
    }

    func testCopyWording() {
        // 端末の言語に左右されないよう、訳す前のキー(ソースの日本語)で見る
        #if APP_STORE_REVIEW
        XCTAssertEqual(OpenAppFromControlIntent.title.key, "記録帳を開く")
        XCTAssertFalse(ActionButtonHintSection.hint.contains("作った"))
        #else
        XCTAssertEqual(OpenAppFromControlIntent.title.key, "アプリを開く")
        #endif
    }

    func testCopyHasEnglish() throws {
        let path = try XCTUnwrap(Bundle(for: AppRuntime.self).path(forResource: "en", ofType: "lproj"))
        let en = try XCTUnwrap(Bundle(path: path))
        let keys = [
            OpenAppFromControlIntent.title.key, OpenRecordFromControlIntent.title.key, "アクションボタン",
            "アクションボタンのある iPhone では、設定アプリの「アクションボタン」で「ショートカット」を選び、Tailor It の「アプリを開く」を割り当てると、押すだけで作ったアプリを開けます。",
            "アクションボタンのある iPhone では、設定アプリの「アクションボタン」で「ショートカット」を選び、Tailor It の「記録帳を開く」を割り当てると、押すだけで記録帳を開けます。",
        ]
        for key in keys {
            let value = en.localizedString(forKey: key, value: "__missing__", table: nil)
            XCTAssertNotEqual(value, "__missing__", "英語が無い: \(key)")
            XCTAssertNotEqual(value, key, "英語が日本語のまま: \(key)")
        }
    }
}
