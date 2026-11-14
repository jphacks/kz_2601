#if METAMON_STG
import Foundation

/// 開発用のつまみの読み口(#2079・Android の `DebugKnobs.kt` と同じ名前)。
/// 名前は起動時の環境変数と同じ(`METAMON_…`)。環境変数があればそれ、無ければメニューで保存した値。
/// 保存先は stg の版だけの UserDefaults のスイート(キー `metamon.debug.<名前>`)。「全部元に戻す」で消える。
enum DebugKnobs {
    static let suiteName = "jp.kz2601.metamon.debug"
    static let keyPrefix = "metamon.debug."

    nonisolated(unsafe) static var store = DebugKnobStore(
        defaults: UserDefaults(suiteName: suiteName) ?? .standard,
        environment: { ProcessInfo.processInfo.environment })

    static func string(_ name: String) -> String? { store.string(name) }
    static func bool(_ name: String) -> Bool? { store.bool(name) }
    static func set(_ name: String, _ value: String?) { store.set(name, value) }
    static func set(_ name: String, _ value: Bool?) { store.set(name, value) }
    static func isFromEnvironment(_ name: String) -> Bool { store.isFromEnvironment(name) }
    static func resetAll() { store.resetAll() }
}

/// 読み書きの本体(テストでスイートと環境変数を差し替える)
struct DebugKnobStore {
    let defaults: UserDefaults
    let environment: () -> [String: String]

    func string(_ name: String) -> String? {
        if let v = environment()[name] { return v }
        return defaults.string(forKey: DebugKnobs.keyPrefix + name)
    }

    /// "1" / "true" / "yes" を true、"0" / "false" / "no" を false。それ以外・無しは nil
    func bool(_ name: String) -> Bool? {
        switch string(name)?.lowercased() {
        case "1", "true", "yes": true
        case "0", "false", "no": false
        default: nil
        }
    }

    /// nil で消す(既定に戻す)
    func set(_ name: String, _ value: String?) {
        let key = DebugKnobs.keyPrefix + name
        if let value { defaults.set(value, forKey: key) } else { defaults.removeObject(forKey: key) }
    }

    func set(_ name: String, _ value: Bool?) {
        set(name, value.map { $0 ? "1" : "0" })
    }

    /// 起動時の環境変数で決まっていて、メニューで変えても効かないか
    func isFromEnvironment(_ name: String) -> Bool {
        environment()[name] != nil
    }

    func resetAll() {
        for key in defaults.dictionaryRepresentation().keys where key.hasPrefix(DebugKnobs.keyPrefix) {
            defaults.removeObject(forKey: key)
        }
    }
}
#endif
