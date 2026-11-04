import MetamonDSL

/// `timerDisplay.liveActivity`(#423)。持たないホストでは何もしない。
@MainActor
public protocol LiveActivityHost: AnyObject {
    /// タイマーの開始・一時停止・リセット・終了をロック画面 / Dynamic Island に映す。
    func timerActivity(_ update: TimerActivityUpdate)
}

public extension DSLRuntime {
    var liveActivity: (any LiveActivityHost)? { self as? any LiveActivityHost }
}
