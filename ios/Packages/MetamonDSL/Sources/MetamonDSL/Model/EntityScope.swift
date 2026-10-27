import Foundation

/// entity のレコードを誰が見られるか(DSL `entity.scope`・docs/dsl/README.md §3.5)。
/// 画面の「誰でも見られます」表示(#253)に使う。
public enum EntityScope: String, Sendable {
    case `private`, shared, `public`

    /// 省略・未知の値は private 扱い(backend と同じ)。
    public init(raw: String?) {
        self = raw.flatMap(EntityScope.init(rawValue:)) ?? .private
    }

    /// 自分以外も見られる(shared / public)。
    public var isVisibleToOthers: Bool { self != .private }
}

public extension AppDSL {
    func scope(of entityId: String) -> EntityScope { EntityScope(raw: entityScopes[entityId]) }
}

/// `entities[]` から id と scope だけを読む(Entity 本体のデコードとは独立。壊れた要素は飛ばす)。
struct EntityScopeProbe: Decodable {
    var id: String
    var scope: String?

    static func scopes(_ probes: [EntityScopeProbe]) -> [String: String] {
        var out: [String: String] = [:]
        for p in probes { if let s = p.scope { out[p.id] = s } }
        return out
    }
}
