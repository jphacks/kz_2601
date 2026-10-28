import Foundation

/// 共有 entity の「自分だけの印」1 つ(#255・schema `$defs/Mark`)。いいね・保存・既読のように 1 人 1 回のトグル。
/// 誰が付けたかは appconfig が持ち、レコードの `data` には入らない。
public struct Mark: Decodable, Sendable, Identifiable, Hashable {
    public var id: String
    /// heart / star / bookmark / thumbsup / check / eye / flag(未知の値は heart で描く)
    public var icon: String
    public var label: String

    public init(id: String, icon: String, label: String) {
        self.id = id
        self.icon = icon
        self.label = label
    }

    /// SF Symbol。付けた状態は塗りつぶし。
    public func systemImage(on: Bool) -> String {
        let base: String = switch icon {
        case "star": "star"
        case "bookmark": "bookmark"
        case "thumbsup": "hand.thumbsup"
        case "check": "checkmark.circle"
        case "eye": "eye"
        case "flag": "flag"
        default: "heart"
        }
        return on ? base + ".fill" : base
    }
}

/// 1 件のレコードの印の状態(appconfig `MarkState`: 件数と、自分が付けているか)。
public struct MarkState: Codable, Sendable, Hashable {
    public var count: Int
    public var mine: Bool

    public init(count: Int, mine: Bool) {
        self.count = count
        self.mine = mine
    }

    /// 押した直後の楽観的な状態(件数は 0 未満にしない)。
    public func toggled() -> MarkState {
        MarkState(count: max(0, count + (mine ? -1 : 1)), mine: !mine)
    }
}

public extension Entity {
    func mark(_ id: String) -> Mark? { marks.first { $0.id == id } }
}

public extension Record {
    /// 印の状態(サーバーが付けていなければ 0 件・未)。
    func markState(_ id: String) -> MarkState { marks?[id] ?? MarkState(count: 0, mine: false) }
}
