import AuthenticationServices
import Foundation
import MetamonRenderer
import MetamonServices

/// 設定の「つないだサービス」(#1493)を出す版か。審査用の版は OAuth の provider を外す(決定 6)ので出さない。
enum ConnectedServicesPolicy {
    static var isAvailable: Bool {
        #if APP_STORE_REVIEW
        false
        #else
        true
        #endif
    }
}

/// 「つないだサービス」の一覧を取る時に送る `supportedFeatures`(#1493)。
/// 端末は provider の一覧を持たず、サーバーが登録している provider をそのまま出す(nil = 絞らない)ので、
/// 新しい provider はアプリを更新しなくても並ぶ。審査用の版だけ空(= 何も出さない・決定 6)。
/// generate に送る `ClientCapabilities.supportedFeatures` とは別(外を呼ぶ DSL の部品はまだ無い)。
enum ConnectedServicesFeatures {
    static var current: [String]? {
        #if APP_STORE_REVIEW
        list(reviewBuild: true)
        #else
        list(reviewBuild: false)
        #endif
    }

    static func list(reviewBuild: Bool) -> [String]? { reviewBuild ? [] : nil }
}

/// provider の端末側の名前とアイコン。名前はサーバーの `nameJa`/`nameEn` を優先し、無い時だけ使う
/// (鍵なしの provider はサーバーに名前が無いことが多い)。知らない provider は id のまま・アイコンは汎用。
enum ConnectedServiceCatalog {
    private static let names: [String: (ja: String, en: String)] = [
        "asana": ("Asana", "Asana"),
        "coingecko": ("CoinGecko(暗号資産の価格)", "CoinGecko"),
        "connpass": ("connpass", "connpass"),
        "deepl": ("DeepL 翻訳", "DeepL"),
        "estat": ("e-Stat(政府統計)", "e-Stat"),
        "frankfurter": ("為替レート(Frankfurter)", "Exchange rates (Frankfurter)"),
        "github": ("GitHub", "GitHub"),
        "googleCalendar": ("Google カレンダー", "Google Calendar"),
        "googlePlaces": ("Google プレイス", "Google Places"),
        "googleSheets": ("Google スプレッドシート", "Google Sheets"),
        "googleTasks": ("Google ToDo リスト", "Google Tasks"),
        "gtfsRealtime": ("GTFS リアルタイム(交通)", "GTFS Realtime"),
        "linear": ("Linear", "Linear"),
        "natureRemo": ("Nature Remo", "Nature Remo"),
        "nhk": ("NHK 番組表", "NHK program guide"),
        "notion": ("Notion", "Notion"),
        "odpt": ("公共交通オープンデータ(ODPT)", "ODPT (public transport)"),
        "openMeteo": ("Open-Meteo(天気)", "Open-Meteo"),
        "openWeather": ("OpenWeather(天気)", "OpenWeather"),
        "openaiUsage": ("OpenAI の利用量", "OpenAI usage"),
        "outlookCalendar": ("Outlook カレンダー", "Outlook Calendar"),
        "podcasts": ("ポッドキャスト", "Podcasts"),
        "qiita": ("Qiita", "Qiita"),
        "rakutenRecipe": ("楽天レシピ", "Rakuten Recipe"),
        "slack": ("Slack", "Slack"),
        "switchbot": ("SwitchBot", "SwitchBot"),
        "tmdb": ("TMDB(映画・ドラマ)", "TMDB"),
        "todoist": ("Todoist", "Todoist"),
        "vercel": ("Vercel", "Vercel"),
        "wikipedia": ("Wikipedia", "Wikipedia"),
        "withings": ("Withings", "Withings"),
        "youtube": ("YouTube", "YouTube"),
        "zipcloud": ("郵便番号(zipcloud)", "Postal codes (zipcloud)"),
    ]

    private static let symbols: [String: String] = [
        "asana": "circle.hexagongrid",
        "coingecko": "bitcoinsign.circle",
        "connpass": "person.3",
        "deepl": "character.bubble",
        "estat": "chart.bar",
        "frankfurter": "yensign.circle",
        "github": "chevron.left.forwardslash.chevron.right",
        "googleCalendar": "calendar",
        "googlePlaces": "mappin.and.ellipse",
        "googleSheets": "tablecells",
        "googleTasks": "checklist",
        "gtfsRealtime": "tram",
        "linear": "list.bullet.rectangle",
        "natureRemo": "homekit",
        "nhk": "tv",
        "notion": "doc.text",
        "odpt": "tram",
        "openMeteo": "cloud.sun",
        "openWeather": "cloud.sun",
        "openaiUsage": "gauge.with.dots.needle.33percent",
        "outlookCalendar": "calendar",
        "podcasts": "mic",
        "qiita": "text.book.closed",
        "rakutenRecipe": "fork.knife",
        "slack": "number",
        "switchbot": "homekit",
        "tmdb": "film",
        "todoist": "checklist",
        "vercel": "triangle",
        "wikipedia": "book",
        "withings": "heart",
        "youtube": "play.rectangle",
        "zipcloud": "envelope",
    ]

    /// 端末が名前とアイコンを持っている provider(テスト用)
    static var knownIDs: [String] { names.keys.sorted() }

    /// 知らない provider のアイコン
    static let fallbackSymbol = "link"

    /// 知らない provider は id のまま
    static func name(_ id: String, english: Bool) -> String {
        guard let n = names[id] else { return id }
        return english ? n.en : n.ja
    }

    static func symbol(_ id: String) -> String { symbols[id] ?? fallbackSymbol }
}

#if !APP_STORE_REVIEW
/// 一覧の 1 行の状態。
enum ConnectedServiceStatus: Equatable {
    case connected
    /// provider がトークンを拒んだ(`status: error`)
    case needsReconnect
    case notConnected
    /// つながなくても使える(OAuth の要らない provider・「鍵なしで使えるもの」の行。ボタンは無い)
    case keyless
}

struct ConnectedServiceRow: Equatable, Identifiable {
    var provider: String
    var name: String
    var status: ConnectedServiceStatus
    /// このサーバーで今つなげる(鍵なしなら今使える)か(false なら「つなぐ」「つなぎ直す」を出さない。切るのはできる)
    var enabled: Bool = true
    var id: String { provider }

    /// つないでいない(か鍵なし)うえに今使えない(行ごと押せなくする)
    var isUnavailable: Bool { !enabled && (status == .notConnected || status == .keyless) }
    /// 「鍵なしで使えるもの」の行
    var isKeyless: Bool { status == .keyless }
}

extension [ConnectedServiceRow] {
    /// 「つないだサービス」(つなぐ・切るのある行)
    var connectable: [ConnectedServiceRow] { filter { !$0.isKeyless } }
    /// 「鍵なしで使えるもの」
    var keyless: [ConnectedServiceRow] { filter(\.isKeyless) }
}

/// 「つないだサービス」の表示の状態。
enum ConnectedServicesState: Equatable {
    case loading
    /// サーバーが外の API を止めている(503 `ext_unavailable`)。セクションごと出さない
    case hidden
    case failed(String)
    case loaded([ConnectedServiceRow])

    /// 一覧の結果から。ext_unavailable は隠す・ほかの失敗は再試行できる状態。
    static func from(_ result: Result<ExtConnectionList, Error>, english: Bool = LanguagePreference.shared.code == "en") -> ConnectedServicesState {
        switch result {
        case .success(let list):
            return .loaded(rows(list, english: english))
        case .failure(let error):
            if (error as? APIError)?.isExtUnavailable == true { return .hidden }
            return .failed(L10n.tr("一覧を読み込めませんでした。"))
        }
    }

    /// provider の表示名。サーバーの `nameJa`/`nameEn` → 端末の名前 → id の順(#2001 のつなぐ案内でも使う)
    static func name(_ provider: ExtProvider?, id: String, english: Bool) -> String {
        let n = provider.map { english ? $0.nameEn : $0.nameJa } ?? ""
        return n.isEmpty ? ConnectedServiceCatalog.name(id, english: english) : n
    }

    /// つないだもの(サーバーの providers の順 → 知らないもの)の後に、まだつないでいない provider、最後に鍵なしで使えるもの。
    /// `providers` が無い古いサーバーでは、つないだものだけ出す。
    static func rows(_ list: ExtConnectionList, english: Bool) -> [ConnectedServiceRow] {
        let providers = list.providers ?? []
        let byId = Dictionary(providers.map { ($0.id, $0) }, uniquingKeysWith: { a, _ in a })
        let order = providers.map(\.id)
        func name(_ id: String) -> String { Self.name(byId[id], id: id, english: english) }
        // 古いサーバーは enabled を知らせないので、つないだものはつなぎ直せる扱い
        func enabled(_ id: String) -> Bool { byId[id]?.enabled ?? (list.providers == nil) }
        func rank(_ id: String) -> Int { order.firstIndex(of: id) ?? order.count }

        var seen = Set<String>()
        let connected = list.connections
            .filter { seen.insert($0.provider).inserted }
            .sorted { rank($0.provider) < rank($1.provider) }
            .map { ConnectedServiceRow(provider: $0.provider, name: name($0.provider),
                                       status: $0.status == .active ? .connected : .needsReconnect, enabled: enabled($0.provider)) }
        let rest = providers.filter { seen.insert($0.id).inserted }
            .map { ConnectedServiceRow(provider: $0.id, name: name($0.id), status: .notConnected, enabled: $0.enabled) }
        let keyless = (list.keyless ?? []).filter { seen.insert($0.id).inserted }
            .map { ConnectedServiceRow(provider: $0.id, name: Self.name($0, id: $0.id, english: english), status: .keyless, enabled: $0.enabled) }
        return connected + rest + keyless
    }
}

/// OAuth の戻り先 `tailorit://connected?provider=…&status=ok|error`(runner の `/oauth/callback/{provider}`)。
enum ConnectCallback: Equatable {
    case ok(provider: String?)
    case error(provider: String?)
    case invalid

    static let scheme = "tailorit"

    static func parse(_ url: URL) -> ConnectCallback {
        guard url.scheme == scheme, url.host == "connected",
              let items = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems else { return .invalid }
        let provider = items.first { $0.name == "provider" }?.value
        switch items.first(where: { $0.name == "status" })?.value {
        case "ok": return .ok(provider: provider)
        case "error": return .error(provider: provider)
        default: return .invalid
        }
    }

    /// 頼んだ provider の `ok` だけを成功とみなす(読めない戻り先・別の provider は失敗)
    func succeeded(for provider: String) -> Bool {
        if case .ok(let p) = self { return p == provider }
        return false
    }
}

/// 「つないだサービス」の状態と操作。View から独立させてテストする。
/// 読めた一覧は `ExtConnectionListCache` に控え、読めない時(オフラインなど)は控えを `stale` 付きで出す(控えが無ければ再試行できる失敗)。
@MainActor
@Observable
final class ConnectedServicesModel {
    private(set) var state: ConnectedServicesState = .loading
    /// 直前の一覧に `providers` があったか(無いのは古いサーバー)。`.loaded` の時だけ意味がある
    private(set) var providersKnown = false
    /// 今の `.loaded` が控え(読めなかったので前の一覧を出している)。再試行のボタンと一言を出す
    private(set) var stale = false
    /// 操作中の provider(ボタンを止める)
    private(set) var busy: String?
    /// 直前の操作の失敗(セクションの下に出す)
    var message: String?

    private let cache: ExtConnectionListCache?

    /// `cache` が nil なら控えを使わない(テスト用)
    init(cache: ExtConnectionListCache? = .shared) {
        self.cache = cache
    }

    /// 同意の画面を開き、戻り先の URL を返す(View が `webAuthenticationSession` を渡す)
    typealias Authenticate = @MainActor (URL) async throws -> URL

    func load(_ client: any ExternalServicesClient) async {
        do {
            let list = try await client.connections(supportedFeatures: ConnectedServicesFeatures.current)
            providersKnown = list.providers != nil
            stale = false
            state = .from(.success(list))
            cache?.store(list)
        } catch is CancellationError {
            // 読み込み中のまま止まらないように。もう読めた一覧は上書きしない(画面に戻れば .task が読み直す)
            if state == .loading { fallBack() }
        } catch {
            if (error as? APIError)?.isExtUnavailable == true {
                state = .hidden
            } else {
                fallBack()
            }
        }
    }

    /// 読めなかった: 控えがあればそれを `stale` で出す。無ければ再試行できる失敗
    private func fallBack() {
        if let cached = cache?.load() {
            providersKnown = cached.providers != nil
            stale = true
            state = .from(.success(cached))
        } else {
            state = .failed(L10n.tr("一覧を読み込めませんでした。"))
        }
    }

    /// つなぐ / つなぎ直す。同意から戻ったら一覧を取り直す。
    func connect(_ provider: String, client: any ExternalServicesClient, authenticate: Authenticate) async {
        guard busy == nil else { return }
        busy = provider
        message = nil
        defer { busy = nil }
        do {
            let url = try await client.startConnection(provider: provider, scopes: nil)
            if !MockExternalServicesClient.isMockAuthURL(url) {
                let back = try await authenticate(url)
                if !ConnectCallback.parse(back).succeeded(for: provider) { message = L10n.tr("つなげませんでした。もう一度試してください。") }
            }
        } catch {
            if Self.isUserCancel(error) {
                // 自分で閉じた。何も言わない
            } else if (error as? APIError)?.isExtUnavailable == true {
                state = .hidden
                return
            } else {
                message = L10n.tr("つなげませんでした。もう一度試してください。")
            }
        }
        await load(client)
    }

    func disconnect(_ provider: String, client: any ExternalServicesClient) async {
        guard busy == nil else { return }
        busy = provider
        message = nil
        defer { busy = nil }
        do {
            try await client.disconnect(provider: provider)
        } catch {
            if (error as? APIError)?.isExtUnavailable == true {
                state = .hidden
                return
            }
            message = L10n.tr("切れませんでした。もう一度試してください。")
        }
        await load(client)
    }

    static func isUserCancel(_ error: Error) -> Bool {
        if error is CancellationError { return true }
        let ns = error as NSError
        return ns.domain == ASWebAuthenticationSessionErrorDomain && ns.code == ASWebAuthenticationSessionError.Code.canceledLogin.rawValue
    }
}
#endif
