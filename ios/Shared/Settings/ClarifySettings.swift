import Foundation

/// 生成・改変の前にチャットで相談するかの設定(#75 / #186)。既定 ON。OFF は従来どおり一発で反映(create も edit も)。`METAMON_CLARIFY=0/1` があれば UserDefaults より優先(E2E・デモ端末用)。
/// iOS と Mac で共有する(Mac の設定の「対話」と会話のシート・#973 / #967)。
enum ClarifySettings {
    private static let key = "metamon.clarify.enabled"

    static var enabled: Bool {
        get {
            switch ProcessInfo.processInfo.environment["METAMON_CLARIFY"] {
            case "0": return false
            case "1": return true
            default: return UserDefaults.standard.object(forKey: key) as? Bool ?? true
            }
        }
        set { UserDefaults.standard.set(newValue, forKey: key) }
    }

    private static let autoBuildKey = "metamon.clarify.autoBuild"

    /// 音声だけで進めている時、ready で自動生成するか(#160)。既定 ON(デモ本線は声だけで最後まで進むのが主役)。
    /// `METAMON_CLARIFY_AUTOBUILD=0/1` があれば優先。
    static var autoBuild: Bool {
        get {
            switch ProcessInfo.processInfo.environment["METAMON_CLARIFY_AUTOBUILD"] {
            case "0": return false
            case "1": return true
            default: return UserDefaults.standard.object(forKey: autoBuildKey) as? Bool ?? true
            }
        }
        set { UserDefaults.standard.set(newValue, forKey: autoBuildKey) }
    }
}
