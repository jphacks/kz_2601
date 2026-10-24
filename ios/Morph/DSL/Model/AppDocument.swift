import Foundation

// DSL v1 (docs/dsl/dsl.schema.json) の Swift 表現。
// 方針: 未知の type / キー / enum 値はエラーにせず無視・fallback する(AI 出力の揺れ対策)。
// backend は厳格に検証するが、iOS が受ける JSON に「未知」が混ざっても落ちない。

/// appconfig の `DSLEnvelope`(`GET /apps/{appId}/dsl`)。version は本文でなく封筒に載る。
struct AppDocument: Decodable, Sendable {
    var appId: String
    /// 単調増加。iOS はこれが進んだ時だけ再描画する(ホットリロードの基準)。
    var version: Int
    var dsl: AppDSL

    init(appId: String, version: Int, dsl: AppDSL) {
        self.appId = appId
        self.version = version
        self.dsl = dsl
    }

    private enum Keys: String, CodingKey { case appId, version, dsl }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        appId = try c.decodeIfPresent(String.self, forKey: .appId) ?? "local"
        version = try c.decodeIfPresent(Int.self, forKey: .version) ?? 0
        dsl = try c.decode(AppDSL.self, forKey: .dsl)
    }

    /// バンドルされた DSL 例(docs/dsl/examples/*.json をそのまま同梱)を封筒に包んで読む。
    static func bundled(_ name: String, version: Int = 1, bundle: Bundle = .main) throws -> AppDocument {
        AppDocument(appId: "local", version: version, dsl: try AppDSL.bundled(name, bundle: bundle))
    }

    static func decode(_ data: Data) throws -> AppDocument {
        try JSONDecoder().decode(AppDocument.self, from: data)
    }

    /// バンドル読み込みすら失敗した時の最終手段。
    static let fallback = AppDocument(appId: "fallback", version: 0, dsl: .fallback)
}

/// DSL 本文(schema root)。
struct AppDSL: Decodable, Sendable {
    var schemaVersion: Int
    var app: AppMeta
    var initialScreen: String
    @Lossy var entities: [Entity]
    @Lossy var screens: [Screen]

    func screen(_ id: String) -> Screen? { screens.first { $0.id == id } }
    func entity(_ id: String) -> Entity? { entities.first { $0.id == id } }

    /// 初期状態(音声画面のみ)か。
    var isVoiceOnly: Bool { app.voiceEntry == .fullscreen }

    static func bundled(_ name: String, bundle: Bundle = .main) throws -> AppDSL {
        guard let url = bundle.url(forResource: name, withExtension: "json") else {
            throw CocoaError(.fileNoSuchFile, userInfo: [NSFilePathErrorKey: "\(name).json"])
        }
        return try JSONDecoder().decode(AppDSL.self, from: Data(contentsOf: url))
    }

    static let fallback: AppDSL = {
        let json = """
        {"schemaVersion":1,"app":{"name":"Morph","voiceEntry":"none"},"initialScreen":"home","entities":[],
         "screens":[{"id":"home","title":"Morph","components":[{"type":"text","text":"DSL を読み込めませんでした"}]}]}
        """
        return try! JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }()
}

enum VoiceEntry: String, Decodable, Sendable {
    case fullscreen, floating, none
    init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .floating) }
}

struct AppMeta: Decodable, Sendable {
    var name: String
    var description: String?
    var icon: String?
    var theme: Theme?
    var voiceEntry: VoiceEntry
    @Lossy var notifications: [NotificationSpec]
    /// 2〜5 個ならタブバー表示。空なら単一スタック。
    @Lossy var tabs: [Tab]

    var hasTabs: Bool { tabs.count >= 2 }

    private enum Keys: String, CodingKey { case name, description, icon, theme, voiceEntry, notifications, tabs }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? "Morph"
        description = try c.decodeIfPresent(String.self, forKey: .description)
        icon = try c.decodeIfPresent(String.self, forKey: .icon)
        theme = try? c.decodeIfPresent(Theme.self, forKey: .theme)
        voiceEntry = try c.decodeIfPresent(VoiceEntry.self, forKey: .voiceEntry) ?? .floating
        _notifications = try c.decode(Lossy<NotificationSpec>.self, forKey: .notifications)
        _tabs = try c.decode(Lossy<Tab>.self, forKey: .tabs)
    }
}

/// 下タブ 1 つ。各タブは `screen` を根にした独立の NavigationStack。
struct Tab: Decodable, Identifiable, Sendable {
    var id: String { screen }
    var screen: String
    var title: String
    var icon: String?
}

struct Theme: Decodable, Sendable {
    /// "#RRGGBB"
    var primaryColor: String?
    /// system | light | dark
    var mode: String?
}

/// ローカル通知(P0)。`app.notifications[]` は DSL ロード時に同期、`scheduleNotification` アクションは押下時に予約。
struct NotificationSpec: Decodable, Identifiable, Sendable {
    enum Repeat: String, Decodable, Sendable {
        case once, daily, weekly
        init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .once) }
    }

    var id: String
    var title: String
    var body: String
    var `repeat`: Repeat
    /// "HH:MM" ローカル時刻
    var time: String?
    /// mon..sun(weekly のみ)
    var weekday: String?
}

struct Entity: Decodable, Identifiable, Sendable {
    var id: String
    var name: String
    var titleField: String?
    @Lossy var fields: [Field]

    func field(_ id: String) -> Field? { fields.first { $0.id == id } }
}

enum FieldType: String, Decodable, Sendable {
    case string, text, integer, number, boolean, date, datetime, `enum`, currency, photo, location
    init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .string) }

    var isNumeric: Bool { self == .integer || self == .number || self == .currency }
    var isDate: Bool { self == .date || self == .datetime }
}

struct Field: Decodable, Identifiable, Sendable {
    var id: String
    var name: String
    var type: FieldType
    var required: Bool?
    var options: [String]?
    /// リテラル既定値。date/datetime は "now" 可。
    var defaultValue: JSONValue?
    var unit: String?
}

struct Screen: Decodable, Identifiable, Sendable {
    var id: String
    var title: String
    /// 指定時はこの entity のフォーム画面(入力系 Leaf の `field` はドラフトのキー)。recordId 付きで開かれたら編集モード。
    var entity: String?
    @Lossy var toolbar: [ToolbarButton]
    @Lossy var components: [Component]

    private enum Keys: String, CodingKey { case id, title, entity, toolbar, components }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        id = try c.decode(String.self, forKey: .id)
        title = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        entity = try c.decodeIfPresent(String.self, forKey: .entity)
        _toolbar = try c.decode(Lossy<ToolbarButton>.self, forKey: .toolbar)
        _components = try c.decode(Lossy<Component>.self, forKey: .components)
    }

    /// 画面内の全コンポーネント(コンテナの子も含む・2段固定なので再帰不要)。
    var allComponents: [Component] { components.flatMap { [$0] + $0.children } }

    /// この画面が参照する entity id(レコード取得対象)。
    var referencedEntities: Set<String> {
        var s = Set(allComponents.compactMap(\.entity))
        if let entity { s.insert(entity) }
        return s
    }

    /// voiceInput を含む画面はスクロールさせず全面に出す。
    var isVoiceScreen: Bool { allComponents.contains { $0.type == .voiceInput } }
}

struct ToolbarButton: Decodable, Identifiable, Sendable {
    var id: String { label + (icon ?? "") }
    var label: String
    var icon: String?
    var action: Action
}
