import Foundation

// DSL v1 (docs/dsl/dsl.schema.json) の Swift 表現。
// 方針: 未知の type / キー / enum 値はエラーにせず無視・fallback する(AI 出力の揺れ対策)。
// backend は厳格に検証するが、iOS が受ける JSON に「未知」が混ざっても落ちない。

/// appconfig の `DSLEnvelope`(`GET /apps/{appId}/dsl`)。version は本文でなく封筒に載る。
public struct AppDocument: Decodable, Sendable {
    public var appId: String
    /// 単調増加。iOS はこれが進んだ時だけ再描画する(ホットリロードの基準)。
    public var version: Int
    public var dsl: AppDSL

    public init(appId: String, version: Int, dsl: AppDSL) {
        self.appId = appId
        self.version = version
        self.dsl = dsl
    }

    private enum Keys: String, CodingKey { case appId, version, dsl }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        appId = try c.decodeIfPresent(String.self, forKey: .appId) ?? "local"
        version = try c.decodeIfPresent(Int.self, forKey: .version) ?? 0
        dsl = try c.decode(AppDSL.self, forKey: .dsl)
    }

    /// バンドルされた DSL 例(docs/dsl/examples/*.json をそのまま同梱)を封筒に包んで読む。
    public static func bundled(_ name: String, version: Int = 1, bundle: Bundle = .main) throws -> AppDocument {
        AppDocument(appId: "local", version: version, dsl: try AppDSL.bundled(name, bundle: bundle))
    }

    public static func decode(_ data: Data) throws -> AppDocument {
        try JSONDecoder().decode(AppDocument.self, from: data)
    }

    /// バンドル読み込みすら失敗した時の最終手段。
    public static let fallback = AppDocument(appId: "fallback", version: 0, dsl: .fallback)
}

/// DSL 本文(schema root)。
public struct AppDSL: Decodable, Sendable {
    public var schemaVersion: Int
    public var app: AppMeta
    public var initialScreen: String
    @Lossy public var entities: [Entity]
    @Lossy public var screens: [Screen]
    /// サーバーの自動の動き(トップの `serverRules`)の数。中身は iOS では使わない(#340)。設定の「自動の動き」を出すかだけに使う(#347)。
    public var serverRuleCount: Int = 0

    public func screen(_ id: String) -> Screen? { screens.first { $0.id == id } }
    public func entity(_ id: String) -> Entity? { entities.first { $0.id == id } }

    /// 画面の描画に要る entity(部品が直接使うもの + computed / Stat.value の agg 先 + ref の参照先を推移的に)。
    public func entitiesNeeded(by screen: Screen) -> Set<String> {
        var out = screen.referencedEntities
        for c in screen.allComponents { out.formUnion(c.valueExpr?.referencedEntities ?? []) }
        // {"setting": ...}(#267)はどの式からも読めるので、single の entity(1 件ずつ)はいつも読む
        out.formUnion(entities.filter(\.single).map(\.id))
        var queue = Array(out)
        while let id = queue.popLast() {
            guard let e = entity(id) else { continue }
            var next = Set<String>()
            for f in e.fields {
                next.formUnion(f.computed?.referencedEntities ?? [])
                if let r = f.refEntity { next.insert(r) } // ref / refs
            }
            for n in next.subtracting(out) { out.insert(n); queue.append(n) }
        }
        return out
    }

    /// 初期状態(音声画面のみ)か。
    public var isVoiceOnly: Bool { app.voiceEntry == .fullscreen }

    /// タブ表示時の初期タブ(= tab.screen)。initialScreen がタブの根ならそれ、違えば先頭タブ。タブ無しなら空文字。
    public var initialTab: String {
        app.tabs.first { $0.screen == initialScreen }?.screen ?? app.tabs.first?.screen ?? ""
    }

    public static func bundled(_ name: String, bundle: Bundle = .main) throws -> AppDSL {
        guard let url = bundle.url(forResource: name, withExtension: "json") else {
            throw CocoaError(.fileNoSuchFile, userInfo: [NSFilePathErrorKey: "\(name).json"])
        }
        return try load(contentsOf: url)
    }

    public static func load(contentsOf url: URL) throws -> AppDSL {
        try JSONDecoder().decode(AppDSL.self, from: Data(contentsOf: url))
    }

    public static let fallback: AppDSL = {
        let json = """
        {"schemaVersion":1,"app":{"name":"Tailor It","voiceEntry":"none"},"initialScreen":"home","entities":[],
         "screens":[{"id":"home","title":"Tailor It","components":[{"type":"text","text":"DSL を読み込めませんでした"}]}]}
        """
        return try! JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }()
}

extension AppDSL {
    private enum Keys: String, CodingKey { case schemaVersion, app, initialScreen, entities, screens, serverRules }

    /// 毎日リセットのチェック(#132)の toggle field は、デコード時に新規作成の既定値(今日)を外す。
    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        schemaVersion = try c.decode(Int.self, forKey: .schemaVersion)
        app = try c.decode(AppMeta.self, forKey: .app)
        initialScreen = try c.decode(String.self, forKey: .initialScreen)
        _entities = try c.decode(Lossy<Entity>.self, forKey: .entities)
        _screens = try c.decode(Lossy<Screen>.self, forKey: .screens)
        // 形を問わず数だけ(未知の形でも落とさない)
        serverRuleCount = (try? c.decodeIfPresent([JSONValue].self, forKey: .serverRules))?.count ?? 0
        clearDailyToggleDefaults()
    }
}

public enum VoiceEntry: String, Decodable, Sendable {
    case fullscreen, floating, none
    public init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .floating) }
}

public struct AppMeta: Decodable, Sendable {
    public var name: String
    public var description: String?
    public var icon: String?
    public var theme: Theme?
    public var voiceEntry: VoiceEntry
    @Lossy public var notifications: [NotificationSpec]
    /// 2〜5 個ならタブバー表示。空なら単一スタック。
    @Lossy public var tabs: [Tab]

    public var hasTabs: Bool { tabs.count >= 2 }

    private enum Keys: String, CodingKey { case name, description, icon, theme, voiceEntry, notifications, tabs }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? "Tailor It"
        description = try c.decodeIfPresent(String.self, forKey: .description)
        icon = try c.decodeIfPresent(String.self, forKey: .icon)
        theme = try? c.decodeIfPresent(Theme.self, forKey: .theme)
        voiceEntry = try c.decodeIfPresent(VoiceEntry.self, forKey: .voiceEntry) ?? .floating
        _notifications = try c.decode(Lossy<NotificationSpec>.self, forKey: .notifications)
        _tabs = try c.decode(Lossy<Tab>.self, forKey: .tabs)
    }
}

/// 下タブ 1 つ。各タブは `screen` を根にした独立の NavigationStack。
public struct Tab: Decodable, Identifiable, Sendable {
    public var id: String { screen }
    public var screen: String
    public var title: String
    public var icon: String?
}

public struct Theme: Decodable, Sendable {
    /// "#RRGGBB"
    public var primaryColor: String?
    /// system | light | dark
    public var mode: String?
}

/// ローカル通知(P0)。`app.notifications[]` は DSL ロード時に同期、`scheduleNotification` アクションは押下時に予約。
public struct NotificationSpec: Decodable, Identifiable, Sendable {
    /// monthly / yearly(#266)はレコード基準(date / datetime の field)だけ。未知の値は once(古い版は monthly を once として読む)。
    public enum Repeat: String, Decodable, Sendable {
        case once, daily, weekly, monthly, yearly
        public init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .once) }
    }

    public var id: String
    public var title: String
    public var body: String
    public var `repeat`: Repeat
    /// "HH:MM" ローカル時刻
    public var time: String?
    /// mon..sun(weekly のみ)
    public var weekday: String?
    /// レコードの時刻から通知(#228・app.notifications のみ): `entity` の各レコードの `field`
    /// (datetime / date / "HH:mm" 文字列)に `offsetMinutes` を足した時刻。
    public var entity: String?
    public var field: String?
    public var offsetMinutes: Int?
    /// 本文に出すレコードの field(件名・アラーム名)
    public var titleField: String?
    /// boolean field。false のレコードは通知しない(アラームのオン/オフ)
    public var enabledField: String?
    /// 条件で知らせる(#265・app.notifications のみ): 真偽の式(ref 不可・agg で集計)。偽 → 真に変わった時に 1 回すぐ知らせる。
    /// 時刻・レコードの日付の通知とは組み合わせない(backend が拒否)。式が読めない通知は `Lossy` が落とす(誤って時刻で鳴らさない)。
    public var when: Expr?
    /// 止めるまで鳴るアラーム(iOS 26.1 以降の AlarmKit)。使えない時(古い iOS・許可が無い・審査用の版)は同じ時刻の普通の通知。
    /// 古い版はこのキーを知らずに普通の通知として予約する(それが落とし方)。
    public var alarm: Bool?

    public var isRecordBased: Bool { when == nil && entity?.isEmpty == false && field?.isEmpty == false }
    /// 条件の通知(#265)。予約(時刻・レコードの日付)には入れず、レコードの保存・同期の後に評価する。
    public var isConditional: Bool { when != nil }

    public init(
        id: String, title: String, body: String = "", repeat: Repeat = .once, time: String? = nil, weekday: String? = nil,
        entity: String? = nil, field: String? = nil, offsetMinutes: Int? = nil, titleField: String? = nil, enabledField: String? = nil,
        when: Expr? = nil, alarm: Bool? = nil
    ) {
        self.id = id
        self.title = title
        self.body = body
        self.repeat = `repeat`
        self.time = time
        self.weekday = weekday
        self.entity = entity
        self.field = field
        self.offsetMinutes = offsetMinutes
        self.titleField = titleField
        self.enabledField = enabledField
        self.when = when
        self.alarm = alarm
    }
}

public struct Entity: Decodable, Identifiable, Sendable {
    public var id: String
    public var name: String
    public var titleField: String?
    /// "private"(省略時)/ "shared" / "public"。未知の値は private 扱い。
    public var scope: String?
    /// 共有レコードを誰が更新できるか(#252): "author"(省略時)/ "members"。削除はどちらでも作った人だけ。
    public var edit: String?
    /// 1 人 1 回の印(いいね・保存・#255)。shared / public の entity だけ。
    @Lossy public var marks: [Mark] = []
    /// 記録の保存期間(日数・#482)。作ってからこの日数を過ぎたレコードを端末が消す(`RecordRetention`)。nil = 消さない。
    @LossyPositiveInt public var retentionDays: Int?
    /// 仲間探しでの役割(#507): "profile"(みんなが見るプロフィール)/ "matched"(つながった人だけが読む)。未知の値は無視。
    public var match: String?
    /// 設定値の置き場(#267)。レコードは常に 1 件(`SingleRecord`)。フォームは開くとその 1 件の編集、一覧には出さない。式 `{"setting": ...}` で読む。
    @LossyBool public var single: Bool = false
    /// 一意制約(#273)。field id か "$author" の組が同じレコードは 1 件だけ(サーバーが 409 で断る)。端末は表示に使わない。
    public var unique: [String]?
    /// 共有レコードを誰が消せるか(#272): "author"(省略時)/ "admins" / "authorOrAdmins"。`RecordPermission.canDelete` が読む。
    public var deleteBy: String?
    /// 宛先の field id(#271)。この entity の記録は作った人と宛先の人だけが読める(サーバーが一覧から外す)。端末は表示に使わない。
    public var addressedTo: String?
    @Lossy public var fields: [Field]

    /// field id、または予約値(`$author` / `$createdAt`・#250)の仮 Field。
    public func field(_ id: String) -> Field? { fields.first { $0.id == id } ?? RecordValue.field(id, in: self) }

    /// レコードが appId 共通の置き場にあり、他の人のレコードも混ざる(shared / public)。
    public var isShared: Bool { scope == "shared" || scope == "public" }
    /// 作った人以外も更新できる(shared / public かつ edit:"members")。
    public var membersCanEdit: Bool { isShared && edit == "members" }
    /// 仲間探しのプロフィール(#507)。「つながりたい」の宛先はそのレコードの author。
    public var isMatchProfile: Bool { isShared && match == "profile" }
    /// つながった人だけが読む記録(#507)。つながりが変わったら読み直す。
    public var isMatchMatched: Bool { isShared && match == "matched" }

    /// 保存対象(computed 以外)のフィールド。フォームのドラフト・送信はこちらだけ。
    public var storedFields: [Field] { fields.filter { !$0.isComputed } }
    public var computedFields: [Field] { fields.filter(\.isComputed) }

    /// 一覧の見出しに使う field(titleField → 最初の string field → 最初の field)。hidden の field は見出しにしない(#262)。
    public var displayTitleField: String? {
        let visible = storedFields.filter { !$0.hidden }
        if let titleField, field(titleField)?.hidden != true { return titleField }
        return visible.first { $0.type == .string }?.id ?? visible.first?.id
    }

    /// hidden(#262)の field id。
    public var hiddenFieldIds: Set<String> { Set(fields.filter(\.hidden).map(\.id)) }
}

public enum FieldType: String, Decodable, Sendable {
    case string, text, integer, number, boolean, date, datetime, `enum`, currency, photo, location
    /// 端末内の動画("video:<id>|<サムネイル data URL>"・#206)。`VideoValue` で読む。
    case video
    /// 端末内の録音("audio:<id>|<秒>"・#420)。`AudioValue` で読む。
    case audio
    /// 別 entity のレコード参照(保存値はレコード id 文字列・#101)。`Field.refEntity` が参照先。
    case ref
    /// 別 entity のレコードの複数選択(保存値はレコード id の配列・#231)。空 / 無し = 全員(割り勘する人)。
    /// 式では `len`(選んだ件数)と filter op `contains` だけが読む。
    case refs
    /// 共有データの参加者(保存値は uid・#272)。入力は参加者の一覧から選び、一覧には表示名を出す。
    case member
    /// 共有データの参加者の複数選択(保存値は uid の配列・#271)。グループ宛てのメッセージの宛先。
    case members
    public init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .string) }

    public var isNumeric: Bool { self == .integer || self == .number || self == .currency }
    public var isDate: Bool { self == .date || self == .datetime }
}

public struct Field: Decodable, Identifiable, Sendable {
    public var id: String
    public var name: String
    public var type: FieldType
    public var required: Bool?
    public var options: [String]?
    /// リテラル既定値。date/datetime は "now" 可。
    public var defaultValue: JSONValue?
    public var unit: String?
    /// type=ref / refs の参照先 entity id。
    public var refEntity: String?
    /// 読み取り専用の算出フィールド(#101)。行コンテキスト = そのレコード。**Record には保存しない**。
    public var computed: Expr?
    /// "weight": 保存した値をヘルスケアの体重にも書き込む(#167)。
    public var health: String?
    /// 回答者に見せない項目(クイズの正解・#262)。レコードの作者のフォームにだけ出し、一覧・表・グラフ・見出しには出さない。
    /// 式(比較)からは読める。値は端末に届いているので厳密な秘匿ではない(docs/dsl/expressions.md §7.6)。
    public var hidden: Bool
    /// "admins" = 管理者だけが入れる・変えられる項目(#272)。サーバーが守り、端末は断られた理由を出す。
    public var editableBy: String?

    public var isComputed: Bool { computed != nil }
    /// 管理者だけが変えられる項目(#272)。
    public var isAdminOnly: Bool { editableBy == "admins" }

    /// デコードで読むキー全部。スキーマとのずれ検出テスト(#537)が使う。
    static let codingKeyNames = Set(Keys.allCases.map(\.rawValue))

    private enum Keys: String, CodingKey, CaseIterable { case id, name, type, required, options, defaultValue, unit, refEntity, computed, health, hidden, editableBy }

    /// 予約値の仮 Field 用(`RecordValue`)。
    init(id: String, name: String, type: FieldType) {
        self.id = id
        self.name = name
        self.type = type
        self.hidden = false
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        id = try c.decode(String.self, forKey: .id)
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? id
        type = try c.decodeIfPresent(FieldType.self, forKey: .type) ?? .string
        required = try? c.decodeIfPresent(Bool.self, forKey: .required)
        options = try? c.decodeIfPresent([String].self, forKey: .options)
        defaultValue = try? c.decodeIfPresent(JSONValue.self, forKey: .defaultValue)
        unit = try? c.decodeIfPresent(String.self, forKey: .unit)
        refEntity = try? c.decodeIfPresent(String.self, forKey: .refEntity)
        computed = try? c.decodeIfPresent(Expr.self, forKey: .computed)
        health = try? c.decodeIfPresent(String.self, forKey: .health)
        hidden = (try? c.decodeIfPresent(Bool.self, forKey: .hidden)) ?? false
        editableBy = try? c.decodeIfPresent(String.self, forKey: .editableBy)
    }
}

public struct Screen: Decodable, Identifiable, Sendable {
    public var id: String
    public var title: String
    /// 指定時はこの entity のフォーム画面(入力系 Leaf の `field` はドラフトのキー)。recordId 付きで開かれたら編集モード。
    public var entity: String?
    @Lossy public var toolbar: [ToolbarButton]
    @Lossy public var components: [Component]

    private enum Keys: String, CodingKey { case id, title, entity, toolbar, components }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        id = try c.decode(String.self, forKey: .id)
        title = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        entity = try c.decodeIfPresent(String.self, forKey: .entity)
        _toolbar = try c.decode(Lossy<ToolbarButton>.self, forKey: .toolbar)
        _components = try c.decode(Lossy<Component>.self, forKey: .components)
    }

    /// 画面内の全コンポーネント(コンテナの子も含む・2段固定なので再帰不要)。
    public var allComponents: [Component] { components.flatMap { [$0] + $0.children } }

    /// この画面が参照する entity id(レコード取得対象)。
    public var referencedEntities: Set<String> {
        var s = Set(allComponents.compactMap(\.entity))
        if let entity { s.insert(entity) }
        return s
    }

    /// voiceInput を含む画面はスクロールさせず全面に出す。
    public var isVoiceScreen: Bool { allComponents.contains { $0.type == .voiceInput } }
}

public struct ToolbarButton: Decodable, Identifiable, Sendable {
    public var id: String { label + (icon ?? "") }
    public var label: String
    public var icon: String?
    public var action: Action
}
