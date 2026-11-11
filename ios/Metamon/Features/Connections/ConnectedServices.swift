import AuthenticationServices
import Foundation
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

#if !APP_STORE_REVIEW
/// 一覧の 1 行の状態。
enum ConnectedServiceStatus: Equatable {
    case connected
    /// provider がトークンを拒んだ(`status: error`)
    case needsReconnect
    case notConnected
}

struct ConnectedServiceRow: Equatable, Identifiable {
    var provider: String
    var status: ConnectedServiceStatus
    var id: String { provider }
    var name: String { ConnectedServicesCatalog.name(provider) }
}

/// 「つないだサービス」の表示の状態。
enum ConnectedServicesState: Equatable {
    case loading
    /// サーバーが外の API を止めている(503 `ext_unavailable`)。セクションごと出さない
    case hidden
    case failed(String)
    case loaded([ConnectedServiceRow])

    /// 一覧の結果から。ext_unavailable は隠す・ほかの失敗は再試行できる状態。
    static func from(_ result: Result<[ExtConnection], Error>,
                     candidates: [String] = ConnectedServicesCatalog.oauthProviders) -> ConnectedServicesState {
        switch result {
        case .success(let list):
            return .loaded(rows(list, candidates: candidates))
        case .failure(let error):
            if (error as? APIError)?.isExtUnavailable == true { return .hidden }
            return .failed(L10n.tr("一覧を読み込めませんでした。"))
        }
    }

    /// つないだもの(知っている順 → 知らないもの)の後に、まだつないでいない候補。
    static func rows(_ list: [ExtConnection], candidates: [String]) -> [ConnectedServiceRow] {
        var seen = Set<String>()
        let connected = list
            .filter { seen.insert($0.provider).inserted }
            .sorted { order($0.provider, candidates) < order($1.provider, candidates) }
            .map { ConnectedServiceRow(provider: $0.provider, status: $0.status == .active ? .connected : .needsReconnect) }
        let rest = candidates.filter { !seen.contains($0) }.map { ConnectedServiceRow(provider: $0, status: .notConnected) }
        return connected + rest
    }

    private static func order(_ p: String, _ candidates: [String]) -> Int { candidates.firstIndex(of: p) ?? candidates.count }
}

/// 端末が知っている OAuth の provider(backend `internal/extapi` の `NeedsOAuth`)。一覧の API はつないだものしか返さないので、
/// まだつないでいないものはここから出す。知らない provider がつながっていれば ID のまま出す。
enum ConnectedServicesCatalog {
    static var oauthProviders: [String] {
        #if DEBUG
        ["googleCalendar", "todoist", "fakeOauth"]
        #else
        ["googleCalendar", "todoist"]
        #endif
    }

    static func name(_ provider: String) -> String {
        switch provider {
        case "googleCalendar": L10n.tr("Google カレンダー")
        case "todoist": "Todoist"
        case "fakeOauth": L10n.tr("試験用のサービス")
        default: provider
        }
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
}

/// 「つないだサービス」の状態と操作。View から独立させてテストする。
@MainActor
@Observable
final class ConnectedServicesModel {
    private(set) var state: ConnectedServicesState = .loading
    /// 操作中の provider(ボタンを止める)
    private(set) var busy: String?
    /// 直前の操作の失敗(セクションの下に出す)
    var message: String?

    /// 同意の画面を開き、戻り先の URL を返す(View が `webAuthenticationSession` を渡す)
    typealias Authenticate = @MainActor (URL) async throws -> URL

    func load(_ client: any ExternalServicesClient) async {
        do {
            state = .from(.success(try await client.connections()))
        } catch is CancellationError {
        } catch {
            state = .from(.failure(error))
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
                if case .error = ConnectCallback.parse(back) { message = L10n.tr("つなげませんでした。もう一度試してください。") }
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
