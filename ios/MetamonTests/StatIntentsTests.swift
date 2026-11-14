import XCTest
import AppIntents
import MetamonDSL
import MetamonServices
@testable import Metamon

/// #1994: ショートカットの「数字を読む」はウィジェットと同じ値を返す。
@MainActor
final class StatIntentsTests: XCTestCase {
    private func dsl(stats: String) throws -> AppDSL {
        let json = """
        {"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home",
         "entities":[{"id":"expense","name":"支出","titleField":"memo","fields":[
           {"id":"amount","name":"金額","type":"currency","unit":"円"},
           {"id":"memo","name":"メモ","type":"string"}]}],
         "screens":[{"id":"home","title":"家計簿","components":[\(stats)]}]}
        """
        return try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    private let sum = #"{"type":"stat","label":"支出の合計","entity":"expense","aggregate":"sum","field":"amount","format":"currency"}"#
    private let count = #"{"type":"stat","label":"件数","entity":"expense","aggregate":"count","filter":{"field":"amount","op":"gte","value":400}}"#
    private let latest = #"{"type":"stat","label":"最新","entity":"expense","aggregate":"latest","field":"memo"}"#
    private let expr = #"{"type":"stat","label":"目標","value":120}"#

    private var records: [String: [Record]] {
        ["expense": [
            Record(id: "a", data: ["amount": .number(500), "memo": .string("コンビニ")], createdAt: "2026-10-31T01:00:00Z"),
            Record(id: "b", data: ["amount": .number(300), "memo": .string("電車")], createdAt: "2026-10-31T00:00:00Z"),
            Record(id: "c", data: ["amount": .number(1000), "memo": .string("ランチ")], createdAt: "2026-10-29T03:00:00Z"),
        ]]
    }

    private func read(_ c: Component, _ d: AppDSL, _ r: [String: [Record]]) -> StatReading.Value? {
        StatReading.read(c, dsl: d, records: r, countUnit: WidgetSnapshotWriter.countUnit)
    }

    func testTextMatchesWidgetSnapshot() throws {
        let d = try dsl(stats: [sum, count, latest].joined(separator: ","))
        let widget = WidgetSnapshotWriter.makeSnapshot(dsl: d, records: records, previous: nil)
        let stats = StatReading.stats(in: d)
        XCTAssertEqual(stats.count, 3)
        for (c, w) in zip(stats, widget.stats) {
            XCTAssertEqual(read(c, d, records)?.text, w.value, "ウィジェットと同じ文字列: \(c.label ?? "")")
            XCTAssertEqual(WidgetSnapshotWriter.statKey(c), w.key, "見分け方もウィジェットと同じ")
        }
    }

    func testNumbers() throws {
        let d = try dsl(stats: [sum, count, latest, expr].joined(separator: ","))
        let s = StatReading.stats(in: d)
        XCTAssertEqual(read(s[0], d, records)?.number, 1800)
        XCTAssertEqual(read(s[1], d, records)?.number, 2, "filter は stat と同じに掛ける")
        XCTAssertNil(read(s[2], d, records)?.number, "latest は数で取れない")
        XCTAssertEqual(read(s[2], d, records)?.text, "コンビニ")
        XCTAssertEqual(read(s[3], d, [:])?.number, 120, "式の stat は記録が無くても読める")
    }

    func testNotLoadedIsNil() throws {
        let d = try dsl(stats: sum)
        XCTAssertNil(read(StatReading.stats(in: d)[0], d, [:]), "記録が未取得なら 0 と言い切らない")
    }

    /// id を書かない stat は読むたびに id が変わる。key は中身から作るので、DSL を読み直しても同じ stat を引ける
    func testKeyIsStableAcrossDecodes() throws {
        let first = try dsl(stats: sum), second = try dsl(stats: sum)
        let key = WidgetSnapshotWriter.statKey(StatReading.stats(in: first)[0])
        XCTAssertNotNil(StatReading.stat(key, in: second))
        XCTAssertNil(StatReading.stat("expense|sum|amount|消した数字", in: second))
    }

    func testEntityIdAndTitle() throws {
        let d = try dsl(stats: sum)
        let app = AppSummary(id: "app1", name: "家計簿", currentVersion: 2)
        let e = StatEntity(app: app, stat: StatReading.stats(in: d)[0], dsl: d)
        XCTAssertEqual(e.title, "家計簿 › 支出の合計")
        XCTAssertEqual(StatEntity.split(e.id)?.appId, "app1")
        XCTAssertEqual(StatEntity.split(e.id)?.statKey, e.statKey)
        let missing = StatEntity(missingId: e.id)
        XCTAssertTrue(missing.isMissing)
        XCTAssertEqual(missing.appId, "app1")
    }

    func testLabelFallsBackToEntityName() throws {
        let d = try dsl(stats: #"{"type":"stat","entity":"expense","aggregate":"count"}"#)
        XCTAssertEqual(StatReading.label(of: StatReading.stats(in: d)[0], in: d), "支出")
    }

    /// 暗号化した field の stat は値を返さず、記録も読まない(一覧のひと目の数字と同じ判定)
    func testEncryptedStatIsLocked() async throws {
        let json = """
        {"schemaVersion":1,"app":{"name":"日記"},"initialScreen":"home",
         "entities":[{"id":"diary","name":"日記","titleField":"body","fields":[
           {"id":"body","name":"本文","type":"text","encrypted":true}]}],
         "screens":[{"id":"home","title":"日記","components":[
           {"type":"stat","label":"最新","entity":"diary","aggregate":"latest","field":"body"}]}]}
        """
        let d = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let c = try XCTUnwrap(StatReading.stats(in: d).first)
        XCTAssertTrue(AppGlance.touchesEncrypted(c, in: d))
        let rows = ["diary": [Record(id: "a", data: ["body": .string("ひみつ")], createdAt: "2026-10-31T01:00:00Z")]]
        let glance = AppGlance.Value(statId: c.id, label: "最新", value: "ひみつ", at: .now, statKey: WidgetSnapshotWriter.statKey(c))
        var readRecords = false
        let result = await StatLookupRunner.resolve(c, dsl: d, glance: glance) {
            readRecords = true
            return rows
        }
        XCTAssertEqual(result, .locked, "記録・一覧の値があっても返さない")
        XCTAssertFalse(readRecords, "記録を読まない")
        XCTAssertFalse(String(localized: GetStatValueIntent.lockedValue).contains("🔒"), "Siri に記号を読ませない")
    }

    func testPlainStatResolvesValue() async throws {
        let d = try dsl(stats: sum)
        let c = StatReading.stats(in: d)[0]
        let result = await StatLookupRunner.resolve(c, dsl: d, glance: nil) { self.records }
        XCTAssertEqual(result, .value(.init(text: read(c, d, records)!.text, number: 1800)))
    }

    /// 記録を読むので、端末のロック中は解いてから
    func testIntentsRequireAuthentication() {
        guard case .requiresAuthentication = GetStatValueIntent.authenticationPolicy else { return XCTFail("値") }
        guard case .requiresAuthentication = GetStatNumberIntent.authenticationPolicy else { return XCTFail("数値") }
    }

    /// 一覧のひと目の数字は statKey で突き合わせる(id を書かない stat は DSL を読み直すと id が変わる)
    func testCachedGlanceMatchesByKey() async throws {
        let first = try dsl(stats: sum), second = try dsl(stats: sum)
        let saved = try XCTUnwrap(AppGlance.compute(dsl: first, records: records, setting: nil))
        let c = StatReading.stats(in: second)[0]
        XCTAssertNotEqual(saved.statId, c.id, "id を書かない stat は読み直すと id が変わる")
        XCTAssertEqual(StatLookupRunner.cachedGlance(saved, for: c), saved.value)
        let result = await StatLookupRunner.resolve(c, dsl: second, glance: saved) { nil }
        XCTAssertEqual(result, .cached(saved.value))

        var other = saved
        other.statKey = "expense|count||件数"
        XCTAssertNil(StatLookupRunner.cachedGlance(other, for: c), "別の stat の値は使わない")
        var legacy = saved
        legacy.statKey = nil
        XCTAssertNil(StatLookupRunner.cachedGlance(legacy, for: c), "key の無い古い保存値は使わない")
    }

    /// 消えたアプリ・stat は落ちずに分かる文で失敗する
    func testMissingTargetThrowsStatNotFound() async throws {
        do {
            _ = try await StatLookupRunner.run(StatEntity(missingId: "gone/expense|sum|amount|x"))
            XCTFail("投げるはず")
        } catch TailorIntentError.statNotFound {
        } catch {
            XCTFail("別のエラー: \(error)")
        }
    }

    // MARK: - id の組み立てと分解(#2241)

    /// `id` と `split` は往復する。`/` は最初の 1 つだけが区切りなので、statKey に `/` が入っても壊れない
    func testIdRoundTripsThroughSplit() {
        let cases: [(appId: String, statKey: String)] = [
            ("app1", "expense|sum|amount|支出の合計"),
            ("app1", "expense|sum|amount|今月の支出/予算"),
            ("app1", "expense|sum|amount|a/b/c"),
            ("app1", "/先頭が区切り"),
            ("app1", ""),
        ]
        for (appId, statKey) in cases {
            let id = StatEntity.id(appId: appId, statKey: statKey)
            let parts = StatEntity.split(id)
            XCTAssertEqual(parts?.appId, appId, "appId: \(id)")
            XCTAssertEqual(parts?.statKey, statKey, "statKey: \(id)")
        }
    }

    /// 区切りが無い id は分解できない(別の数字を当てずに nil を返す)
    func testSplitRejectsIdWithoutSeparator() {
        XCTAssertNil(StatEntity.split(""), "空の id")
        XCTAssertNil(StatEntity.split("app1"), "appId だけ")
        XCTAssertNil(StatEntity.split("expense|sum|amount|支出の合計"), "statKey だけ")
    }

    /// 分解できない id でも実体は id を落とさず、`isMissing` で残す
    func testMissingEntityKeepsRawId() {
        let broken = StatEntity(missingId: "appIdだけ")
        XCTAssertEqual(broken.id, "appIdだけ", "id は原文のまま残す")
        XCTAssertEqual(broken.appId, "appIdだけ")
        XCTAssertEqual(broken.statKey, "")
        XCTAssertTrue(broken.isMissing)
        XCTAssertEqual(broken.title, String(localized: "見つからない数字"))

        let empty = StatEntity(missingId: "")
        XCTAssertEqual(empty.id, "")
        XCTAssertEqual(empty.appId, "")
        XCTAssertEqual(empty.statKey, "")
        XCTAssertTrue(empty.isMissing)

        let slashed = StatEntity(missingId: "app1/expense|sum|amount|今月の支出/予算")
        XCTAssertEqual(slashed.appId, "app1")
        XCTAssertEqual(slashed.statKey, "expense|sum|amount|今月の支出/予算", "statKey の `/` を切り落とさない")
    }

    /// `/` を含む題の stat も、id を分解した statKey で引き直せる(ショートカットに保存した id からの復元)
    func testStatKeyWithSlashResolvesBackToStat() throws {
        let slashLabel = #"{"type":"stat","label":"今月の支出/予算","entity":"expense","aggregate":"sum","field":"amount"}"#
        let d = try dsl(stats: slashLabel)
        let app = AppSummary(id: "app1", name: "家計簿", currentVersion: 2)
        let e = StatEntity(app: app, stat: StatReading.stats(in: d)[0], dsl: d)
        XCTAssertTrue(e.statKey.contains("/"))
        let parts = try XCTUnwrap(StatEntity.split(e.id))
        XCTAssertEqual(parts.appId, "app1")
        XCTAssertEqual(parts.statKey, e.statKey)
        XCTAssertNotNil(StatReading.stat(parts.statKey, in: d), "保存した id から同じ stat を引ける")
    }

    // MARK: - 選べる数字は一覧のひと目の数字と同じ(#2241)

    /// `StatReading.stats` は `AppGlance.choices` そのもの(ウィジェット・一覧・ショートカットで並びと中身がそろう)
    func testStatsAreAppGlanceChoices() throws {
        let d = try dsl(stats: [sum, count, latest, expr].joined(separator: ","))
        let stats = StatReading.stats(in: d)
        let choices = AppGlance.choices(in: d)
        XCTAssertEqual(stats.count, choices.count)
        XCTAssertEqual(stats.map(\.id), choices.map(\.id))
        XCTAssertEqual(stats.map { WidgetSnapshotWriter.statKey($0) }, choices.map { WidgetSnapshotWriter.statKey($0) })
    }

    /// 初期状態(音声画面のみ)のアプリは選べる数字が無い。ここも `AppGlance.choices` と同じ答え
    func testVoiceOnlyAppHasNoStats() throws {
        let json = """
        {"schemaVersion":1,"app":{"name":"家計簿","voiceEntry":"fullscreen"},"initialScreen":"home","entities":[],
         "screens":[{"id":"home","title":"家計簿","components":[\(sum)]}]}
        """
        let d = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        XCTAssertTrue(d.isVoiceOnly)
        XCTAssertTrue(AppGlance.choices(in: d).isEmpty)
        XCTAssertTrue(StatReading.stats(in: d).isEmpty, "一覧のひと目の数字と同じ判定")
        XCTAssertNil(StatReading.stat("expense|sum|amount|支出の合計", in: d))
    }

    // MARK: - 題の無い stat の label(#2241)

    /// label → entity 名 → 「数字」の順に落ちる
    func testLabelFallbackOrder() throws {
        let labeled = try dsl(stats: sum)
        XCTAssertEqual(StatReading.label(of: StatReading.stats(in: labeled)[0], in: labeled), "支出の合計", "label があればそれ")

        let blank = try dsl(stats: #"{"type":"stat","label":"","entity":"expense","aggregate":"count"}"#)
        XCTAssertEqual(StatReading.label(of: StatReading.stats(in: blank)[0], in: blank), "支出", "空の label は無いものとして entity 名")

        let noEntity = try dsl(stats: #"{"type":"stat","value":120}"#)
        XCTAssertEqual(StatReading.label(of: StatReading.stats(in: noEntity)[0], in: noEntity),
                       String(localized: "数字"), "entity も無い式の stat は既定の題")

        let unknownEntity = try dsl(stats: #"{"type":"stat","entity":"消えたentity","aggregate":"count"}"#)
        XCTAssertEqual(StatReading.label(of: StatReading.stats(in: unknownEntity)[0], in: unknownEntity),
                       String(localized: "数字"), "DSL に無い entity を指していても落ちない")
    }

    /// 題の無い stat の実体名も label の決まりに従う
    func testEntityTitleUsesLabelFallback() throws {
        let d = try dsl(stats: #"{"type":"stat","entity":"expense","aggregate":"count"}"#)
        let app = AppSummary(id: "app1", name: "家計簿", currentVersion: 2)
        let e = StatEntity(app: app, stat: StatReading.stats(in: d)[0], dsl: d)
        XCTAssertEqual(e.label, "支出")
        XCTAssertEqual(e.title, "家計簿 › 支出")
    }
}
