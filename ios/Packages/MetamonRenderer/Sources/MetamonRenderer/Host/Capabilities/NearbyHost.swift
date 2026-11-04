import MetamonDSL

/// 近距離共有(#36 / #412)。持たないホスト(プレビュー・テスト・審査用ビルド)ではボタンを押しても何もしない。
@MainActor
public protocol NearbyHost: AnyObject {
    /// `nearbyShare`: 近くの端末を探すシートを開き、選んだ相手に `records`(entity の保存フィールドのみ)を送る。
    func shareNearby(entity: String, records: [[String: JSONValue]])
    /// `musicPlayer`: 近くの端末を探すシートを開き、選んだ相手に曲のリクエストを送る。
    func requestSong(_ song: MusicSong)
}

public extension DSLRuntime {
    var nearby: (any NearbyHost)? { self as? any NearbyHost }
}
