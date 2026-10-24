import Foundation

/// appconfig が保持するアプリ1本ぶんの DSL ルートドキュメント。
/// サーバー側で JSON Patch 適用済みの「最新版」を丸ごと受け取る(クライアントはパッチ適用しない)。
struct AppDocument: Decodable, Sendable {
    var schemaVersion: Int
    var appId: String
    var version: Int
    var name: String
    var theme: Theme?
    var navigation: Navigation
    var screens: [String: Screen]
    /// 初期 state(省略可)
    var state: JSONValue?

    struct Theme: Decodable, Sendable {
        var accent: String?
        var colorScheme: String?
    }

    enum NavKind: String, Decodable, Sendable {
        case stack, tab
        init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .stack) }
    }

    struct Navigation: Decodable, Sendable {
        var type: NavKind
        /// stack のとき最初に表示する screen id
        var root: String?
        @Lossy var tabs: [Tab]
    }

    struct Tab: Decodable, Identifiable, Sendable {
        var id: String { screen }
        var screen: String
        var title: String
        var icon: String?
    }

    struct Screen: Decodable, Sendable {
        var title: String?
        var scroll: Bool?
        var body: Node
        @Lossy var toolbar: [Node]
        @Lossy var onAppear: [Action]
    }

    /// 単一画面かつ中身が aiVoice のみ(=初期状態)か。
    var isVoiceOnly: Bool {
        guard screens.count == 1, let only = screens.values.first else { return false }
        if case .aiVoice = only.body.component { return true }
        return false
    }

    var rootScreenId: String {
        navigation.root ?? navigation.tabs.first?.screen ?? screens.keys.sorted().first ?? ""
    }

    static func bundled(_ name: String, bundle: Bundle = .main) throws -> AppDocument {
        guard let url = bundle.url(forResource: name, withExtension: "json") else {
            throw CocoaError(.fileNoSuchFile, userInfo: [NSFilePathErrorKey: "\(name).json"])
        }
        return try decode(Data(contentsOf: url))
    }

    static func decode(_ data: Data) throws -> AppDocument {
        try JSONDecoder().decode(AppDocument.self, from: data)
    }

    /// バンドル読み込みすら失敗した時の最終手段。
    static let fallback: AppDocument = {
        let json = """
        {"schemaVersion":1,"appId":"fallback","version":0,"name":"Morph",
         "navigation":{"type":"stack","root":"home"},
         "screens":{"home":{"title":"Morph","body":{"type":"text","text":"DSL を読み込めませんでした"}}}}
        """
        return try! decode(Data(json.utf8))
    }()
}
