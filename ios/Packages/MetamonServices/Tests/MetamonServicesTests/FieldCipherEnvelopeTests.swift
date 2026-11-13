import XCTest
import CryptoKit
import MetamonDSL
@testable import MetamonServices

/// #233: 項目の暗号の包み(isEnvelope・seal / open・opened・sealed)。鍵はテストの中でその場で作る。
final class FieldCipherEnvelopeTests: XCTestCase {
    func testIsEnvelope() {
        XCTAssertTrue(FieldCipher.isEnvelope(.string("enc:v1:abc")))
        XCTAssertTrue(FieldCipher.isEnvelope(.string(FieldCipher.prefix)))
        XCTAssertFalse(FieldCipher.isEnvelope(.string("平文")))
        XCTAssertFalse(FieldCipher.isEnvelope(.string("ENC:V1:abc")), "大文字は封筒でない")
        XCTAssertFalse(FieldCipher.isEnvelope(.string("enc:v2:abc")))
        XCTAssertFalse(FieldCipher.isEnvelope(.string(" enc:v1:abc")))
        XCTAssertFalse(FieldCipher.isEnvelope(.number(1)))
        XCTAssertFalse(FieldCipher.isEnvelope(.null))
        XCTAssertFalse(FieldCipher.isEnvelope(nil))
    }

    func testRoundTripEmptyAndLongText() throws {
        let key = SymmetricKey(size: .bits256)
        let cipher = FieldCipher(keys: MemoryFieldKeyStore(key: key))
        for plain in ["", "a", "😀 改行\nあり", String(repeating: "長", count: 5000)] {
            let env = try XCTUnwrap(cipher.seal(plain, with: key))
            XCTAssertTrue(FieldCipher.isEnvelope(.string(env)))
            XCTAssertEqual(cipher.open(env, with: key), plain)
        }
    }

    func testOpenRejectsNonEnvelopeAndTamperedText() throws {
        let key = SymmetricKey(size: .bits256)
        let cipher = FieldCipher(keys: MemoryFieldKeyStore(key: key))
        let env = try XCTUnwrap(cipher.seal("秘密", with: key))
        XCTAssertNil(cipher.open(String(env.dropFirst(FieldCipher.prefix.count)), with: key), "接頭辞の無い文は開かない")
        var bytes = try XCTUnwrap(Data(base64Encoded: String(env.dropFirst(FieldCipher.prefix.count))))
        bytes[bytes.count - 1] ^= 0x01
        XCTAssertNil(cipher.open(FieldCipher.prefix + bytes.base64EncodedString(), with: key), "書き換えられた暗号文")
        XCTAssertNil(cipher.open(FieldCipher.prefix, with: key), "空の封筒")
    }

    /// 封筒の無い記録はそのまま返す。封筒でない値は鍵が無くても変えない。
    func testOpenedLeavesPlainValues() throws {
        let key = SymmetricKey(size: .bits256)
        let cipher = FieldCipher(keys: MemoryFieldKeyStore(key: key))
        let plain: [String: JSONValue] = ["title": "t", "n": .number(3), "none": .null]
        XCTAssertEqual(cipher.opened(plain, key: nil), plain)
        let env = try XCTUnwrap(cipher.seal("本文", with: key))
        XCTAssertEqual(cipher.opened(["body": .string(env), "title": "t", "n": .number(3)], key: nil, lockedText: "🔒"),
                       ["body": "🔒", "title": "t", "n": .number(3)])
    }

    /// 包む項目だけを暗号にし、他の項目・もう暗号文の値はそのまま。鍵は作ってよい時だけ作る。
    func testSealedEncryptsOnlyEncryptedFields() throws {
        let store = MemoryFieldKeyStore()
        let cipher = FieldCipher(keys: store)
        let data: [String: JSONValue] = ["body": "日記", "title": "見出し", "n": .number(1)]
        let r = cipher.sealed(data, fields: ["body", "missing"], stored: nil, mayCreateKey: true)
        let key = try XCTUnwrap(store.load(), "鍵を作ってよい時は作る")
        guard case .string(let env)? = r.data["body"] else { return XCTFail("body") }
        XCTAssertEqual(cipher.open(env, with: key), "日記")
        XCTAssertEqual(r.data["title"], "見出し", "包まない項目はそのまま")
        XCTAssertEqual(r.data["n"], .number(1))
        XCTAssertNil(r.data["missing"], "書き込みに無い項目は足さない")
        XCTAssertTrue(r.unsaved.isEmpty)

        let again = cipher.sealed(["body": .string(env)], fields: ["body"], stored: nil, mayCreateKey: true)
        XCTAssertEqual(again.data["body"], .string(env), "もう暗号文の値は包み直さない")
    }

    /// 保存済みの値が読める暗号文なら、空・null で消せる。文字以外の値は平文で送らない。
    func testSealedWithReadableStoredValue() throws {
        let key = SymmetricKey(size: .bits256)
        let cipher = FieldCipher(keys: MemoryFieldKeyStore(key: key))
        let stored: [String: JSONValue] = ["body": .string(try XCTUnwrap(cipher.seal("前の文", with: key)))]
        for value: JSONValue in [.string(""), .null] {
            let r = cipher.sealed(["body": value], fields: ["body"], stored: stored, mayCreateKey: false)
            XCTAssertEqual(r.data["body"], value, "自分の鍵で読める暗号文は空で上書きしてよい")
        }
        let locked = cipher.sealed(["body": .string(FieldCipher.lockedText)], fields: ["body"], stored: stored, mayCreateKey: false)
        XCTAssertNil(locked.data["body"], "置き換えの文は保存しない")
        let number = cipher.sealed(["body": .number(5)], fields: ["body"], stored: nil, mayCreateKey: false)
        XCTAssertNil(number.data["body"])
        XCTAssertTrue(number.unsaved.isEmpty)
    }
}
