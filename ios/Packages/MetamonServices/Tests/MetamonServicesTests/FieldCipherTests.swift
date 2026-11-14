import XCTest
import CryptoKit
import MetamonDSL
@testable import MetamonServices

/// 暗号化して保存する項目(`Field.encrypted`・#233)。
@MainActor
final class FieldCipherTests: XCTestCase {
    private func envelopeString(_ v: JSONValue?) -> String? {
        if case .string(let s)? = v, s.hasPrefix(FieldCipher.prefix) { return s }
        return nil
    }

    func testEnvelopeRoundTrip() throws {
        let key = SymmetricKey(size: .bits256)
        let cipher = FieldCipher(keys: MemoryFieldKeyStore(key: key))
        let a = try XCTUnwrap(cipher.seal("今日は疲れた", with: key))
        let b = try XCTUnwrap(cipher.seal("今日は疲れた", with: key))
        XCTAssertTrue(a.hasPrefix("enc:v1:"))
        XCTAssertFalse(a.contains("疲れた"))
        XCTAssertNotEqual(a, b, "nonce が毎回違う")
        XCTAssertEqual(cipher.open(a, with: key), "今日は疲れた")
        XCTAssertEqual(cipher.opened(["body": .string(a), "title": "t"], key: key), ["body": "今日は疲れた", "title": "t"])
    }

    /// 鍵が無い・別の鍵の端末: 落ちずに置き換えの文を出す。
    func testMissingKeyShowsPlaceholder() throws {
        let key = SymmetricKey(size: .bits256)
        let cipher = FieldCipher(keys: MemoryFieldKeyStore())
        let env = try XCTUnwrap(cipher.seal("秘密", with: key))
        XCTAssertNil(cipher.open(env, with: nil))
        XCTAssertNil(cipher.open(env, with: SymmetricKey(size: .bits256)))
        XCTAssertNil(cipher.open("enc:v1:%%%", with: key), "壊れた封筒も落ちない")
        XCTAssertEqual(cipher.opened(["body": .string(env)], key: nil), ["body": .string(FieldCipher.lockedText)])
    }

    /// 読めない暗号文を、置き換えの文・空・null で上書きしない。鍵が無ければ平文を保存しない。
    func testSealNeverOverwritesCiphertext() throws {
        let other = FieldCipher(keys: MemoryFieldKeyStore(key: SymmetricKey(size: .bits256)))
        let env = try XCTUnwrap(other.seal("秘密", with: XCTUnwrap(other.keys.load())))
        let store = MemoryFieldKeyStore()
        let cipher = FieldCipher(keys: store)
        let stored: [String: JSONValue] = ["body": .string(env)]
        for value: JSONValue in [.string(FieldCipher.lockedText), .string(""), .null] {
            let r = cipher.sealed(["body": value, "title": "t"], fields: ["body"], stored: stored, mayCreateKey: false)
            XCTAssertNil(r.data["body"], "\(value)")
            XCTAssertEqual(r.data["title"], "t")
            XCTAssertTrue(r.unsaved.isEmpty)
        }
        let typed = cipher.sealed(["body": "新しい文"], fields: ["body"], stored: stored, mayCreateKey: false)
        XCTAssertNil(typed.data["body"], "鍵が無いので平文は送らない")
        XCTAssertEqual(typed.unsaved, ["body"])
        XCTAssertNil(store.load(), "別の端末の鍵の同期を待つ間は鍵を作らない")
    }

    /// 端末 A で保存 → ローカル DB と送信の列は暗号文・画面は平文。鍵の無い端末 B では置き換えの文、保存しても暗号文は残る。
    func testEngineStoresCiphertextAndKeepsItOnLockedDevice() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-enc-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: dir) }
        let a = RecordSyncEngine(client: InMemoryAppConfigClient(), connectivity: ManualConnectivity(online: false), directory: dir)
        let keyA = MemoryFieldKeyStore()
        a.cipher = FieldCipher(keys: keyA)
        a.encryptedFields["app"] = ["diary": ["body"]]

        let written = await a.write(appId: "app", entity: "diary", recordId: nil, data: ["title": "月曜", "body": "誰にも見せない"])
        XCTAssertNotNil(keyA.load(), "最初の暗号化で鍵を作る")
        XCTAssertEqual(written.records.first?.data["body"], "誰にも見せない")
        let raw = await a.store.rows(appId: "app", entity: "diary")
        XCTAssertNotNil(envelopeString(raw.first?.data["body"]))
        XCTAssertEqual(raw.first?.data["title"], "月曜")
        let ops = await a.queue.operations
        let env = try XCTUnwrap(envelopeString(ops.first?.data["body"]), "サーバーへは暗号文だけ")

        // 鍵がまだ同期されていない端末 B(同じ DB を開く)
        let b = RecordSyncEngine(client: InMemoryAppConfigClient(), connectivity: ManualConnectivity(online: false), directory: dir)
        let keyB = MemoryFieldKeyStore()
        b.cipher = FieldCipher(keys: keyB)
        b.encryptedFields["app"] = ["diary": ["body"]]
        var events: [SyncEvent] = []
        b.onEvent = { events.append($0) }
        let shown = await b.cached(appId: "app", entity: "diary")
        XCTAssertEqual(shown.first?.data["body"], .string(FieldCipher.lockedText))

        let id = try XCTUnwrap(shown.first?.id)
        _ = await b.write(appId: "app", entity: "diary", recordId: id, data: ["title": "火曜", "body": .string(FieldCipher.lockedText)])
        var row = await b.store.row(appId: "app", entity: "diary", recordId: id)
        XCTAssertEqual(envelopeString(row?.data["body"]), env, "置き換えの文で暗号文を上書きしない")
        XCTAssertEqual(row?.data["title"], "火曜")
        XCTAssertTrue(events.isEmpty)

        _ = await b.write(appId: "app", entity: "diary", recordId: id, data: ["body": "B で書いた"])
        row = await b.store.row(appId: "app", entity: "diary", recordId: id)
        XCTAssertEqual(envelopeString(row?.data["body"]), env, "鍵が無い端末は平文を保存しない")
        XCTAssertNil(keyB.load(), "暗号文がある間は鍵を作らない")
        XCTAssertEqual(events.count, 1)
        let lastOps = await b.queue.operations
        XCTAssertFalse(lastOps.contains { $0.data.values.contains(.string("B で書いた")) })

        // 鍵が届いたら読める
        b.cipher = FieldCipher(keys: keyA)
        let unlocked = await b.cached(appId: "app", entity: "diary")
        XCTAssertEqual(unlocked.first?.data["body"], "誰にも見せない")
    }

    func testFieldDecodesEncrypted() throws {
        let json = #"{"schemaVersion":1,"app":{"name":"日記","voiceEntry":"none"},"initialScreen":"home","entities":[{"id":"diary","name":"日記","fields":[{"id":"body","name":"本文","type":"text","encrypted":true},{"id":"n","name":"数","type":"integer","encrypted":true}]}],"screens":[{"id":"home","title":"h","components":[]}]}"#
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        XCTAssertEqual(dsl.encryptedFields, ["diary": ["body"]], "string / text だけ")
    }
}
