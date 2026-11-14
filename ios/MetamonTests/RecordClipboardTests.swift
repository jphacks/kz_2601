import XCTest
import UIKit
import MetamonDSL
@testable import MetamonRenderer

/// 記録のコピー・貼り付け(#1871): 載せる項目・項目の対応(id・名前・型の変換・合わない項目)・クリップボードの読み書き。
final class RecordClipboardTests: XCTestCase {
    private func entity(_ json: String) throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(json.utf8))
    }

    private lazy var books: Entity = try! entity("""
    {"id":"books","name":"本","fields":[
      {"id":"title","name":"題名","type":"string"},
      {"id":"author","name":"著者","type":"string"},
      {"id":"pages","name":"ページ数","type":"integer"},
      {"id":"rating","name":"評価","type":"number"},
      {"id":"read","name":"読了","type":"boolean"},
      {"id":"finishedAt","name":"読み終えた日時","type":"datetime"},
      {"id":"genre","name":"ジャンル","type":"enum","options":["小説","技術書"]},
      {"id":"memo","name":"メモ","type":"text","encrypted":true},
      {"id":"answer","name":"答え","type":"string","hidden":true},
      {"id":"cover","name":"表紙","type":"photo"},
      {"id":"shelf","name":"本棚","type":"ref","refEntity":"shelves"},
      {"id":"double","name":"倍","type":"number","computed":{"op":"mul","args":[{"ref":"pages"},2]}}
    ]}
    """)

    private var bookData: [String: JSONValue] {
        ["title": .string("走れメロス"), "author": .string("太宰治"), "pages": .number(32), "rating": .number(4.5),
         "read": .bool(true), "finishedAt": .string("2026-11-01T03:00:00Z"), "genre": .string("小説"),
         "memo": .string("秘密のメモ"), "answer": .string("秘密"), "cover": .string("photo:abc"),
         "shelf": .string("rec-1"), "double": .number(64)]
    }

    private func payload() throws -> RecordClipboard.Payload {
        try XCTUnwrap(RecordClipboard.make(appId: "reading", appName: "読書メモ", entity: books, data: bookData))
    }

    // MARK: - 載せる項目

    func testCopyExcludesEncryptedHiddenMediaRefAndComputed() throws {
        let ids = try payload().fields.map(\.id)
        XCTAssertEqual(ids, ["title", "author", "pages", "rating", "read", "finishedAt", "genre"])
        XCTAssertFalse(ids.contains("memo"), "暗号化した項目の平文は載せない")
    }

    func testCopySkipsNullAndMistypedValues() throws {
        var data = bookData
        data["author"] = .null
        data["pages"] = .string("三十二")
        let p = try XCTUnwrap(RecordClipboard.make(appId: "a", appName: "A", entity: books, data: data))
        XCTAssertFalse(p.fields.contains { $0.id == "author" || $0.id == "pages" })
    }

    func testCopyWithNothingCopyableIsNil() {
        XCTAssertNil(RecordClipboard.make(appId: "a", appName: "A", entity: books, data: ["memo": .string("x"), "cover": .string("photo:1")]))
    }

    func testPlainTextFallback() throws {
        let text = RecordClipboard.plainText(try payload())
        XCTAssertTrue(text.hasPrefix("題名: 走れメロス\n著者: 太宰治\nページ数: 32\n評価: 4.5"))
        XCTAssertFalse(text.contains("秘密"))
    }

    // MARK: - 対応

    func testSameEntityMatchesById() throws {
        let r = RecordClipboard.paste(try payload(), into: books, appId: "reading")
        XCTAssertEqual(r.values["title"], .string("走れメロス"))
        XCTAssertEqual(r.values["pages"], .number(32))
        XCTAssertEqual(r.values["read"], .bool(true))
        XCTAssertEqual(r.matched, ["題名", "著者", "ページ数", "評価", "読了", "読み終えた日時", "ジャンル"])
        XCTAssertEqual(r.unmatched, [])
        XCTAssertNil(r.values["memo"])
    }

    func testOtherAppMatchesByNameAndCoercesTypes() throws {
        let club = try entity("""
        {"id":"club","name":"読書会","fields":[
          {"id":"bookTitle","name":"題名","type":"text"},
          {"id":"writer","name":" 著者 ","type":"string"},
          {"id":"p","name":"ページ数","type":"number"},
          {"id":"score","name":"評価","type":"integer"},
          {"id":"day","name":"読み終えた日時","type":"date"},
          {"id":"kind","name":"ジャンル","type":"enum","options":["技術書","雑誌"]},
          {"id":"memo","name":"メモ","type":"text"}
        ]}
        """)
        let r = RecordClipboard.paste(try payload(), into: club, appId: "club")
        XCTAssertEqual(r.values["bookTitle"], .string("走れメロス"), "string → text")
        XCTAssertEqual(r.values["writer"], .string("太宰治"), "名前は前後の空白を無視")
        XCTAssertEqual(r.values["p"], .number(32), "integer → number")
        XCTAssertNil(r.values["score"], "4.5 は integer に入れない")
        XCTAssertNotNil(r.values["day"], "datetime → date")
        XCTAssertNil(r.values["kind"], "選択肢に無い値は入れない")
        XCTAssertNil(r.values["memo"])
        XCTAssertEqual(r.matched, ["題名", " 著者 ", "ページ数", "読み終えた日時"])
        XCTAssertEqual(r.unmatched, ["評価", "読了", "ジャンル"])
    }

    func testCoercionRules() {
        XCTAssertEqual(RecordClipboard.coerce(.string("12"), from: "string", to: field("integer")), .number(12))
        XCTAssertNil(RecordClipboard.coerce(.string("1.5"), from: "string", to: field("integer")))
        XCTAssertEqual(RecordClipboard.coerce(.number(3), from: "currency", to: field("string")), .string("3"))
        XCTAssertEqual(RecordClipboard.coerce(.number(3), from: "number", to: field("integer")), .number(3))
        XCTAssertEqual(RecordClipboard.coerce(.string("A"), from: "enum", to: field("text")), .string("A"))
        XCTAssertEqual(RecordClipboard.coerce(.string("A"), from: "string", to: field("enum", options: ["A", "B"])), .string("A"))
        XCTAssertNil(RecordClipboard.coerce(.bool(true), from: "boolean", to: field("string")))
        XCTAssertNil(RecordClipboard.coerce(.string("2026-11-01"), from: "date", to: field("datetime")))
        XCTAssertNil(RecordClipboard.coerce(.string("rec-1"), from: "ref", to: field("ref")), "ref はアプリをまたいで意味を持たない")
        XCTAssertNil(RecordClipboard.coerce(.number(1), from: "string", to: field("string")), "宣言と値の型が違うものは入れない")
    }

    func testEachTargetFieldIsFilledOnce() throws {
        let target = try entity("""
        {"id":"t","name":"t","fields":[{"id":"name","name":"名前","type":"string"}]}
        """)
        let p = RecordClipboard.Payload(v: 1, appId: "a", appName: "A", entityId: "s", entityName: "s", fields: [
            .init(id: "a", name: "名前", type: "string", value: .string("一")),
            .init(id: "b", name: "名前", type: "string", value: .string("二")),
        ])
        let r = RecordClipboard.paste(p, into: target, appId: "t")
        XCTAssertEqual(r.values, ["name": .string("一")])
        XCTAssertEqual(r.unmatched, ["名前"])
    }

    func testDoesNotPasteIntoEncryptedOrHiddenTargets() throws {
        let target = try entity("""
        {"id":"t","name":"t","fields":[{"id":"title","name":"題名","type":"string","encrypted":true},
         {"id":"author","name":"著者","type":"string","hidden":true}]}
        """)
        let r = RecordClipboard.paste(try payload(), into: target, appId: "t")
        XCTAssertTrue(r.values.isEmpty)
        XCTAssertTrue(r.matched.isEmpty)
    }

    private func field(_ type: String, options: [String]? = nil) -> Field {
        let opts = options.map { ",\"options\":[\($0.map { "\"\($0)\"" }.joined(separator: ","))]" } ?? ""
        return try! JSONDecoder().decode(Field.self, from: Data("{\"id\":\"f\",\"name\":\"f\",\"type\":\"\(type)\"\(opts)}".utf8))
    }

    func testOtherAppWithSameEntityIdMatchesByNameNotId() throws {
        // 別のアプリにたまたま同じ entity id・field id がある。id では合わせない
        let other = try entity("""
        {"id":"books","name":"本","fields":[
          {"id":"title","name":"著者","type":"string"},
          {"id":"author","name":"題名","type":"string"}
        ]}
        """)
        let r = RecordClipboard.paste(try payload(), into: other, appId: "another")
        XCTAssertEqual(r.values["title"], .string("太宰治"), "名前で合わせる")
        XCTAssertEqual(r.values["author"], .string("走れメロス"))
        let same = RecordClipboard.paste(try payload(), into: other, appId: "reading")
        XCTAssertEqual(same.values["title"], .string("走れメロス"), "同じアプリなら id で合わせる")
    }

    func testAdminOnlyTargetsNeedAdmin() throws {
        let target = try entity("""
        {"id":"t","name":"t","fields":[{"id":"title","name":"題名","type":"string","editableBy":"admins"},
         {"id":"author","name":"著者","type":"string"}]}
        """)
        let member = RecordClipboard.paste(try payload(), into: target, appId: "t")
        XCTAssertNil(member.values["title"], "管理者でなければ管理者だけの項目には入れない")
        XCTAssertEqual(member.values["author"], .string("太宰治"))
        XCTAssertEqual(member.unmatched.first, "題名")
        let admin = RecordClipboard.paste(try payload(), into: target, appId: "t", isAdmin: true)
        XCTAssertEqual(admin.values["title"], .string("走れメロス"))
    }

    func testCurrencyIsIntegerAndBounded() {
        XCTAssertEqual(RecordClipboard.coerce(.number(4.5), from: "number", to: field("currency")), .number(5), "currency は四捨五入")
        XCTAssertEqual(RecordClipboard.coerce(.string("1200.4"), from: "string", to: field("currency")), .number(1200))
        XCTAssertEqual(RecordClipboard.coerce(.number(-2.5), from: "number", to: field("currency")), .number(-3))
        XCTAssertNil(RecordClipboard.coerce(.number(1e19), from: "number", to: field("currency")), "サーバーの上限を超える")
        XCTAssertNil(RecordClipboard.coerce(.number(-1e19), from: "number", to: field("integer")))
        XCTAssertEqual(RecordClipboard.coerce(.number(4e18), from: "number", to: field("integer")), .number(4e18))
        XCTAssertEqual(RecordClipboard.coerce(.number(1e19), from: "number", to: field("number")), .number(1e19), "number に上限は無い")
    }

    func testTextOverCharacterLimitIsUnmatched() throws {
        let long = String(repeating: "あ", count: 4001)
        XCTAssertNil(RecordClipboard.coerce(.string(long), from: "text", to: field("string")))
        XCTAssertEqual(RecordClipboard.coerce(.string(long), from: "text", to: field("text")), .string(long), "text は 20000 文字まで")
        XCTAssertNil(RecordClipboard.coerce(.string(String(repeating: "a", count: 20001)), from: "text", to: field("text")))
        let target = try entity("""
        {"id":"t","name":"t","fields":[{"id":"title","name":"題名","type":"string"}]}
        """)
        let p = RecordClipboard.Payload(appId: "a", appName: "A", entityId: "s", entityName: "s",
                                        fields: [.init(id: "x", name: "題名", type: "text", value: .string(long))])
        let r = RecordClipboard.paste(p, into: target, appId: "t")
        XCTAssertTrue(r.values.isEmpty)
        XCTAssertEqual(r.unmatched, ["題名"])
    }

    func testDatesAreValidatedAndReencoded() {
        XCTAssertEqual(RecordClipboard.coerce(.string("2026-11-01"), from: "date", to: field("date")), .string("2026-11-01"))
        XCTAssertEqual(RecordClipboard.coerce(.string("2026-1-5"), from: "date", to: field("date")), .string("2026-01-05"))
        XCTAssertNil(RecordClipboard.coerce(.string("2026-02-30"), from: "date", to: field("date")), "あり得ない日")
        XCTAssertNil(RecordClipboard.coerce(.string("きのう"), from: "date", to: field("date")))
        XCTAssertEqual(RecordClipboard.coerce(.string("2026-11-01T03:00:00Z"), from: "datetime", to: field("datetime")),
                       .string("2026-11-01T03:00:00Z"))
        XCTAssertNil(RecordClipboard.coerce(.string("2026-11-01T25:00:00Z"), from: "datetime", to: field("datetime")))
        XCTAssertNil(RecordClipboard.coerce(.string("not a date"), from: "datetime", to: field("datetime")))
    }

    func testDatetimeWithFractionalSecondsIsAccepted() throws {
        let day = FieldCoding.dateFormatter.string(from: try XCTUnwrap(FieldCoding.date(from: .string("2026-11-01T03:00:00Z"))))
        XCTAssertEqual(RecordClipboard.coerce(.string("2026-11-01T03:00:00.123Z"), from: "datetime", to: field("date")), .string(day))
        XCTAssertNotNil(RecordClipboard.coerce(.string("2026-11-01T03:00:00.123Z"), from: "datetime", to: field("datetime")))
    }

    func testLocationIsValidated() {
        XCTAssertEqual(RecordClipboard.coerce(.string("35.68, 139.76"), from: "location", to: field("location")), .string("35.68,139.76"))
        XCTAssertNil(RecordClipboard.coerce(.string("91,0"), from: "location", to: field("location")), "緯度は 90 まで")
        XCTAssertNil(RecordClipboard.coerce(.string("0,181"), from: "location", to: field("location")), "経度は 180 まで")
        XCTAssertNil(RecordClipboard.coerce(.string("東京"), from: "location", to: field("location")))
        XCTAssertNil(RecordClipboard.coerce(.string("1,2,3"), from: "location", to: field("location")))
    }

    @MainActor
    func testUnreadableMessageTellsDeniedFromEmpty() {
        XCTAssertEqual(RecordPasteboard.unreadableMessage(hasRecord: true), L("貼り付けが許可されませんでした"))
        XCTAssertEqual(RecordPasteboard.unreadableMessage(hasRecord: false), L("貼り付けられる記録がありません"))
        XCTAssertNotEqual(RecordPasteboard.unreadableMessage(hasRecord: true), RecordPasteboard.unreadableMessage(hasRecord: false))
    }

    @MainActor
    func testPasteMessage() {
        let msg = RecordPasteboard.message(for: .init(values: [:], matched: ["題名", "著者"], unmatched: ["評価"]))
        XCTAssertTrue(msg.contains("2"))
        XCTAssertTrue(msg.contains("題名"))
        XCTAssertTrue(msg.contains("評価"))
    }

    // MARK: - 符号化・クリップボード

    func testEncodeDecodeRoundTrip() throws {
        let p = try payload()
        let data = try XCTUnwrap(RecordClipboard.encode(p))
        XCTAssertEqual(RecordClipboard.decode(data), p)
    }

    func testDecodeRejectsGarbageNewerVersionAndOversize() throws {
        XCTAssertNil(RecordClipboard.decode(Data("not json".utf8)))
        var p = try payload()
        p.v = RecordClipboard.version + 1
        XCTAssertNil(RecordClipboard.decode(try NearbyShare.encode(p)))
        XCTAssertNil(RecordClipboard.decode(Data(count: RecordClipboard.maxBytes + 1)))
    }

    func testDecodeClipsLongNamesAndDropsBadIds() throws {
        var p = try payload()
        p.appName = String(repeating: "あ", count: 500)
        p.fields.append(.init(id: "", name: "x", type: "string", value: .string("y")))
        let d = try XCTUnwrap(RecordClipboard.decode(try NearbyShare.encode(p)))
        XCTAssertEqual(d.appName.count, NearbyLimits.maxLabelLength)
        XCTAssertFalse(d.fields.contains { $0.id.isEmpty })
    }

    @MainActor
    func testPasteboardWriteAndRead() throws {
        let board = try XCTUnwrap(UIPasteboard(name: UIPasteboard.Name("record-clipboard-\(UUID().uuidString)"), create: true))
        defer { UIPasteboard.remove(withName: board.name) }
        XCTAssertFalse(RecordPasteboard.hasRecord(on: board))
        let p = try payload()
        XCTAssertTrue(RecordPasteboard.write(p, text: "題名: 走れメロス", to: board))
        XCTAssertTrue(RecordPasteboard.hasRecord(on: board))
        XCTAssertEqual(RecordPasteboard.read(from: board), p)
        XCTAssertEqual(board.string, "題名: 走れメロス", "文字で貼る先向けの文字も載る")
    }
}
