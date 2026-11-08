import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// サーバーの新しいエラーコード(#317 plan_limit_reached / free_plan_paused・#321 text_not_allowed・#346 app_stopped)を
/// コードの文字列で見分け、決まった短い文にする。課金画面は自動で開かず、参加者には課金の導線を出さない(U3/U4)。
@MainActor
final class ErrorCodeLimitsTests: XCTestCase {
    private var saved: AppLanguage = .system

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    private func planLimitError(resource: String = "recordsPerApp", upgrade: Bool = true, byCreator: Bool = false) -> APIError {
        var details: [String: JSONValue] = ["resource": .string(resource), "plan": .string("free"), "limit": .number(5000),
                                            "used": .number(5000), "upgrade": .bool(upgrade)]
        if byCreator { details["reason"] = .string("creator_plan_limit") }
        return .server(status: 403, code: "plan_limit_reached", message: "このアプリを作った人のプランの上限に達しました", details: details)
    }

    // MARK: - plan_limit_reached

    func testPlanLimitDecodesDetails() throws {
        let limit = try XCTUnwrap(PlanLimit(planLimitError(byCreator: true)))
        XCTAssertEqual(limit, PlanLimit(resource: .recordsPerApp, plan: "free", limit: 5000, upgrade: true, byCreator: true))
        XCTAssertNil(PlanLimit(APIError.server(status: 403, code: "forbidden", message: "only the author")), "作った人以外の 403 とは別")
        XCTAssertEqual(PlanLimit(APIError.server(status: 403, code: "plan_limit_reached", message: ""))?.resource, .other)
    }

    func testPlanLimitIsNotReadOnlyOrBusy() {
        guard case .planLimit(let limit) = FailureKind.classify(planLimitError()) else {
            return XCTFail("plan_limit_reached は 403 forbidden / rejected として扱わない")
        }
        XCTAssertFalse(limit.byCreator)
        XCTAssertFalse(FailureKind.classify(planLimitError()).isRetryable)
    }

    func testPlanLimitCopyTable() {
        let table: [(PlanLimit, body: String, pro: Bool)] = [
            (PlanLimit(resource: .apps, upgrade: true), "作れるアプリの数が無料プランの上限に達しました。使わないものを消すか、Pro にすると増やせます", true),
            (PlanLimit(resource: .recordsPerApp, upgrade: true), "このアプリに保存できる記録の数が無料プランの上限に達しました。使わないものを消すか、Pro にすると増やせます", true),
            (PlanLimit(resource: .photosPerApp, upgrade: false), "このアプリに保存できる写真の数が上限に達しました。使わないものを消すと、続けて使えます", false),
            (PlanLimit(resource: .recordsPerApp, upgrade: true, byCreator: true), "このアプリに保存できる記録の数が上限に達しました。作った人に知らせてください", false),
            (PlanLimit(resource: .photosPerApp, upgrade: false, byCreator: true), "このアプリに保存できる写真の数が上限に達しました。作った人に知らせてください", false),
        ]
        for (limit, body, pro) in table {
            let card = ErrorPresentation(kind: .planLimit(limit), transcript: "")
            XCTAssertEqual(card.title, "上限に達しました")
            XCTAssertEqual(card.body, body, "\(limit)")
            XCTAssertEqual(card.primary, pro ? .upgradePro : nil, "Pro への導線は作成者本人だけ: \(limit)")
            XCTAssertFalse(card.body.contains("作った人以外"), "#378 の文言と混ぜない")
            XCTAssertNil(card.body.range(of: "[0-9]", options: .regularExpression), "上限の数は出さない")
        }
    }

    /// 参加者には「Pro」も課金画面も出さない(U3/U4)
    func testParticipantNeverSeesPro() {
        let limit = PlanLimit(planLimitError(byCreator: true))!
        XCTAssertFalse(ErrorCopy.showsProPath(limit))
        XCTAssertFalse(ErrorCopy.planLimitBody(limit).contains("Pro"))
    }

    func testPlanLimitInGenerateIsCardNotPaywall() {
        let card = ErrorPresentation.from(.classify(planLimitError(resource: "apps")), transcript: "家計簿")
        XCTAssertEqual(card?.kind, .planLimit(PlanLimit(resource: .apps, plan: "free", limit: 5000, upgrade: true)))
        XCTAssertEqual(card?.primary, .upgradePro, "押した時だけ課金画面")
    }

    // MARK: - free_plan_paused

    func testFreePlanPausedIsNotBusy() throws {
        let error = APIError.server(status: 503, code: "free_plan_paused", message: "本日は利用が集中したため…",
                                    details: ["reason": .string("daily_budget"), "resumeAt": .string("2026-11-08T15:00:00Z"),
                                              "retryAfterSec": .number(3600), APIError.retryAfterKey: .number(3600)])
        guard case .freePlanPaused(let at) = FailureKind.classify(error) else { return XCTFail("混雑(busy)にしない") }
        XCTAssertEqual(at, ISO8601DateFormatter().date(from: "2026-11-08T15:00:00Z"))
        let card = try XCTUnwrap(ErrorPresentation.from(.freePlanPaused(resumeAt: at), transcript: "x"))
        XCTAssertEqual(card.title, "無料プランを一時停止しています")
        XCTAssertEqual(ErrorCopy.freePlanPausedBody(resumeAt: at, timeZone: TimeZone(identifier: "Asia/Tokyo")!),
                       "利用が集中したため、無料プランの機能を一時的に止めています。11月9日 0:00に再開します")
        XCTAssertNil(card.primary, "もう一度では解けない")
        XCTAssertEqual(ErrorCopy.freePlanPausedBody(resumeAt: nil),
                       "利用が集中したため、無料プランの機能を一時的に止めています。しばらくしてからもう一度お試しください")
    }

    // MARK: - app_stopped

    func testAppStopped() {
        let error = APIError.server(status: 403, code: "app_stopped", message: "the operator stopped this app's server features",
                                    details: ["level": .string("app")])
        XCTAssertEqual(FailureKind.classify(error), .appStopped)
        XCTAssertEqual(ErrorCopy.nextStep(for: error), "このアプリの自動の動きは運営が止めています。この変更は保存できませんでした")
        XCTAssertFalse(ErrorCopy.appStoppedBody.contains("operator"), "サーバーの文・理由は出さない")
    }

    // MARK: - text_not_allowed

    func testTextNotAllowedNamesField() {
        let error = APIError.server(status: 422, code: "text_not_allowed", message: "表示できない言葉が含まれています",
                                    details: ["field": .string("description")])
        XCTAssertTrue(error.isTextNotAllowed)
        XCTAssertEqual(error.field, "description")
        XCTAssertEqual(ErrorCopy.nextStep(for: error), "表示できない言葉が含まれています")
        XCTAssertEqual(ShareAppSheet.textNotAllowedMessage(field: "description"), "「ひとこと説明」に表示できない言葉が含まれています")
        XCTAssertEqual(ShareAppSheet.textNotAllowedMessage(field: "tags"), "「タグ」に表示できない言葉が含まれています")
        XCTAssertEqual(ShareAppSheet.textNotAllowedMessage(field: nil), "表示できない言葉が含まれています")
    }

    /// 参加の表示名(#321): 参加の確認画面はこの文で表示名の欄を目立たせる
    func testJoinDisplayNameRejected() {
        let error = APIError.server(status: 422, code: "text_not_allowed", message: "表示できない言葉が含まれています",
                                    details: ["field": .string("displayName")])
        XCTAssertEqual(AppRuntime.importFailureMessage(error), "「表示名」に表示できない言葉が含まれています")
        XCTAssertEqual(AppRuntime.importFailureMessage(error), AppRuntime.displayNameRejected)
    }

    // MARK: - 同期キュー(記録の保存)

    func testSyncQueueEmitsDedicatedEvents() async throws {
        let cases: [(APIError, SyncEvent)] = [
            (planLimitError(byCreator: true),
             .planLimit(appId: "a", entity: "expense", limit: PlanLimit(resource: .recordsPerApp, plan: "free", limit: 5000, upgrade: true, byCreator: true))),
            (.server(status: 403, code: "app_stopped", message: "", details: ["level": .string("global")]),
             .appStopped(appId: "a", entity: "expense")),
            (.server(status: 422, code: "text_not_allowed", message: "表示できない言葉が含まれています", details: ["field": .string("memo")]),
             .textNotAllowed(appId: "a", entity: "expense", field: "memo")),
        ]
        for (error, expected) in cases {
            let dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-codes-\(UUID().uuidString)")
            defer { try? FileManager.default.removeItem(at: dir) }
            let store = RecordStore(directory: dir)
            let queue = SyncQueue(directory: dir, store: store, client: RefusingClient(error: error), policy: .immediate)
            await store.upsert(LocalRecord(appId: "a", entity: "expense", recordId: "local-e1", data: ["memo": "x"], pendingOp: .create, clientId: "e1"))
            let events = await queue.events()
            _ = await queue.enqueue(SyncOperation(id: "e1", appId: "a", entity: "expense", recordId: "local-e1", kind: .create, data: ["memo": "x"]))
            await queue.drain()
            var it = events.makeAsyncIterator()
            let first = await it.next()
            XCTAssertEqual(first, expected, "403 を「作った人だけ」に、422 を汎用の拒否にしない")
            let count = await queue.count
            XCTAssertEqual(count, 0, "再送しない")
            let rows = await store.records(appId: "a", entity: "expense")
            XCTAssertTrue(rows.isEmpty, "楽観的に足した行は戻す")
        }
    }

    // MARK: - 断られた保存に「保存しました」を残さない(M10)

    func testRefusedWriteRevokesSaveFeedback() throws {
        let doc = try DSLExamples.document("warikan", version: 1)
        let rt = AppRuntime(document: doc, services: BuildConfig.make(environment: .mock))
        let entity = try XCTUnwrap(rt.dsl.entity("expenses"))
        let refusals: [SyncEvent] = [
            .planLimit(appId: doc.appId, entity: "expenses", limit: PlanLimit(resource: .recordsPerApp)),
            .appStopped(appId: doc.appId, entity: "expenses"),
            .textNotAllowed(appId: doc.appId, entity: "expenses", field: "memo"),
        ]
        for event in refusals {
            let form = FormDraft(entity: entity)
            form.markSaved(queuedOffline: false)
            rt.lastSavedForm = .init(appId: doc.appId, entity: "expenses", form: form)
            rt.revokeSaveFeedback(for: event)
            XCTAssertNil(form.lastSaveResult, "保存バーの「保存しました」を取り消す: \(event)")
            XCTAssertNil(rt.lastSavedForm)
        }
    }

    func testOtherEntityOrSentKeepsSaveFeedback() throws {
        let doc = try DSLExamples.document("warikan", version: 1)
        let rt = AppRuntime(document: doc, services: BuildConfig.make(environment: .mock))
        let form = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("expenses")))
        form.markSaved(queuedOffline: false)
        rt.lastSavedForm = .init(appId: doc.appId, entity: "expenses", form: form)
        rt.revokeSaveFeedback(for: .sent(appId: doc.appId, entity: "expenses"))
        rt.revokeSaveFeedback(for: .appStopped(appId: doc.appId, entity: "members"))
        rt.revokeSaveFeedback(for: .appStopped(appId: "other", entity: "expenses"))
        XCTAssertEqual(form.lastSaveResult, .saved)
    }
}

/// records の書き込みが常に `error` で失敗する appconfig。
private actor RefusingClient: AppConfigClient {
    let error: APIError
    let inner = InMemoryAppConfigClient()
    init(error: APIError) { self.error = error }

    func fetchDocument(appId: String) async throws -> AppDocument { try await inner.fetchDocument(appId: appId) }
    func listVersions(appId: String) async throws -> [VersionSummary] { [] }
    func restore(appId: String, toVersion version: Int) async throws -> AppDocument { try await inner.restore(appId: appId, toVersion: version) }
    func list(appId: String, entity: String) async throws -> [Record] { [] }
    func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record { throw error }
    func create(appId: String, entity: String, data: [String: JSONValue], clientId: String) async throws -> Record { throw error }
    func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record { throw error }
    func delete(appId: String, entity: String, id: String) async throws { throw error }
}
