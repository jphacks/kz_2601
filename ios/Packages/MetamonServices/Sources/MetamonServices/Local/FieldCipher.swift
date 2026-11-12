import Foundation
import CryptoKit
import Security
import MetamonDSL

/// 暗号化して保存する項目(`Field.encrypted`・#233)の鍵の置き場。
public protocol FieldKeyStore: Sendable {
    /// 今ある鍵。無い(別の端末で作った鍵がまだ同期されていない)時は nil。
    func load() -> SymmetricKey?
    /// 鍵を作って保存する。既にあればそれを返す。保存できなければ nil。
    func create() -> SymmetricKey?
}

/// iCloud キーチェーンに置く鍵(`kSecAttrSynchronizable`)。同じ Apple ID の端末で同じ鍵を読む。
/// 同期される項目は「この端末だけ」の保護クラスを使えないので、`AfterFirstUnlock` にする。
public struct KeychainFieldKeyStore: FieldKeyStore {
    static let service = "jp.kz2601.metamon.field-key"
    static let account = "v1"

    public init() {}

    private var query: [String: Any] {
        [kSecClass as String: kSecClassGenericPassword,
         kSecAttrService as String: Self.service,
         kSecAttrAccount as String: Self.account,
         kSecAttrSynchronizable as String: kCFBooleanTrue as Any]
    }

    public func load() -> SymmetricKey? {
        var q = query
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: CFTypeRef?
        guard SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess, let data = out as? Data, data.count == 32 else { return nil }
        return SymmetricKey(data: data)
    }

    public func create() -> SymmetricKey? {
        if let key = load() { return key }
        let key = SymmetricKey(size: .bits256)
        var q = query
        q[kSecValueData as String] = key.withUnsafeBytes { Data($0) }
        q[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
        switch SecItemAdd(q as CFDictionary, nil) {
        case errSecSuccess: return key
        case errSecDuplicateItem: return load() // 同時に作られた・同期が届いた方を使う
        default: return nil
        }
    }
}

/// テスト・モック用の鍵(メモリだけ)。
public final class MemoryFieldKeyStore: FieldKeyStore, @unchecked Sendable {
    private let lock = NSLock()
    private var key: SymmetricKey?

    public init(key: SymmetricKey? = nil) { self.key = key }

    public func load() -> SymmetricKey? { lock.withLock { key } }
    public func create() -> SymmetricKey? {
        lock.withLock {
            if key == nil { key = SymmetricKey(size: .bits256) }
            return key
        }
    }
}

/// 項目の暗号化(AES-GCM)。レコードには `enc:v1:<base64(nonce + 暗号文 + tag)>` を入れ、サーバーは暗号文だけを見る。
/// 読む時は封筒の文字列をすべて復号する(DSL から encrypted を外した後の記録も読める)。読めない(鍵がまだ無い・別の鍵)時は `lockedText`。
/// 書く時は `Field.encrypted` の値だけを暗号化し、置き換えの文・空で暗号文を上書きしない。
public struct FieldCipher: Sendable {
    /// backend `dsl.EncryptedPrefix`
    public static let prefix = "enc:v1:"
    /// 鍵が読めない端末で値の代わりに出す文。保存の時にこの文が来たら、その項目は送らない(暗号文を残す)。
    public static var lockedText: String { L("🔒 この端末ではまだ読めません(iCloud キーチェーンの同期を待っています)") }
    /// 鍵が無くて暗号化できず、その項目を保存しなかった時の知らせ。
    public static var unsavedMessage: String { L("暗号化の鍵がまだこの端末に届いていないため、暗号化する項目は保存しませんでした。iCloud キーチェーンの同期を待ってからもう一度保存してください") }

    public let keys: any FieldKeyStore

    public init(keys: any FieldKeyStore = KeychainFieldKeyStore()) { self.keys = keys }

    public static func isEnvelope(_ v: JSONValue?) -> Bool {
        if case .string(let s)? = v { return s.hasPrefix(prefix) }
        return false
    }

    public func seal(_ plain: String, with key: SymmetricKey) -> String? {
        guard let box = try? AES.GCM.seal(Data(plain.utf8), using: key), let combined = box.combined else { return nil }
        return Self.prefix + combined.base64EncodedString()
    }

    /// 復号。読めなければ nil。
    public func open(_ envelope: String, with key: SymmetricKey?) -> String? {
        guard let key, envelope.hasPrefix(Self.prefix),
              let data = Data(base64Encoded: String(envelope.dropFirst(Self.prefix.count))),
              let box = try? AES.GCM.SealedBox(combined: data),
              let plain = try? AES.GCM.open(box, using: key) else { return nil }
        return String(data: plain, encoding: .utf8)
    }

    /// 表示用: 封筒の値を平文(読めなければ `lockedText`)に置き換える。
    public func opened(_ data: [String: JSONValue], key: SymmetricKey?) -> [String: JSONValue] {
        opened(data, key: key, lockedText: Self.lockedText)
    }

    /// `lockedText` を先に引いて渡す版(言語の設定を main actor の外で読まない)。
    public func opened(_ data: [String: JSONValue], key: SymmetricKey?, lockedText: @autoclosure () -> String) -> [String: JSONValue] {
        guard data.values.contains(where: Self.isEnvelope) else { return data }
        var out = data
        for (k, v) in data {
            guard case .string(let s) = v, s.hasPrefix(Self.prefix) else { continue }
            out[k] = .string(open(s, with: key) ?? lockedText())
        }
        return out
    }

    /// 書き込みの値を暗号化する。
    /// - Parameters:
    ///   - fields: この entity の `Field.encrypted` の id
    ///   - stored: 今ローカルにある値(更新の時。新規は nil)
    ///   - mayCreateKey: 鍵が無い時に作ってよいか(別の端末の暗号文がまだ読めない時は作らない: 作ると鍵が 2 つになる)
    /// - Returns: 書けなかった(落とした)項目のうち、利用者が入れた値があったもの
    public func sealed(_ data: [String: JSONValue], fields: Set<String>, stored: [String: JSONValue]?,
                       mayCreateKey: Bool) -> (data: [String: JSONValue], unsaved: Set<String>) {
        var out = data
        var unsaved = Set<String>()
        var key: SymmetricKey?? = nil // 必要になった時に 1 回だけ引く
        func currentKey() -> SymmetricKey? {
            if case .some(let k) = key { return k }
            let k = keys.load() ?? (mayCreateKey ? keys.create() : nil)
            key = .some(k)
            return k
        }
        for id in fields {
            guard let value = data[id] else { continue }
            let old = stored?[id]
            let oldUnreadable: Bool = {
                guard case .string(let s)? = old, s.hasPrefix(Self.prefix) else { return false }
                return open(s, with: currentKey()) == nil
            }()
            switch value {
            case .string(let s) where s == Self.lockedText:
                out[id] = nil // 読めなかった値をそのまま保存し直した: 暗号文を残す
            case .string(let s) where s.hasPrefix(Self.prefix):
                break // もう暗号文
            case .string(let s) where !s.isEmpty:
                if let k = currentKey(), let env = seal(s, with: k) {
                    out[id] = .string(env)
                } else {
                    out[id] = nil // 鍵が無い: 平文は保存も送信もしない
                    unsaved.insert(id)
                }
            case .string, .null:
                if oldUnreadable { out[id] = nil } // 読めない暗号文を空で上書きしない
            default:
                out[id] = nil // string / text の field に文字以外は来ない。平文では送らない
            }
        }
        return (out, unsaved)
    }
}
