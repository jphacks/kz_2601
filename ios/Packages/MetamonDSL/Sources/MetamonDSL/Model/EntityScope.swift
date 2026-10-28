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
    /// `Entity.scope`(#249)を読む。entity が無ければ private。
    func scope(of entityId: String) -> EntityScope { EntityScope(raw: entity(entityId)?.scope) }
}
