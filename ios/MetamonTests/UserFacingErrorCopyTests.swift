import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// H9・M15・L1: 失敗の文言に内部の値(dB・サンプル数・英語の OS の文・サーバーの文)を出さず、
/// 「何が起きたか + 次の一手」の決まった文にする。詳しい中身はログ(`ErrorLog`)にだけ残す。
@MainActor
final class UserFacingErrorCopyTests: XCTestCase {
    private var saved: AppLanguage = .system

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    /// 内部の値が混ざっていないか(数字・dB・サンプル・オーディオセッション・英語の文)
    private func assertUserFacing(_ text: String, _ hidden: [String] = [], file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertFalse(text.isEmpty, file: file, line: line)
        for word in ["dB", "サンプル", "オーディオセッション", "入力ルート", "version", "(v"] + hidden {
            XCTAssertFalse(text.contains(word), "「\(word)」が見えている: \(text)", file: file, line: line)
        }
        XCTAssertNil(text.range(of: "[0-9]", options: .regularExpression), "数字が見えている: \(text)", file: file, line: line)
    }

    // MARK: - 録音・文字起こし(表のテスト)

    func testTranscriberFailuresHideDiagnostics() {
        let table: [(TranscriberFailure, title: String, body: String)] = [
            (.notReady("loading 42%"), "音声モデルを準備中です", "少し待ってから、もう一度お試しください"),
            (.microphoneDenied, "マイクを使えません", "設定アプリ > Tailor It でマイクを許可してください"),
            (.audioSession("AVAudioSessionErrorCodeCannotStartRecording 561017449"), "録音できませんでした", "ほかのアプリの通話や録音を終えてから、もう一度お試しください"),
            (.noInputAvailable, "録音できませんでした", "ほかのアプリの通話や録音を終えてから、もう一度お試しください"),
            (.noAudioCaptured(seconds: 1.2), "録音できませんでした", "ほかのアプリの通話や録音を終えてから、もう一度お試しください"),
            (.tooShort(seconds: 0.3), "うまく聞き取れませんでした", "タップしてから話し、話し終えたらもう一度タップしてください"),
            (.silent(seconds: 2.0, peakDecibels: -48), "うまく聞き取れませんでした", "マイクに近づいて、もう一度話してください"),
        ]
        for (failure, title, body) in table {
            let card = ErrorPresentation.voiceFailure(failure)
            XCTAssertEqual(card.title, title, "\(failure)")
            XCTAssertEqual(card.body, body, "\(failure)")
            XCTAssertNil(card.primary, "録音の失敗は話し直してもらう(同じ発話の再送は無い)")
            XCTAssertEqual(card.secondary, .dismiss)
            assertUserFacing(card.announcement, ["AVAudio", "loading"])
        }
    }

    func testEmptyTranscriptIsStateCard() {
        var diag = TranscriberDiagnostics()
        diag.recordedSeconds = 2.0
        diag.sampleCount = 32000
        diag.peakAmplitude = 0.004
        let card = ErrorPresentation.voiceFailure(nil, diagnostics: diag)
        XCTAssertEqual(card.title, "うまく聞き取れませんでした")
        XCTAssertEqual(card.body, "もう一度、はっきり話してください")
        assertUserFacing(card.announcement)
    }

    func testUnknownVoiceErrorHidesOSMessage() {
        let os = NSError(domain: "com.apple.coreaudio.avfaudio", code: 561_145_187, userInfo: [NSLocalizedDescriptionKey: "The operation couldn’t be completed."])
        let card = ErrorPresentation.voiceFailure(os)
        XCTAssertEqual(card.title, "文字起こしできませんでした")
        assertUserFacing(card.announcement, ["operation", "couldn"])
    }

    // MARK: - 生成の失敗(サーバーの文を出さない)

    func testGenerateFailureHidesServerMessage() throws {
        let table: [(FailureKind, title: String, body: String)] = [
            (.rejected("brief.entities must not be empty"), "うまく伝わりませんでした", "言い方を変えて、もう一度お話しください"),
            (.other("The data couldn’t be read because it isn’t in the correct format."), "うまく作れませんでした", "少し待ってから、もう一度お試しください"),
        ]
        for (failure, title, body) in table {
            let card = try XCTUnwrap(ErrorPresentation.from(failure, transcript: "家計簿を作って"))
            XCTAssertEqual(card.title, title)
            XCTAssertEqual(card.body, body)
            assertUserFacing(card.announcement, ["brief", "data", "format"])
        }
        let chat = try XCTUnwrap(ErrorPresentation.from(.other("decode error at turn 3"), transcript: "x", origin: .chat))
        XCTAssertEqual(chat.title, "質問をうまく用意できませんでした")
        assertUserFacing(chat.body, ["decode"])
    }

    func testFailureSentenceUsesNextStep() {
        let offline = ErrorCopy.failure(L10n.tr("インストールできませんでした"), URLError(.notConnectedToInternet), context: "test")
        XCTAssertEqual(offline, "インストールできませんでした。接続を確認して、もう一度お試しください")
        let other = ErrorCopy.failure(L10n.tr("名前を変えられませんでした"),
                                      NSError(domain: "x", code: 1, userInfo: [NSLocalizedDescriptionKey: "Internal Server Error"]), context: "test")
        XCTAssertEqual(other, "名前を変えられませんでした。少し待ってから、もう一度お試しください")
    }

    func testActionErrorHidesServerAndOSMessage() throws {
        let rt = AppRuntime(document: try DSLExamples.document("warikan", version: 1), services: BuildConfig.make(environment: .mock))
        let os = NSError(domain: NSURLErrorDomain, code: -1011, userInfo: [NSLocalizedDescriptionKey: "bad server response"])
        assertUserFacing(rt.message(for: os, entity: nil), ["bad server"])
        let server = APIError.server(status: 500, code: "internal", message: "firestore: deadline exceeded")
        assertUserFacing(rt.message(for: server, entity: nil), ["firestore", "deadline"])
        let invalid = APIError.server(status: 422, code: "invalid_record", message: "field \"payer\" is required", details: ["field": .string("payer")])
        let text = rt.message(for: invalid, entity: "expenses")
        XCTAssertEqual(text, "「払った人」の入力を確認してください")
    }

    func testStartupUnreachableIsStateCardWithoutDetail() {
        let card = ErrorPresentation.notice(icon: "wifi.exclamationmark", title: ErrorCopy.unreachableTitle, body: ErrorCopy.unreachableBody)
        XCTAssertNil(card.primary)
        XCTAssertEqual(card.secondary, .dismiss)
        assertUserFacing(card.announcement)
        assertUserFacing(ErrorCopy.offlineRestored)
    }

    // MARK: - 言葉のぶれ(M15)・確認の主ボタン(L1)

    func testWordingUsesMakeNotGenerate() {
        XCTAssertFalse(ErrorCopy.failedTitle.contains("生成"))
        XCTAssertFalse(ErrorCopy.cancelledToast.contains("生成"))
        XCTAssertEqual(Greeting.homeAction, "最初の画面に戻る")
    }

    func testConfirmButtonSaysWhatHappens() {
        let table: [(ActionType, String, Bool)] = [
            (.deleteRecord, "削除", true),
            (.resetForm, "入力を消す", true),
            (.saveRecord, "保存", false),
            (.export, "書き出す", false),
            (.openUrl, "開く", false),
            (.scheduleNotification, "続ける", false),
        ]
        for (type, label, destructive) in table {
            let b = AppRuntime.Confirm.button(for: type)
            XCTAssertEqual(b.label, label, "\(type)")
            XCTAssertEqual(b.destructive, destructive, "\(type)")
            XCTAssertNotEqual(b.label, "実行")
        }
    }

    // MARK: - en

    func testEnglishCopyHasNoInternalValues() {
        LanguagePreference.shared.setting = .en
        let card = ErrorPresentation.voiceFailure(TranscriberFailure.silent(seconds: 2.0, peakDecibels: -48))
        XCTAssertEqual(card.title, "Couldn't catch that")
        XCTAssertEqual(card.body, "Move closer to the microphone and speak again")
        XCTAssertEqual(ErrorCopy.failure(L10n.tr("インストールできませんでした"), URLError(.notConnectedToInternet), context: "test"),
                       "Couldn't install. Check your connection and try again")
        XCTAssertEqual(AppRuntime.Confirm.button(for: .deleteRecord).label, "Delete")
        assertUserFacing(card.announcement)
    }
}
