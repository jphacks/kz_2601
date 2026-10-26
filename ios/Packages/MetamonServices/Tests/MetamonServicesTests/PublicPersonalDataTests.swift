import XCTest
import MetamonDSL
@testable import MetamonServices

/// #253: generate の `meta.publicPersonalData` と chat の `confirmPublic`(docs/backend/openapi-ai.yaml)、確認ダイアログの指示文。
final class PublicPersonalDataTests: XCTestCase {
    private let dsl = #"{"schemaVersion":1,"app":{"name":"予約"},"initialScreen":"home","entities":[],"screens":[{"id":"home","title":"t","components":[]}]}"#

    func testParsesPublicPersonalDataFromMeta() throws {
        let json = #"{"appId":"a","version":3,"mode":"full","dsl":\#(dsl),"summary":"s","meta":{"model":"m","attempts":1,"publicPersonalData":[{"entity":"reservations","entityName":"予約","field":"phone","fieldName":"電話番号","kind":"phone"},{"entity":"reservations","entityName":"予約","field":"guestName","fieldName":"お名前","kind":"name"}]}}"#
        let r = try HTTPAIClient.parse(Data(json.utf8), appId: "a")
        XCTAssertEqual(r.publicPersonalData.map(\.field), ["phone", "guestName"])
        XCTAssertEqual(r.publicPersonalData.first?.fieldName, "電話番号")
        XCTAssertEqual(r.publicPersonalData.first?.kind, "phone")
    }

    func testOldServerAndBrokenMetaYieldNoFields() throws {
        let old = #"{"appId":"a","version":3,"dsl":\#(dsl),"summary":"s","meta":{"model":"m"}}"#
        XCTAssertEqual(try HTTPAIClient.parse(Data(old.utf8), appId: "a").publicPersonalData, [])
        let noMeta = #"{"appId":"a","version":3,"dsl":\#(dsl)}"#
        XCTAssertEqual(try HTTPAIClient.parse(Data(noMeta.utf8), appId: "a").publicPersonalData, [])
        // 形が壊れていても生成そのものは捨てない
        let broken = #"{"appId":"a","version":3,"dsl":\#(dsl),"meta":{"publicPersonalData":"oops"}}"#
        let r = try HTTPAIClient.parse(Data(broken.utf8), appId: "a")
        XCTAssertEqual(r.document.version, 3)
        XCTAssertEqual(r.publicPersonalData, [])
    }

    func testDecodesConfirmPublicQuestion() throws {
        let json = #"{"kind":"question","text":"名前・電話番号のような個人の情報が、アプリを開いた人なら誰でも見られる記録に入ります。公開してよいですか?","suggestions":["公開してよい","個人の情報は入れない","自分だけの記録にする"],"turn":1,"confirmPublic":["name","phone"]}"#
        let t = try JSONDecoder().decode(ChatTurn.self, from: Data(json.utf8))
        XCTAssertEqual(t.confirmPublic, ["name", "phone"])
        XCTAssertTrue(t.isPublicConfirm)

        let plain = #"{"kind":"question","text":"写真も残しますか?","turn":1}"#
        XCTAssertFalse(try JSONDecoder().decode(ChatTurn.self, from: Data(plain.utf8)).isPublicConfirm)
    }

    func testPromptTextAndInstructions() throws {
        let fields = [
            PublicPersonalField(entity: "reservations", entityName: "予約", field: "guestName", fieldName: "お名前", kind: "name"),
            PublicPersonalField(entity: "reservations", entityName: "予約", field: "phone", fieldName: "電話番号", kind: "phone"),
            PublicPersonalField(entity: "members", entityName: "会員", field: "address", fieldName: "住所", kind: "address"),
        ]
        let p = try XCTUnwrap(PublicDataPrompt(fields: fields, version: 2))
        XCTAssertEqual(p.fieldNames(), "お名前・電話番号・住所")
        XCTAssertEqual(p.removeInstruction(), "予約の「お名前」と「電話番号」の欄と、会員の「住所」の欄を消して")
        XCTAssertEqual(p.privateInstruction(), "予約と会員を自分だけの記録にして")
        XCTAssertEqual(p.fieldNames(english: true), "お名前, 電話番号, 住所")
        XCTAssertEqual(p.removeInstruction(english: true), #"Remove the "お名前" and "電話番号" fields from 予約, and the "住所" field from 会員"#)
        XCTAssertEqual(p.privateInstruction(english: true), "Keep 予約 and 会員 to only me (make it private)")

        let one = try XCTUnwrap(PublicDataPrompt(fields: [fields[1]], version: 1))
        XCTAssertEqual(one.removeInstruction(), "予約の「電話番号」の欄を消して")
        XCTAssertEqual(one.privateInstruction(), "予約を自分だけの記録にして")
    }

    func testNoPromptWithoutFields() {
        XCTAssertNil(PublicDataPrompt(fields: [], version: 1), "旧サーバー(フィールド無し)では出さない")
    }

    func testMockReturnsPublicReservation() async throws {
        let ai = MockAIClient(delay: .zero)
        let r = try await ai.morph(transcript: "美容室の予約アプリを作って", appId: "a", currentVersion: 0)
        XCTAssertEqual(r.publicPersonalData.map(\.kind), ["name", "phone"])
        XCTAssertEqual(r.document.dsl.scope(of: "reservations"), .public)

        let removed = try await ai.morph(transcript: "予約の「お名前」と「電話番号」の欄を消して", appId: "a", currentVersion: 1)
        XCTAssertEqual(removed.publicPersonalData, [])
        XCTAssertNil(removed.document.dsl.entity("reservations")?.field("phone"))

        let privateOnly = try await ai.morph(transcript: "予約を自分だけの記録にして", appId: "a", currentVersion: 2)
        XCTAssertEqual(privateOnly.document.dsl.scope(of: "reservations"), .private)
    }

    func testMockChatAsksOnceBeforePublishingPersonalData() async throws {
        let ai = MockAIClient(delay: .zero)
        var messages = [ChatMessage(role: .user, text: "美容室の予約アプリ。お客さんの名前と電話番号も残したい")]
        let first = try await ai.chat(appId: "a", messages: messages, locale: "ja", mode: .create)
        XCTAssertTrue(first.isPublicConfirm)
        messages += [ChatMessage(role: .assistant, text: first.text), ChatMessage(role: .user, text: "公開してよい")]
        let second = try await ai.chat(appId: "a", messages: messages, locale: "ja", mode: .create)
        XCTAssertFalse(second.isPublicConfirm, "確かめるのは会話で 1 回だけ")
    }
}
