// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import MetamonDSL

/// 近距離共有(#36)で待ち受けるかの判定。iOS(`AppRuntime+Nearby`)と Mac(`MacNearby`・#964)で共有する。
extension AppDSL {
    /// 近くの端末から受け取る部品があるか(送る方の `nearbyShare` か、リクエストを受ける `musicPlayer`)。
    var usesNearbyReceive: Bool { usesNearbyShare || usesMusicRequests }

    /// どこかの画面に送る方の `nearbyShare` があるか(container の子も見る)。passby(#425)は部品が表示中だけ自分で探す。
    var usesNearbyShare: Bool {
        let isSend: (Component) -> Bool = { $0.type == .nearbyShare && $0.mode != "passby" }
        return screens.contains { s in
            s.components.contains { isSend($0) || $0.children.contains(where: isSend) }
        }
    }
}
#endif
