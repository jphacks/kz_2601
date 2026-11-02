import MetamonDSL

/// 自分の印(いいね・保存 #255)。持たないホストでは付けられない(nil と同じ扱い)。
@MainActor
public protocol MarksHost: AnyObject {
    /// 印を付け外しする。確定した状態を返し、付けられなければ nil(ホストが理由を出す)。
    func toggleMark(entity: String, recordId: String, mark: String) async -> MarkState?
}

public extension DSLRuntime {
    var marks: (any MarksHost)? { self as? any MarksHost }
}
