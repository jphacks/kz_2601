import XCTest
@testable import MetamonServices

/// 有料機能の確認カードの出し分け(#77)。
final class PaidFeatureCheckTests: XCTestCase {
    private let shared = PaidFeature(id: "sharedData", title: "みんなで共有するデータ", reason: "サーバーの費用がかかります。")

    private func readyTurn(requiresPro: Bool? = true, free: String? = "掲示板\n(無料版)") -> ChatTurn {
        ChatTurn(kind: .ready, text: "作ります", turn: 1, transcript: "みんなで書き込む掲示板",
                 requiresPro: requiresPro, paidFeatures: requiresPro == true ? [shared] : nil, freeTranscript: free)
    }

    func testShowsCardWhenPaidAndNotProAndEnforced() throws {
        let c = try XCTUnwrap(PaidFeatureCheck.confirmation(for: readyTurn(), text: "みんなで書き込む掲示板", isPro: false, enforced: true))
        XCTAssertEqual(c.paidText, "みんなで書き込む掲示板")
        XCTAssertEqual(c.freeText, "掲示板\n(無料版)")
        XCTAssertEqual(c.featureNames, "みんなで共有するデータ")
    }

    func testProMemberSeesNothing() {
        XCTAssertNil(PaidFeatureCheck.confirmation(for: readyTurn(), text: "みんなで書き込む掲示板", isPro: true, enforced: true))
    }

    /// Debug の既定(回数制限の無効化中)= 出さない。`METAMON_IAP_ENFORCE=1` で enforced になる
    func testNotEnforcedSeesNothing() {
        XCTAssertNil(PaidFeatureCheck.confirmation(for: readyTurn(), text: "みんなで書き込む掲示板", isPro: false, enforced: false))
    }

    func testFreeOrOldBackendSeesNothing() {
        XCTAssertNil(PaidFeatureCheck.confirmation(for: readyTurn(requiresPro: false), text: "みんなで書き込む掲示板", isPro: false, enforced: true))
        XCTAssertNil(PaidFeatureCheck.confirmation(for: readyTurn(requiresPro: nil), text: "みんなで書き込む掲示板", isPro: false, enforced: true))
    }

    /// 予備の依頼文(ready の transcript 以外)で作る時は判定材料が無いので出さない
    func testOnlyForReadyTranscript() {
        XCTAssertNil(PaidFeatureCheck.confirmation(for: readyTurn(), text: "別の文", isPro: false, enforced: true))
        XCTAssertNil(PaidFeatureCheck.confirmation(for: nil, text: "みんなで書き込む掲示板", isPro: false, enforced: true))
    }

    func testMissingFreeTextFallsBack() throws {
        let c = try XCTUnwrap(PaidFeatureCheck.confirmation(for: readyTurn(free: nil), text: "みんなで書き込む掲示板", isPro: false, enforced: true))
        XCTAssertEqual(c.freeText, "みんなで書き込む掲示板\n" + PaidFeatureCheck.fallbackFreeSuffix)
    }

    func testOptionPerIdea() throws {
        let paid = ChatOption(id: "opt1", title: "みんなで", instruction: "家族で共有して", requiresPro: true, paidFeatures: [shared], freeInstruction: "自分だけで")
        let free = ChatOption(id: "opt2", title: "グラフ", instruction: "グラフを追加して")
        XCTAssertEqual(PaidFeatureCheck.confirmation(for: paid, isPro: false, enforced: true)?.freeText, "自分だけで")
        XCTAssertNil(PaidFeatureCheck.confirmation(for: free, isPro: false, enforced: true))
    }

    /// backend の JSON(新フィールドあり / 無し)を読める
    func testDecodesNewAndOldResponses() throws {
        let new = #"""
        {"kind":"proposal","text":"案です","turn":1,"locale":"ja","source":"claude","mode":"edit",
         "options":[{"id":"opt1","title":"みんなで","detail":"","instruction":"共有して","requiresPro":true,
           "paidFeatures":[{"id":"sharedData","title":"みんなで共有するデータ","reason":"r"}],"freeInstruction":"自分だけ"}]}
        """#
        let t = try JSONDecoder().decode(ChatTurn.self, from: Data(new.utf8))
        XCTAssertEqual(t.options?.first?.requiresPro, true)
        XCTAssertEqual(t.options?.first?.paidFeatures?.first?.id, "sharedData")
        XCTAssertEqual(t.options?.first?.freeInstruction, "自分だけ")

        let ready = #"{"kind":"ready","text":"作ります","turn":2,"transcript":"掲示板","requiresPro":true,"paidFeatures":[{"id":"sharedData","title":"t","reason":"r"}],"freeTranscript":"掲示板(無料)"}"#
        let r = try JSONDecoder().decode(ChatTurn.self, from: Data(ready.utf8))
        XCTAssertEqual(r.requiresPro, true)
        XCTAssertEqual(r.freeTranscript, "掲示板(無料)")

        let old = #"{"kind":"ready","text":"作ります","turn":2,"transcript":"家計簿"}"#
        let o = try JSONDecoder().decode(ChatTurn.self, from: Data(old.utf8))
        XCTAssertNil(o.requiresPro)
        XCTAssertNil(PaidFeatureCheck.confirmation(for: o, text: "家計簿", isPro: false, enforced: true))
    }
}
