// 開発用メニュー(#2079)のつまみ。`METAMON_STG` を立てた構成にしか入らない(DebugMenuPolicy.swift の頭の説明)。
#if METAMON_STG
import Foundation

/// 開発用のつまみの**読み口**(Android の `DebugKnobs.kt` と同じ名前)。
///
/// 読む順番は 1 つだけ: **起動時の環境変数 → メニューで保存した値 → 無し**。
/// 名前は今の環境変数と同じ `METAMON_…` にしてあるので、UI テスト・E2E の渡し方(`launchEnvironment`)はそのまま使える。
/// 環境変数で渡した起動では、メニューで保存した値に関係なく環境変数が勝つ(テストの結果が端末の設定で変わらない)。
///
/// 保存先は `UserDefaults` の `metamon.debug.<名前>`。「全部元に戻す」で `metamon.debug.*` だけを消す
/// (`metamon.pro.limitDisabled` などアプリ本体の設定は消さない)。
enum DebugKnobs {
    /// メニューで保存した値のキーの接頭辞。
    static let defaultsPrefix = "metamon.debug."

    static func defaultsKey(_ name: String) -> String { defaultsPrefix + name }

    /// 値の出どころ。メニューで「環境変数で固定中(いま変えても効かない)」を出すのに使う。
    enum Source: Equatable {
        /// 起動時の環境変数。メニューからは変えられない
        case environment
        /// メニューで保存した値
        case saved
        /// どちらも無い(既定のまま)
        case none
    }

    static func source(_ name: String,
                       environment: [String: String] = ProcessInfo.processInfo.environment,
                       defaults: UserDefaults = .standard) -> Source {
        if nonEmpty(environment[name]) != nil { return .environment }
        if nonEmpty(defaults.string(forKey: defaultsKey(name))) != nil { return .saved }
        return .none
    }

    /// 文字の値。環境変数が保存値より優先。どちらも無ければ nil。
    static func string(_ name: String,
                       environment: [String: String] = ProcessInfo.processInfo.environment,
                       defaults: UserDefaults = .standard) -> String? {
        nonEmpty(environment[name]) ?? nonEmpty(defaults.string(forKey: defaultsKey(name)))
    }

    /// 入 / 切の値。`1` / `true` / `yes` / `on` が入(大文字小文字は見ない)。
    static func bool(_ name: String, default fallback: Bool = false,
                     environment: [String: String] = ProcessInfo.processInfo.environment,
                     defaults: UserDefaults = .standard) -> Bool {
        guard let raw = string(name, environment: environment, defaults: defaults) else { return fallback }
        return truthy(raw) ?? fallback
    }

    static func int(_ name: String,
                    environment: [String: String] = ProcessInfo.processInfo.environment,
                    defaults: UserDefaults = .standard) -> Int? {
        string(name, environment: environment, defaults: defaults).flatMap(Int.init)
    }

    static func double(_ name: String,
                       environment: [String: String] = ProcessInfo.processInfo.environment,
                       defaults: UserDefaults = .standard) -> Double? {
        string(name, environment: environment, defaults: defaults).flatMap(Double.init)
    }

    /// メニューから値を保存する。nil / 空は「保存値を消す」。
    static func save(_ value: String?, for name: String, defaults: UserDefaults = .standard) {
        if let value = nonEmpty(value) {
            defaults.set(value, forKey: defaultsKey(name))
        } else {
            defaults.removeObject(forKey: defaultsKey(name))
        }
    }

    static func save(_ value: Bool, for name: String, defaults: UserDefaults = .standard) {
        save(value ? "1" : "0", for: name, defaults: defaults)
    }

    /// 「全部元に戻す」。`metamon.debug.*` のキーだけを消して、消したキーを返す。
    @discardableResult
    static func resetAll(defaults: UserDefaults = .standard) -> [String] {
        let keys = defaults.dictionaryRepresentation().keys.filter { $0.hasPrefix(defaultsPrefix) }.sorted()
        for key in keys { defaults.removeObject(forKey: key) }
        return keys
    }

    /// `1` / `true` / `yes` / `on` を入、`0` / `false` / `no` / `off` を切。それ以外は nil(読めない)。
    static func truthy(_ raw: String) -> Bool? {
        switch raw.trimmingCharacters(in: .whitespaces).lowercased() {
        case "1", "true", "yes", "on": true
        case "0", "false", "no", "off": false
        default: nil
        }
    }

    private static func nonEmpty(_ value: String?) -> String? {
        guard let value, !value.isEmpty else { return nil }
        return value
    }
}

/// メニューに出すつまみ 1 件の宣言。**子の Issue(#2080 以降)はここに 1 行足すだけで項目が出る**。
struct DebugKnob: Identifiable, Equatable {
    /// 出し方。
    enum Kind: Equatable {
        /// 入 / 切(保存値は `1` / `0`)
        case toggle(default: Bool)
        /// 自由入力
        case text(placeholder: String)
        /// 決まった中から選ぶ(空文字 = 既定)
        case choice(options: [String])
    }

    /// 変えた時の効き方。
    enum Effect: Equatable {
        /// 画面に戻ればもう効いている
        case immediate
        /// 起動時に 1 回だけ読む値なので、アプリを終了して開き直すまで効かない
        case needsRelaunch

        var label: String {
            switch self {
            case .immediate: "すぐ効く"
            case .needsRelaunch: "起動し直しが要る"
            }
        }
    }

    /// 起動時の環境変数と**同じ名前**(`METAMON_…`)。保存先は `metamon.debug.<名前>`
    let name: String
    /// 行に出す日本語の名前(開発者向けなので訳さない = xcstrings に入れない)
    let title: String
    /// 1 行の補足(無ければ nil)
    let note: String?
    let kind: Kind
    let effect: Effect

    var id: String { name }

    init(name: String, title: String, note: String? = nil, kind: Kind, effect: Effect) {
        self.name = name
        self.title = title
        self.note = note
        self.kind = kind
        self.effect = effect
    }
}

/// メニューに並べるつまみの一覧。
///
/// **この Issue(#2079)では枠だけで中身は空**。子の Issue が、今 `#if DEBUG` で散らばっている環境変数の
/// つまみ(`METAMON_MOCK` ほか)をここに 1 行ずつ足し、読む側を `DebugKnobs.string("METAMON_…")` に置き換える。
enum DebugKnobCatalog {
    static let all: [DebugKnob] = []

    /// 「起動し直しが要る」つまみの名前(メニューが「アプリを終了する」を出すかの判定に使う)。
    static func needsRelaunch(_ name: String) -> Bool {
        all.first { $0.name == name }?.effect == .needsRelaunch
    }
}
#endif
