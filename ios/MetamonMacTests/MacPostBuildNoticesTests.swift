import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 完成の後のお知らせの順番(#968)。最初のアプリの後の直し方の例(#1278)は要約が閉じてから出る。
@MainActor
final class MacPostBuildNoticesTests: XCTestCase {
    func testEditHintWaitsForSummary() {
        let notices = MacPostBuildNotices()
        notices.summary = .init(appId: "a", text: "できました", action: nil)
        notices.showsEditHint = true
        XCTAssertEqual(notices.active, .summary)

        notices.dismissActive()
        XCTAssertNil(notices.summary)
        XCTAssertEqual(notices.active, .editHint)

        notices.dismissActive()
        XCTAssertFalse(notices.showsEditHint)
        XCTAssertNil(notices.active)
    }

    /// 写し(#1281)は、要約が閉じた後に出る直し方の例(#1278)が閉じるまで撮らない
    func testIsShowingUntilEditHintCloses() {
        let notices = MacPostBuildNotices()
        XCTAssertFalse(notices.isShowing)
        notices.summary = .init(appId: "a", text: "できました", action: nil)
        notices.showsEditHint = true
        notices.dismissActive()
        XCTAssertNil(notices.summary)
        XCTAssertTrue(notices.isShowing, "要約が閉じても直し方の例が出ている")
        notices.dismissActive()
        XCTAssertFalse(notices.isShowing)
    }

    func testEditHintAloneIsActive() {
        let notices = MacPostBuildNotices()
        notices.showsEditHint = true
        XCTAssertEqual(notices.active, .editHint)
    }

    // MARK: - 公開の確認(#253)・保存期間(#482)(#979)

    private let fields = [PublicPersonalField(entity: "booking", entityName: "予約", field: "name", fieldName: "お名前", kind: "name"),
                          PublicPersonalField(entity: "booking", entityName: "予約", field: "phone", fieldName: "電話番号", kind: "phone")]

    private func defaults() -> UserDefaults {
        let name = "mac979-\(UUID().uuidString)"
        let d = UserDefaults(suiteName: name)!
        d.removePersistentDomain(forName: name)
        return d
    }

    /// kakeibo の「支出」に保存期間を付けた DSL
    private func retentionDSL(days: Int = 30) throws -> AppDSL {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        var obj = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(contentsOf: url)) as? [String: Any])
        var entities = try XCTUnwrap(obj["entities"] as? [[String: Any]])
        entities[0]["retentionDays"] = days
        obj["entities"] = entities
        return try JSONDecoder().decode(AppDSL.self, from: JSONSerialization.data(withJSONObject: obj))
    }

    /// iOS と同じ順: 要約 → 公開の確認 → 保存期間 → 直し方の例
    func testOrderSummaryPublicDataRetentionEditHint() throws {
        let notices = MacPostBuildNotices()
        notices.summary = .init(appId: "a", text: "できました", action: nil)
        notices.showsEditHint = true
        notices.notePublicData(fields, appId: "a", version: 2)
        notices.noteRetention(appId: "a", version: 2, dsl: try retentionDSL(), defaults: defaults())
        XCTAssertEqual(notices.active, .summary)
        notices.dismissActive()
        XCTAssertEqual(notices.active, .publicData)
        notices.dismissActive()
        XCTAssertNil(notices.publicData, "閉じる = 公開のまま")
        XCTAssertEqual(notices.active, .retention)
        notices.dismissActive()
        XCTAssertNil(notices.retention, "閉じる = あとで")
        XCTAssertEqual(notices.active, .editHint)
        notices.dismissActive()
        XCTAssertNil(notices.top)
    }

    func testNoPublicDataWithoutFields() {
        let notices = MacPostBuildNotices()
        notices.notePublicData([], appId: "a", version: 1)
        XCTAssertNil(notices.publicData)
        XCTAssertNil(notices.active)
    }

    func testPublicDataButtonsReturnFixInstruction() {
        let notices = MacPostBuildNotices()
        notices.notePublicData(fields, appId: "a", version: 2)
        XCTAssertEqual(notices.resolvePublicData(.remove, currentAppId: "a", currentVersion: 2, english: false),
                       "予約の「お名前」と「電話番号」の欄を消して")
        XCTAssertNil(notices.publicData)

        notices.notePublicData(fields, appId: "a", version: 2)
        XCTAssertEqual(notices.resolvePublicData(.makePrivate, currentAppId: "a", currentVersion: 2, english: false),
                       "予約を自分だけの記録にして")
    }

    func testPublicDataKeepSendsNothing() {
        let notices = MacPostBuildNotices()
        notices.notePublicData(fields, appId: "a", version: 2)
        XCTAssertNil(notices.resolvePublicData(.keep, currentAppId: "a", currentVersion: 2, english: false))
        XCTAssertNil(notices.publicData)
    }

    /// 別のアプリ・別の版に変わっていたら送らない(iOS の `takePublicDataPrompt` と同じ)
    func testPublicDataStaleSendsNothing() {
        let notices = MacPostBuildNotices()
        notices.notePublicData(fields, appId: "a", version: 2)
        XCTAssertNil(notices.resolvePublicData(.remove, currentAppId: "a", currentVersion: 3, english: false))
        notices.notePublicData(fields, appId: "a", version: 2)
        XCTAssertNil(notices.resolvePublicData(.remove, currentAppId: "b", currentVersion: 2, english: false))
    }

    func testRetentionConfirmStoresConsentPerApp() throws {
        let d = defaults()
        let notices = MacPostBuildNotices()
        notices.noteRetention(appId: "a", version: 1, dsl: try retentionDSL(days: 30), defaults: d)
        XCTAssertEqual(notices.retention?.items.map(\.entity), ["expense"])
        XCTAssertEqual(notices.confirmRetention(defaults: d), "a")
        XCTAssertEqual(RetentionConsent.days(appId: "a", defaults: d), ["expense": 30])
        XCTAssertEqual(RetentionConsent.days(appId: "b", defaults: d), [:], "確認はアプリごと")
        notices.noteRetention(appId: "a", version: 1, dsl: try retentionDSL(days: 30), defaults: d)
        XCTAssertNil(notices.retention, "確認済みは聞き直さない")
        notices.noteRetention(appId: "a", version: 2, dsl: try retentionDSL(days: 7), defaults: d)
        XCTAssertNotNil(notices.retention, "期間が変わったら聞き直す")
    }

    func testRetentionLaterDoesNotAskAgainThisSession() throws {
        let d = defaults()
        let notices = MacPostBuildNotices()
        notices.noteRetention(appId: "a", version: 1, dsl: try retentionDSL(), defaults: d)
        notices.deferRetention()
        XCTAssertNil(notices.retention)
        XCTAssertEqual(RetentionConsent.days(appId: "a", defaults: d), [:], "あとで = 消さない")
        notices.noteRetention(appId: "a", version: 1, dsl: try retentionDSL(), defaults: d)
        XCTAssertNil(notices.retention)
        notices.resetAfterWipe()
        notices.noteRetention(appId: "a", version: 1, dsl: try retentionDSL(), defaults: d)
        XCTAssertNotNil(notices.retention, "サインアウトの後はあらためて聞く")
    }

    func testRetentionStopReturnsInstruction() throws {
        let notices = MacPostBuildNotices()
        notices.noteRetention(appId: "a", version: 1, dsl: try retentionDSL(), defaults: defaults())
        XCTAssertEqual(notices.stopRetention(english: false), "記録を自動で消すのをやめて")
        XCTAssertNil(notices.retention)
        XCTAssertNil(notices.stopRetention(english: false), "出ていなければ何も送らない")
        XCTAssertEqual(MacPostBuildNotices.stopRetentionInstruction(entityName: "支出", english: false), "「支出」の記録を自動で消すのをやめて")
    }
}
