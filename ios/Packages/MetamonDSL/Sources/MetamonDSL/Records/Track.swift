import Foundation

/// 位置の記録(`locationTracker`・#168)の点列と集計。純粋ロジックだけ(CoreLocation に依存しない)。
/// 1 点 = 1 レコード(location field "lat,lng" + 日時 field)。時刻順に並べ、`sessionGap` 以上空いた所は別の記録として繋がない
/// (朝と夕方の散歩を 1 本の線にしない・距離にも足さない)。
public enum Track {
    public struct Point: Sendable, Equatable {
        public var lat: Double
        public var lng: Double
        public var date: Date

        public init(lat: Double, lng: Double, date: Date) {
            self.lat = lat
            self.lng = lng
            self.date = date
        }
    }

    /// この秒数以上空いたら別の記録(10 分)。
    public static let sessionGap: TimeInterval = 600

    /// stat の aggregate のうち軌跡を集計するもの。distance = 距離(m)、duration = 記録していた時間(秒)。
    public static func isTrackAggregate(_ op: String?) -> Bool { op == "distance" || op == "duration" }

    /// "lat,lng" → 緯度経度(範囲外・壊れた値は nil)。
    public static func coordinate(_ v: JSONValue) -> (lat: Double, lng: Double)? {
        guard let s = v.stringValue else { return nil }
        let parts = s.split(separator: ",").compactMap { Double($0.trimmingCharacters(in: .whitespaces)) }
        guard parts.count == 2, (-90...90).contains(parts[0]), (-180...180).contains(parts[1]) else { return nil }
        return (parts[0], parts[1])
    }

    public static func encode(lat: Double, lng: Double) -> String { String(format: "%.6f,%.6f", lat, lng) }

    /// レコード → 時刻順の点列。`by`(日時 field)が無い・読めない時は createdAt。どちらも無い点は捨てる。
    public static func points(_ records: [Record], locationField: String, by: String?) -> [Point] {
        records.compactMap { r -> Point? in
            guard let c = coordinate(r[locationField]) else { return nil }
            let d = by.flatMap { FieldCoding.date(from: r[$0]) } ?? r.createdAt.flatMap { FieldCoding.date(from: .string($0)) }
            guard let d else { return nil }
            return Point(lat: c.lat, lng: c.lng, date: d)
        }
        .sorted { $0.date < $1.date }
    }

    /// 時刻順の点列を `gap` 以上空いた所で切る。
    public static func segments(_ points: [Point], gap: TimeInterval = sessionGap) -> [[Point]] {
        var out: [[Point]] = []
        for p in points {
            if let last = out.last?.last, p.date.timeIntervalSince(last.date) < gap {
                out[out.count - 1].append(p)
            } else {
                out.append([p])
            }
        }
        return out
    }

    /// 2 点間の距離(m・haversine)。
    public static func distance(_ a: Point, _ b: Point) -> Double {
        let r = 6_371_000.0
        let dLat = (b.lat - a.lat) * .pi / 180
        let dLng = (b.lng - a.lng) * .pi / 180
        let h = sin(dLat / 2) * sin(dLat / 2) + cos(a.lat * .pi / 180) * cos(b.lat * .pi / 180) * sin(dLng / 2) * sin(dLng / 2)
        return 2 * r * asin(min(1, sqrt(h)))
    }

    public static func totalDistance(_ segments: [[Point]]) -> Double {
        segments.reduce(0) { sum, seg in
            sum + zip(seg, seg.dropFirst()).reduce(0) { $0 + distance($1.0, $1.1) }
        }
    }

    public static func totalDuration(_ segments: [[Point]]) -> TimeInterval {
        segments.reduce(0) { sum, seg in
            guard let first = seg.first, let last = seg.last else { return sum }
            return sum + last.date.timeIntervalSince(first.date)
        }
    }

    /// stat の数値(distance = m / duration = 秒)。
    public static func aggregate(_ op: String?, records: [Record], locationField: String?, by: String?) -> Double {
        let segs = segments(points(records, locationField: locationField ?? "", by: by))
        return op == "duration" ? totalDuration(segs) : totalDistance(segs)
    }

    /// 850 m / 1.23 km
    public static func formatDistance(_ meters: Double) -> String {
        meters < 1000 ? String(format: "%.0f m", meters) : String(format: "%.2f km", meters / 1000)
    }

    /// 1 時間未満は 分:秒(0:19 / 12:34)、以上は 時:分:秒(1:05:00)。言語に依存しない。
    public static func formatDuration(_ seconds: TimeInterval) -> String {
        let s = Int(max(0, seconds))
        return s >= 3600 ? String(format: "%d:%02d:%02d", s / 3600, s / 60 % 60, s % 60) : String(format: "%d:%02d", s / 60, s % 60)
    }

    public static func format(_ op: String?, _ value: Double) -> String {
        op == "duration" ? formatDuration(value) : formatDistance(value)
    }

    // MARK: - 記録中の間引き

    /// 記録中に届いた位置を保存するか: 精度が悪い点(> 50 m)は捨て、前に保存した点から `minDistance` 以上動いた時か
    /// `maxInterval` 以上経った時だけ保存する(止まっている間に点を溜めない・電池とデータ量の節約)。
    public static func shouldKeep(_ p: Point, accuracy: Double, last: Point?, minDistance: Double = 25, maxInterval: TimeInterval = 300) -> Bool {
        guard accuracy >= 0, accuracy <= 50 else { return false }
        guard let last else { return true }
        return distance(last, p) >= minDistance || p.date.timeIntervalSince(last.date) >= maxInterval
    }

    /// 自動停止までの秒数: 省略 1 時間、5 分〜4 時間に丸める(付け忘れで一晩中 GPS を使わない)。
    public static func autoStopSeconds(_ seconds: Int?) -> Int {
        min(max(seconds ?? 3600, 300), 14_400)
    }

    /// モック(METAMON_MOCK=1・シミュレータ)で流す決まった点列: 東京駅の北西を約 1.2 km 歩く 13 点。
    public static let mockRoute: [(lat: Double, lng: Double)] = [
        (35.681236, 139.767125), (35.682100, 139.766300), (35.683000, 139.765600), (35.683900, 139.765000),
        (35.684700, 139.764200), (35.685300, 139.763100), (35.685600, 139.761800), (35.685200, 139.760600),
        (35.684400, 139.759900), (35.683500, 139.760300), (35.682700, 139.761200), (35.682000, 139.762400),
        (35.681500, 139.763700),
    ]
}
