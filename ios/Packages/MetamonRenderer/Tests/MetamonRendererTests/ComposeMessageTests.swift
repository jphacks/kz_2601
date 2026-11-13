import XCTest
@testable import MetamonRenderer

/// `sms:` / `mailto:` の組み立て(#2223)。日本語・改行・複数の宛先のエスケープが肝。
/// `MFMailComposeViewController` は画面なので試せない。試すのは純関数と、出す口の切り替えだけ。
final class ComposeMessageTests: XCTestCase {

    // MARK: - 日本語

    func testJapaneseSubjectAndBodyArePercentEncoded() {
        let draft = ComposeDraft(recipients: ["a@example.com"], subject: "今日の予定", body: "こんにちは")
        let url = ComposeMessageURL.mailto(draft)
        XCTAssertEqual(url?.absoluteString,
                       "mailto:a@example.com?subject=%E4%BB%8A%E6%97%A5%E3%81%AE%E4%BA%88%E5%AE%9A&body=%E3%81%93%E3%82%93%E3%81%AB%E3%81%A1%E3%81%AF")
    }

    /// 組み立てた URL を戻すと元の文字列に戻る(UTF-8 のまま壊れていない)
    func testJapaneseRoundTrips() throws {
        let body = "お疲れさま"
        let url = try XCTUnwrap(ComposeMessageURL.mailto(ComposeDraft(recipients: ["a@example.com"], body: body)))
        let components = try XCTUnwrap(URLComponents(url: url, resolvingAgainstBaseURL: false))
        XCTAssertEqual(components.queryItems?.first { $0.name == "body" }?.value, body)
    }

    func testJapaneseInSmsBody() {
        let url = ComposeMessageURL.sms(ComposeDraft(recipients: ["09012345678"], body: "こんにちは"))
        XCTAssertEqual(url?.absoluteString, "sms:09012345678&body=%E3%81%93%E3%82%93%E3%81%AB%E3%81%A1%E3%81%AF")
    }

    // MARK: - 改行

    func testNewlineBecomesPercent0A() {
        let url = ComposeMessageURL.mailto(ComposeDraft(recipients: ["a@example.com"], body: "1行目\n2行目"))
        XCTAssertEqual(url?.absoluteString,
                       "mailto:a@example.com?body=1%E8%A1%8C%E7%9B%AE%0A2%E8%A1%8C%E7%9B%AE")
    }

    func testCRLFBecomesPercent0D0A() {
        let url = ComposeMessageURL.mailto(ComposeDraft(recipients: ["a@example.com"], body: "A\r\nB"))
        XCTAssertEqual(url?.absoluteString, "mailto:a@example.com?body=A%0D%0AB")
    }

    func testNewlineInSmsBody() {
        let url = ComposeMessageURL.sms(ComposeDraft(recipients: ["09012345678"], body: "打ち合わせ\nよろしく"))
        XCTAssertEqual(url?.absoluteString,
                       "sms:09012345678&body=%E6%89%93%E3%81%A1%E5%90%88%E3%82%8F%E3%81%9B%0A%E3%82%88%E3%82%8D%E3%81%97%E3%81%8F")
    }

    // MARK: - 複数の宛先

    func testMultipleMailRecipientsJoinedWithComma() {
        let url = ComposeMessageURL.mailto(ComposeDraft(recipients: ["a@x.com", "b@y.com", "c@z.com"]))
        XCTAssertEqual(url?.absoluteString, "mailto:a@x.com,b@y.com,c@z.com")
    }

    func testMultipleSmsRecipientsJoinedWithComma() {
        let url = ComposeMessageURL.sms(ComposeDraft(recipients: ["09012345678", "+819087654321"], body: "やあ"))
        XCTAssertEqual(url?.absoluteString, "sms:09012345678,+819087654321&body=%E3%82%84%E3%81%82")
    }

    /// 区切りの `,` は宛先の間だけ。宛先の中の `,` や `+` は `%2C`・`%2B` に逃がす(区切りと混ざらない)
    func testCommaAndPlusInsideAddressAreEscaped() {
        let url = ComposeMessageURL.mailto(ComposeDraft(recipients: ["a+tag@x.com", "b,c@y.com"]))
        XCTAssertEqual(url?.absoluteString, "mailto:a%2Btag@x.com,b%2Cc@y.com")
    }

    func testBlankRecipientsAreDropped() {
        let url = ComposeMessageURL.mailto(ComposeDraft(recipients: [" a@x.com ", "", "   ", "b@y.com"]))
        XCTAssertEqual(url?.absoluteString, "mailto:a@x.com,b@y.com")
    }

    func testNoRecipientStillOpensEmptyDraft() {
        XCTAssertEqual(ComposeMessageURL.mailto(ComposeDraft())?.absoluteString, "mailto:")
        XCTAssertEqual(ComposeMessageURL.sms(ComposeDraft())?.absoluteString, "sms:")
    }

    // MARK: - クエリが壊れないこと

    /// `.urlQueryAllowed` だとここが素通りして `&`・`=` でクエリが割れる
    func testQuerySeparatorsInBodyAreEscaped() {
        let url = ComposeMessageURL.mailto(ComposeDraft(recipients: ["a@x.com"], body: "a&b=c?d+e/f#g"))
        XCTAssertEqual(url?.absoluteString, "mailto:a@x.com?body=a%26b%3Dc%3Fd%2Be%2Ff%23g")
    }

    func testBodyWithAmpersandStaysOneQueryItem() throws {
        let draft = ComposeDraft(recipients: ["a@x.com"], subject: "A&B", body: "x=1&y=2")
        let url = try XCTUnwrap(ComposeMessageURL.mailto(draft))
        let items = try XCTUnwrap(URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems)
        XCTAssertEqual(items.count, 2)
        XCTAssertEqual(items.first { $0.name == "subject" }?.value, "A&B")
        XCTAssertEqual(items.first { $0.name == "body" }?.value, "x=1&y=2")
    }

    /// 空白は `%20`。`mailto:` では `+` は空白ではなくただの `+` なので `+` にしてはいけない
    func testSpaceBecomesPercent20() {
        let url = ComposeMessageURL.mailto(ComposeDraft(recipients: ["a@x.com"], subject: "hello world"))
        XCTAssertEqual(url?.absoluteString, "mailto:a@x.com?subject=hello%20world")
    }

    func testUnreservedCharactersAreKept() {
        let url = ComposeMessageURL.mailto(ComposeDraft(recipients: ["a@x.com"], body: "a-b_c.d~e9Z"))
        XCTAssertEqual(url?.absoluteString, "mailto:a@x.com?body=a-b_c.d~e9Z")
    }

    func testEmptySubjectAndBodyAreSkipped() {
        let url = ComposeMessageURL.mailto(ComposeDraft(recipients: ["a@x.com"], subject: "", body: ""))
        XCTAssertEqual(url?.absoluteString, "mailto:a@x.com")
    }

    /// `sms:` には件名が無いので捨てる
    func testSmsDropsSubject() {
        let url = ComposeMessageURL.sms(ComposeDraft(recipients: ["09012345678"], subject: "件名", body: "本文"))
        XCTAssertEqual(url?.absoluteString, "sms:09012345678&body=%E6%9C%AC%E6%96%87")
    }

    /// 電話番号の `#`・`*` は断片・あいまいなので逃がす。国番号の `+` だけ残す
    func testPhoneSpecialCharactersAreEscaped() {
        let url = ComposeMessageURL.sms(ComposeDraft(recipients: ["+81-90-1234-5678", "*123#"]))
        XCTAssertEqual(url?.absoluteString, "sms:+81-90-1234-5678,%2A123%23")
    }

    func testUrlForChannelPicksScheme() {
        let draft = ComposeDraft(recipients: ["a@x.com"], body: "hi")
        XCTAssertEqual(ComposeMessageURL.url(for: .mail, draft: draft)?.scheme, "mailto")
        XCTAssertEqual(ComposeMessageURL.url(for: .message, draft: draft)?.scheme, "sms")
    }

    // MARK: - 出す口の切り替え

    @MainActor
    func testUsesComposeViewControllerWhenAvailable() async {
        let presenter = FakeComposePresenter(composable: [.mail], presentResult: .sent)
        let draft = ComposeDraft(recipients: ["a@x.com"], subject: "件名", body: "本文")
        let result = await ComposeMessage.open(.mail, draft: draft, using: presenter)
        XCTAssertEqual(result, .sent)
        XCTAssertEqual(presenter.presented.count, 1)
        XCTAssertEqual(presenter.presented.first?.draft, draft)
        XCTAssertTrue(presenter.opened.isEmpty)
    }

    @MainActor
    func testFallsBackToUrlWhenComposeIsUnavailable() async {
        let presenter = FakeComposePresenter(composable: [])
        let result = await ComposeMessage.open(.mail, draft: ComposeDraft(recipients: ["a@x.com"], body: "やあ"),
                                               using: presenter)
        XCTAssertEqual(result, .opened)
        XCTAssertTrue(presenter.presented.isEmpty)
        XCTAssertEqual(presenter.opened.map(\.absoluteString), ["mailto:a@x.com?body=%E3%82%84%E3%81%82"])
    }

    @MainActor
    func testFallsBackToSmsUrlForMessage() async {
        let presenter = FakeComposePresenter(composable: [.mail])
        let result = await ComposeMessage.open(.message, draft: ComposeDraft(recipients: ["09012345678"], body: "hi"),
                                               using: presenter)
        XCTAssertEqual(result, .opened)
        XCTAssertEqual(presenter.opened.map(\.absoluteString), ["sms:09012345678&body=hi"])
    }

    @MainActor
    func testUnavailableWhenUrlCannotBeOpened() async {
        let presenter = FakeComposePresenter(composable: [], canOpen: false)
        let result = await ComposeMessage.open(.message, draft: ComposeDraft(recipients: ["09012345678"]),
                                               using: presenter)
        XCTAssertEqual(result, .unavailable)
    }

    @MainActor
    func testCancelledIsReturnedAsIs() async {
        let presenter = FakeComposePresenter(presentResult: .cancelled)
        let result = await ComposeMessage.open(.message, draft: ComposeDraft(recipients: ["09012345678"]),
                                               using: presenter)
        XCTAssertEqual(result, .cancelled)
    }

    /// 宛先・件名・本文はログに出さない
    func testDescriptionHidesContents() {
        let draft = ComposeDraft(recipients: ["a@x.com"], subject: "秘密", body: "見せない")
        XCTAssertEqual(draft.description, "ComposeDraft(recipients: 1, subject: true, body: true)")
        XCTAssertFalse(draft.debugDescription.contains("秘密"))
        XCTAssertFalse(draft.debugDescription.contains("a@x.com"))
    }
}
