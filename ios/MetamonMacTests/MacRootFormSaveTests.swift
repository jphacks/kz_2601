import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// タブ / main の根に置いたフォームで保存した時(#1884・iOS #1719 と同じ `NavigationResolver.afterSave`)。
/// 根は閉じる先が無いので留まり、「保存しました」を出して読み上げる。新規なら入力を空に戻す。
@MainActor
final class MacRootFormSaveTests: XCTestCase {
    private var posted: [String] = []
    private var savedPost: ((String, MacVoiceOver.Priority) -> Void)!
    private let tabRoot = Route(screen: "form", stack: Route.tabStack("form"))

    override func setUp() async throws {
        savedPost = MacVoiceOver.post
        posted = []
        MacVoiceOver.post = { [weak self] text, _ in self?.posted.append(text) }
    }

    override func tearDown() async throws {
        MacVoiceOver.post = savedPost
    }

    private let json = """
    {"schemaVersion":1,"initialScreen":"form","app":{"name":"メモ","tabs":[{"screen":"form","title":"入力"},{"screen":"list","title":"一覧"}]},
     "entities":[{"id":"notes","name":"メモ","fields":[{"id":"memo","name":"メモ","type":"string"}]}],
     "screens":[{"id":"form","title":"入力","entity":"notes","components":[{"type":"textField","field":"memo"}]},
                {"id":"list","title":"一覧","components":[{"type":"list","entity":"notes"}]},
                {"id":"entry","title":"追加","entity":"notes","components":[{"type":"textField","field":"memo"}]}]}
    """

    private func runtime() throws -> MacAppRuntime {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        return MacAppRuntime(appId: "test-root-save-\(UUID().uuidString.lowercased())", dsl: dsl,
                             ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    private func saveAction() throws -> Action {
        try JSONDecoder().decode(Action.self, from: Data(#"{"type":"saveRecord","entity":"notes"}"#.utf8))
    }

    private func form(_ rt: MacAppRuntime, memo: String) throws -> FormDraft {
        let form = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("notes")))
        form.values["memo"] = .string(memo)
        return form
    }

    func testNewRecordAtTabRootStaysResetsAndToasts() async throws {
        let rt = try runtime()
        let form = try form(rt, memo: "買い物")
        await rt.run(try saveAction(), ActionContext(route: tabRoot, form: form))

        XCTAssertEqual(rt.records(for: "notes").count, 1, "記録は保存する")
        XCTAssertEqual(rt.toast, MacAnnouncement.saved.text, "根では閉じる先が無いので「保存しました」を出す")
        XCTAssertEqual(rt.announcedToast, MacAnnouncement.saved.text, "toast の側では読み直さない")
        XCTAssertEqual(posted, [MacAnnouncement.saved.text], "VoiceOver で 1 回だけ読む")
        XCTAssertEqual(form.values["memo"] ?? .null, .null, "新規は続けて入れられるよう空に戻す")
        XCTAssertFalse(form.hasUnsavedChanges)
        XCTAssertEqual(rt.selectedTab, "form", "ほかのタブへは切り替えない")
        XCTAssertEqual(rt.paths[tabRoot.stack] ?? [], [])
    }

    /// 3 秒以内に 2 回保存: 文言は同じでも toastSerial が変わり、AppCanvas の表示と読み上げの印が出し直しになる
    func testSecondSaveWithSameToastRestartsToast() async throws {
        let rt = try runtime()
        let form = try form(rt, memo: "1")
        await rt.run(try saveAction(), ActionContext(route: tabRoot, form: form))
        let first = rt.toastSerial
        rt.announcedToast = nil // AppCanvas が表示して印を戻した

        form.values["memo"] = .string("2")
        await rt.run(try saveAction(), ActionContext(route: tabRoot, form: form))

        XCTAssertEqual(rt.records(for: "notes").count, 2)
        XCTAssertEqual(rt.toast, MacAnnouncement.saved.text)
        XCTAssertNotEqual(rt.toastSerial, first, "同じ文言でも .task(id:) を出し直す")
        XCTAssertEqual(rt.announcedToast, MacAnnouncement.saved.text, "2 回目も toast の側では読み直さない")
        XCTAssertEqual(posted, [MacAnnouncement.saved.text, MacAnnouncement.saved.text])
    }

    /// 保存の間に入れ直した値は、保存の後で空に戻さない
    func testTypingDuringSaveIsKept() async throws {
        let rt = try runtime()
        let form = try form(rt, memo: "買い物")
        // 書き込みの await が終わった直後(保存しましたを読む時)に、次の入力が入っていた
        MacVoiceOver.post = { [weak self] text, _ in
            self?.posted.append(text)
            if text == MacAnnouncement.saved.text { form.values["memo"] = .string("次のメモ") }
        }
        await rt.run(try saveAction(), ActionContext(route: tabRoot, form: form))

        XCTAssertEqual(rt.records(for: "notes").count, 1)
        XCTAssertEqual(rt.records(for: "notes").first?.data["memo"], .string("買い物"), "保存したのは押した時の値")
        XCTAssertEqual(form.values["memo"], .string("次のメモ"), "入れ直した値を消さない")
        XCTAssertEqual(rt.toast, MacAnnouncement.saved.text)
    }

    /// 保存の間(書き込みの await の間)に入れ直した値は保存していない: 保存の後も未保存のまま、離れる前に破棄の確認が出る
    func testTypingDuringSaveStaysUnsaved() async throws {
        let rt = try runtime()
        let form = try form(rt, memo: "買い物")
        rt.openForms.track(form, at: tabRoot) // `ScreenView` と同じく控えに入れる
        try await typeDuringSave(rt, form, "次のメモ")

        XCTAssertEqual(rt.records(for: "notes").first?.data["memo"], .string("買い物"))
        XCTAssertEqual(form.values["memo"], .string("次のメモ"), "入れ直した値を消さない")
        XCTAssertTrue(form.hasUnsavedChanges, "送ったのは「買い物」。入れ直した値は保存していない")
        var left = false
        rt.confirmDiscard { left = true } // アプリの切り替え・⌘W と同じ
        XCTAssertFalse(left, "確認の前に離れない")
        XCTAssertNotNil(rt.discardRequest, "破棄の確認を出す")
    }

    /// 既存の記録を直している間に入れ直した: もう一度保存すると書く(変更なしで飛ばさない)
    func testTypingDuringEditSaveStaysUnsaved() async throws {
        let rt = try runtime()
        await rt.run(try saveAction(), ActionContext(route: tabRoot, form: try form(rt, memo: "朝")))
        let record = try XCTUnwrap(rt.records(for: "notes").first)
        let edit = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("notes")))
        edit.load(record)
        edit.values["memo"] = .string("昼")
        try await typeDuringSave(rt, edit, "夜", recordId: record.id)

        XCTAssertTrue(edit.hasUnsavedChanges)
        XCTAssertEqual(rt.records(for: "notes").first?.data["memo"], .string("昼"))
        await rt.run(try saveAction(), ActionContext(route: tabRoot, form: edit, recordId: record.id))
        XCTAssertEqual(rt.records(for: "notes").first?.data["memo"], .string("夜"), "2 回目の保存で書く")
        XCTAssertFalse(edit.hasUnsavedChanges)
    }

    /// 書き込み(recordSync の actor)を待っている間に `text` を入れる
    private func typeDuringSave(_ rt: MacAppRuntime, _ form: FormDraft, _ text: String, recordId: String? = nil) async throws {
        let action = try saveAction()
        let saving = Task { await rt.run(action, ActionContext(route: tabRoot, form: form, recordId: recordId)) }
        for _ in 0..<1000 where !rt.book.isSaving(form) { await Task.yield() }
        XCTAssertTrue(rt.book.isSaving(form), "保存の途中に入れる")
        form.values["memo"] = .string(text)
        await saving.value
    }

    func testEditingExistingRecordKeepsValues() async throws {
        let rt = try runtime()
        await rt.run(try saveAction(), ActionContext(route: tabRoot, form: try form(rt, memo: "朝")))
        let record = try XCTUnwrap(rt.records(for: "notes").first)
        rt.toast = nil
        posted = []

        let edit = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("notes")))
        edit.load(record)
        edit.values["memo"] = .string("夜")
        await rt.run(try saveAction(), ActionContext(route: tabRoot, form: edit, recordId: record.id))

        XCTAssertEqual(rt.toast, MacAnnouncement.saved.text)
        XCTAssertEqual(edit.values["memo"], .string("夜"), "既存の記録を直した時は入力を残す")
        XCTAssertFalse(edit.hasUnsavedChanges, "保存の間に入れていなければ保存済み")
        rt.openForms.track(edit, at: tabRoot)
        var left = false
        rt.confirmDiscard { left = true }
        XCTAssertTrue(left, "保存済みなら確認なしで離れる")
        XCTAssertNil(rt.discardRequest)
        XCTAssertEqual(posted, [MacAnnouncement.saved.text])

        // 何も変えずに保存: 書かず、何も出さない(M10)
        rt.toast = nil
        posted = []
        await rt.run(try saveAction(), ActionContext(route: tabRoot, form: edit, recordId: record.id))
        XCTAssertNil(rt.toast)
        XCTAssertEqual(posted, [])
    }

    func testSheetFormStillCloses() async throws {
        let rt = try runtime()
        let nav = try JSONDecoder().decode(Action.self, from: Data(#"{"type":"navigate","screen":"entry","presentation":"sheet"}"#.utf8))
        await rt.run(nav, ActionContext(route: tabRoot))
        let sheet = try XCTUnwrap(rt.sheet)
        await rt.run(try saveAction(), ActionContext(route: sheet, form: try form(rt, memo: "a")))
        XCTAssertNil(rt.sheet, "sheet からの保存は今までどおり閉じる")
        XCTAssertNil(rt.toast, "閉じる時は toast を出さない")
        XCTAssertEqual(posted, [MacAnnouncement.saved.text])
    }

    func testPushedFormStillCloses() async throws {
        let rt = try runtime()
        let nav = try JSONDecoder().decode(Action.self, from: Data(#"{"type":"navigate","screen":"entry","presentation":"push"}"#.utf8))
        await rt.run(nav, ActionContext(route: tabRoot))
        let pushed = try XCTUnwrap(rt.paths[tabRoot.stack]?.last)
        await rt.run(try saveAction(), ActionContext(route: pushed, form: try form(rt, memo: "a")))
        XCTAssertEqual(rt.paths[tabRoot.stack] ?? [], [], "積まれた画面からの保存は今までどおり閉じる")
        XCTAssertNil(rt.toast)
    }
}
