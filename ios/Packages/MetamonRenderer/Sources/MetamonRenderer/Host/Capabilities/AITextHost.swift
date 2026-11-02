import MetamonDSL

/// aiButton のオンライン版(#417)と端末内 AI の集計(#446)。持たないホストは端末内 AI だけ・数えない。
@MainActor
public protocol AITextHost: AnyObject {
    /// オンライン版。端末内 AI が使えない時だけ呼ぶ。同意・Pro の確認(画面を出す)もホストが持つ。
    func generateAIText(_ request: AITextRequest) async throws -> AITextResult
    /// オンライン版を持つか(審査用ビルドは false)。
    var supportsOnlineAIText: Bool { get }
    /// 端末内 AI の集計。決まったラベルだけ渡す。送るか・いつ送るかはホストが決める。
    func recordAIButton(_ event: AIButtonMetric.Event)
}

public extension DSLRuntime {
    var aiText: (any AITextHost)? { self as? any AITextHost }
}
