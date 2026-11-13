// 審査用ビルドではフィードバックごと外す(#319)
#if !APP_STORE_REVIEW
import XCTest
import MetamonServices
@testable import Metamon

/// フィードバックの下書き(`MacFeedbackDraft`・#1309 / #2325): 送れるか・送る中身・書きかけの判定。
@MainActor
final class MacFeedbackDraftTests: XCTestCase {
    private let device = FeedbackSubmission.Device(appVersion: "1.2.3", build: "45", os: "macOS 15.1", model: "Mac15,3", locale: "ja_JP")
    private let appRef = FeedbackSubmission.AppRef(appId: "app-1", dslVersion: 3)

    private func context(withApp: Bool = true) -> MacFeedbackContext {
        let app = MacFeedbackContext.AppSummary(ref: appRef, name: "家計簿", screens: ["ホーム"], entities: ["支出"], json: nil)
        return MacFeedbackContext(screenshot: nil, app: withApp ? app : nil, device: device)
    }

    // MARK: - 送れるか

    func testEmptyOrBlankCannotSend() {
        XCTAssertFalse(MacFeedbackDraft.canSend(message: "", agreed: true, sending: false, sent: false))
        XCTAssertFalse(MacFeedbackDraft.canSend(message: "  \n\t ", agreed: true, sending: false, sent: false), "空白・改行だけは空と同じ")
        XCTAssertTrue(MacFeedbackDraft.canSend(message: "落ちました", agreed: true, sending: false, sent: false))
    }

    func testLengthLimit() {
        let max = FeedbackConsent.maxMessageLength
        XCTAssertTrue(MacFeedbackDraft.canSend(message: String(repeating: "あ", count: max), agreed: true, sending: false, sent: false),
                      "上限ちょうどは送れる")
        XCTAssertFalse(MacFeedbackDraft.canSend(message: String(repeating: "あ", count: max + 1), agreed: true, sending: false, sent: false),
                       "上限を超えたら送れない")
        XCTAssertTrue(MacFeedbackDraft.canSend(message: "  " + String(repeating: "a", count: max) + "\n", agreed: true, sending: false, sent: false),
                      "前後の空白は数えない")
    }

    func testConsentIsRequired() {
        XCTAssertFalse(MacFeedbackDraft.canSend(message: "要望です", agreed: false, sending: false, sent: false), "同意のチェックが要る")
    }

    func testCannotSendTwice() {
        XCTAssertFalse(MacFeedbackDraft.canSend(message: "要望です", agreed: true, sending: true, sent: false), "送信中は押せない")
        XCTAssertFalse(MacFeedbackDraft.canSend(message: "要望です", agreed: true, sending: false, sent: true), "送った後は押せない")
    }

    // MARK: - 送る中身

    func testAttachmentsOffByDefaultSendOnlyMessage() {
        let s = MacFeedbackDraft.submission(kind: .bug, message: "  落ちました\n", context: context(), attachDevice: false, attachApp: false)
        XCTAssertEqual(s.kind, .bug)
        XCTAssertEqual(s.message, "落ちました", "前後の空白を落として送る")
        XCTAssertNil(s.device, "端末の情報はオンにした時だけ")
        XCTAssertNil(s.app)
        XCTAssertNil(s.screenshotJPEG)
        XCTAssertEqual(s.consentVersion, FeedbackConsent.version)
    }

    func testAttachDevice() {
        let s = MacFeedbackDraft.submission(kind: .idea, message: "要望", context: context(), attachDevice: true, attachApp: false)
        XCTAssertEqual(s.device, device)
        XCTAssertNil(s.app)
    }

    func testAttachAppSendsOnlyRef() {
        let s = MacFeedbackDraft.submission(kind: .other, message: "設計", context: context(), attachDevice: false, attachApp: true)
        XCTAssertEqual(s.app, appRef, "送るのは appId と版だけ")
        XCTAssertNil(s.device)
        let none = MacFeedbackDraft.submission(kind: .other, message: "設計", context: context(withApp: false), attachDevice: false, attachApp: true)
        XCTAssertNil(none.app, "サーバーに無いアプリは付けられない")
    }

    // MARK: - 書きかけ・送った後

    func testUnsentDraft() {
        XCTAssertFalse(MacFeedbackDraft.isUnsent(message: "", sent: false))
        XCTAssertFalse(MacFeedbackDraft.isUnsent(message: " \n", sent: false), "空白だけは書きかけにしない")
        XCTAssertTrue(MacFeedbackDraft.isUnsent(message: "書きかけ", sent: false))
        XCTAssertFalse(MacFeedbackDraft.isUnsent(message: "書きかけ", sent: true), "送った後は閉じても聞かない")
    }

    func testClosingDiscardsCapturedContext() {
        let center = MacFeedbackCenter.shared
        center.close()
        center.ensureContext()
        XCTAssertNotNil(center.context)
        center.close() // 送った後・閉じた時
        XCTAssertNil(center.context, "閉じたら撮ったものも書いた内容も残さない")
    }
}
#endif
