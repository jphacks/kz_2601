import Foundation

/// 道案内(`routeGuide`・#411)の進み方。純粋ロジックだけ(MapKit / CoreLocation に依存しない・位置を渡してテストする)。
///
/// 経路は MapKit の `MKRoute.steps` をそのまま写したもの: step k の `instruction` は step k の**始まり**でする操作
/// (「右折して〇〇通りに入る」)、`points` はその後に走る線。step i を走っている間の「次に曲がる所」は step i+1 の
/// instruction で、場所は step i の線の終わり。最後の step の終わりが目的地。
public enum RouteGuidance {
    public struct Coord: Sendable, Equatable {
        public var lat: Double
        public var lng: Double

        public init(lat: Double, lng: Double) {
            self.lat = lat
            self.lng = lng
        }
    }

    public enum Mode: String, Sendable, CaseIterable {
        case automobile, walking

        /// DSL の `mode`(省略・不明は車)。
        public init(dsl: String?) { self = Mode(rawValue: dsl ?? "") ?? .automobile }

        /// 経路からこれ以上離れたら道を外れた(m・位置の精度は別に足す)
        var offRouteMeters: Double { self == .automobile ? 50 : 30 }
        /// 目的地にこれより近づいたら到着(m)
        var arrivalMeters: Double { self == .automobile ? 40 : 20 }
        /// 曲がる所の手前で読み上げる距離(m・遠い順)。最後の 1 つは「まもなく」。
        public var cueMeters: [Double] { self == .automobile ? [700, 300, 80] : [100, 25] }
    }

    public struct Step: Sendable, Equatable {
        public var instruction: String
        public var points: [Coord]

        public init(instruction: String, points: [Coord]) {
            self.instruction = instruction
            self.points = points
        }

        public var length: Double { RouteGuidance.length(points) }
    }

    public struct Route: Sendable, Equatable {
        public var steps: [Step]
        /// MapKit の予想所要時間(秒)。残りの時間はこれを残りの距離の割合で按分する。
        public var expectedTravelTime: TimeInterval

        public init(steps: [Step], expectedTravelTime: TimeInterval) {
            self.steps = steps
            self.expectedTravelTime = expectedTravelTime
        }

        public var distance: Double { steps.reduce(0) { $0 + $1.length } }
        public var destination: Coord? { steps.last(where: { !$0.points.isEmpty })?.points.last }
        /// 地図に引く線(全 step をつないだもの)
        public var polyline: [Coord] {
            var out: [Coord] = []
            for p in steps.flatMap(\.points) where out.last != p { out.append(p) }
            return out
        }
    }

    public struct Progress: Sendable, Equatable {
        /// 今走っている step
        public var stepIndex: Int
        /// 次に曲がる所(nil = 次は目的地)
        public var nextInstruction: String?
        /// 次に曲がる所(または目的地)までの距離(m)
        public var distanceToManeuver: Double
        /// 目的地までの残りの距離(m・経路に沿って)
        public var remainingDistance: Double
        /// 目的地までの残りの時間(秒)
        public var remainingTime: TimeInterval
        /// 経路からの距離(m)
        public var distanceFromRoute: Double
        public var isOffRoute: Bool
        public var arrived: Bool
    }

    // MARK: - 目的地

    /// 目的地の候補 1 件(レコードの location field)。
    public struct Place: Sendable, Equatable, Identifiable {
        public var id: String
        public var name: String
        public var coord: Coord

        public init(id: String, name: String, coord: Coord) {
            self.id = id
            self.name = name
            self.coord = coord
        }
    }

    /// フォーム・詳細画面の今のレコード(locationPicker で選んだ値を含む)の場所。値が無い・壊れていれば nil。
    public static func place(id: String, values: [String: JSONValue], locationField: String, titleField: String?) -> Place? {
        guard let c = Track.coordinate(values[locationField] ?? .null) else { return nil }
        let name = titleField.flatMap { values[$0]?.stringValue }?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return Place(id: id, name: name, coord: Coord(lat: c.lat, lng: c.lng))
    }

    /// entity のレコードのうち場所を持つものを候補にする(並びはレコードの順)。
    public static func places(_ records: [Record], locationField: String, titleField: String?) -> [Place] {
        records.compactMap { place(id: $0.id, values: $0.data, locationField: locationField, titleField: titleField) }
    }

    // MARK: - 進み方

    /// 現在地 `at` での進み具合。step は戻らない(`from` 以降だけを見る)。
    /// 一番近い step を選ぶが、今の step との差が 10 m 以内なら今の step のまま(重なった道で先へ飛ばない)。
    public static func progress(_ route: Route, at p: Coord, accuracy: Double = 0, from stepIndex: Int, mode: Mode) -> Progress {
        let steps = route.steps
        guard !steps.isEmpty, let dest = route.destination else {
            return Progress(stepIndex: 0, nextInstruction: nil, distanceToManeuver: 0, remainingDistance: 0, remainingTime: 0,
                            distanceFromRoute: 0, isOffRoute: false, arrived: true)
        }
        let tolerance = mode.offRouteMeters + min(max(accuracy, 0), 50)
        let toDest = distanceMeters(p, dest)
        // 線の無い step(MapKit の最初の「出発地」など)は走る対象にしない
        let candidates = (max(stepIndex, 0)..<steps.count).filter { steps[$0].points.count >= 2 }
        guard let current = candidates.first else {
            let arrived = toDest <= mode.arrivalMeters
            return Progress(stepIndex: steps.count - 1, nextInstruction: nil, distanceToManeuver: toDest, remainingDistance: toDest,
                            remainingTime: 0, distanceFromRoute: toDest, isOffRoute: !arrived && toDest > tolerance, arrived: arrived)
        }
        let projections = candidates.map { (index: $0, hit: project(p, onto: steps[$0].points)) }
        let nearest = projections.min { $0.hit.distance < $1.hit.distance }!
        let atCurrent = projections[0].hit
        let (index, hit) = nearest.index != current && atCurrent.distance - nearest.hit.distance <= 10
            ? (current, atCurrent) : (nearest.index, nearest.hit)
        let distance = hit.distance
        let onStep = max(0, steps[index].length - hit.along)
        let later = steps.dropFirst(index + 1).reduce(0) { $0 + $1.length }
        let remaining = onStep + later
        let total = route.distance
        let time = total > 0 ? route.expectedTravelTime * remaining / total : 0
        let arrived = toDest <= mode.arrivalMeters || (remaining <= mode.arrivalMeters && distance <= tolerance)
        return Progress(stepIndex: index, nextInstruction: nextInstruction(after: index, in: steps),
                        distanceToManeuver: onStep, remainingDistance: remaining, remainingTime: time,
                        distanceFromRoute: distance, isOffRoute: !arrived && distance > tolerance, arrived: arrived)
    }

    /// step i の次の操作。後ろに線のある step が無く、最後の instruction も空なら nil(次は目的地)。
    static func nextInstruction(after i: Int, in steps: [Step]) -> String? {
        for s in steps.dropFirst(i + 1) {
            let t = s.instruction.trimmingCharacters(in: .whitespacesAndNewlines)
            if !t.isEmpty { return t }
            if s.points.count >= 2 { return nil }
        }
        return nil
    }

    // MARK: - 経路の引き直し

    /// 道を外れた位置がこの回数続いたら引き直す(GPS の 1 回の飛びで引き直さない)。
    public static let offRouteFixesToReroute = 3
    /// 引き直しの最短間隔(秒)。
    public static let rerouteInterval: TimeInterval = 15

    public static func shouldReroute(offRouteFixes: Int, lastReroute: Date?, now: Date) -> Bool {
        guard offRouteFixes >= offRouteFixesToReroute else { return false }
        guard let lastReroute else { return true }
        return now.timeIntervalSince(lastReroute) >= rerouteInterval
    }

    // MARK: - 読み上げ

    public enum Cue: Sendable, Equatable {
        /// 案内の開始・step が変わった: 次の操作までの距離と操作(nil = 目的地)
        case next(distance: Double, instruction: String?)
        /// 曲がる所に近づいた(`cueMeters` を下回った)
        case approach(distance: Double, instruction: String?)
        /// 曲がる所の直前(最後の `cueMeters`)
        case now(instruction: String?)
        case arrived
        case rerouting
    }

    /// 今の進み具合で読み上げる 1 つ(無ければ nil)。読んだ印は `spoken`(step ごとのキー)に足す。
    /// - step が変わったら(初回も)一度だけ「n m 先、〜」。ただし最後の距離より近ければ「まもなく」。
    /// - `cueMeters` の各距離を下回った時に一度だけ。複数を一度に下回ったら一番近いものだけ。
    public static func cue(for p: Progress, spoken: inout Set<String>, mode: Mode) -> Cue? {
        if p.arrived {
            return spoken.insert("arrived").inserted ? .arrived : nil
        }
        let marks = mode.cueMeters
        let first = spoken.insert("step\(p.stepIndex)").inserted
        // 今の距離より遠い印は「通過済み」にする(step の途中から始まっても遠い順に読み直さない)
        let crossed = marks.filter { p.distanceToManeuver <= $0 }
        var newlyCrossed: [Double] = []
        for m in crossed where spoken.insert("step\(p.stepIndex)@\(Int(m))").inserted { newlyCrossed.append(m) }
        let last = marks.last ?? 0
        if p.distanceToManeuver <= last, newlyCrossed.contains(last) || first {
            return .now(instruction: p.nextInstruction)
        }
        if first { return .next(distance: p.distanceToManeuver, instruction: p.nextInstruction) }
        if !newlyCrossed.isEmpty { return .approach(distance: p.distanceToManeuver, instruction: p.nextInstruction) }
        return nil
    }

    /// 読み上げ・表示用に丸めた距離(m): 100 m 未満は 10 m、1 km 未満は 50 m、それ以上は 100 m 単位。
    public static func rounded(_ meters: Double) -> Double {
        let m = max(0, meters)
        if m < 100 { return max(10, (m / 10).rounded() * 10) }
        if m < 1000 { return (m / 50).rounded() * 50 }
        return (m / 100).rounded() * 100
    }

    /// 「350 m」「1.2 km」(言語に依存しない)。
    public static func formatDistance(_ meters: Double) -> String {
        let r = rounded(meters)
        return r < 1000 ? String(format: "%.0f m", r) : String(format: "%.1f km", r / 1000)
    }

    // MARK: - 幾何(数 km の範囲なので正距円筒の平面近似で十分)

    static let metersPerDegree = 111_195.0

    public static func distanceMeters(_ a: Coord, _ b: Coord) -> Double {
        let r = 6_371_000.0
        let dLat = (b.lat - a.lat) * .pi / 180
        let dLng = (b.lng - a.lng) * .pi / 180
        let h = sin(dLat / 2) * sin(dLat / 2) + cos(a.lat * .pi / 180) * cos(b.lat * .pi / 180) * sin(dLng / 2) * sin(dLng / 2)
        return 2 * r * asin(min(1, sqrt(h)))
    }

    static func length(_ points: [Coord]) -> Double {
        zip(points, points.dropFirst()).reduce(0) { $0 + distanceMeters($1.0, $1.1) }
    }

    /// 点 p から線への最短距離(m)と、線の始まりからその足までの道のり(m)。
    static func project(_ p: Coord, onto points: [Coord]) -> (distance: Double, along: Double) {
        guard let first = points.first else { return (.infinity, 0) }
        guard points.count >= 2 else { return (distanceMeters(p, first), 0) }
        let kx = cos(p.lat * .pi / 180) * metersPerDegree
        let ky = metersPerDegree
        var best = (distance: Double.infinity, along: 0.0)
        var walked = 0.0
        for (a, b) in zip(points, points.dropFirst()) {
            let ax = (a.lng - p.lng) * kx, ay = (a.lat - p.lat) * ky
            let bx = (b.lng - p.lng) * kx, by = (b.lat - p.lat) * ky
            let dx = bx - ax, dy = by - ay
            let len2 = dx * dx + dy * dy
            let t = len2 > 0 ? min(1, max(0, -(ax * dx + ay * dy) / len2)) : 0
            let qx = ax + t * dx, qy = ay + t * dy
            let d = (qx * qx + qy * qy).squareRoot()
            let seg = distanceMeters(a, b)
            if d < best.distance { best = (d, walked + t * seg) }
            walked += seg
        }
        return best
    }

    // MARK: - モック

    /// モック(METAMON_MOCK=1)の経路: `from` から東西に進んで右(左)折し、南北に進んで `to`。所要時間は車 30 km/h・徒歩 4.5 km/h。
    public static func mockRoute(from: Coord, to: Coord, mode: Mode) -> Route {
        let corner = Coord(lat: from.lat, lng: to.lng)
        let steps = [
            Step(instruction: "", points: [from]),
            Step(instruction: "東西の通りを進む", points: [from, corner]),
            Step(instruction: "曲がって南北の通りに入る", points: [corner, to]),
            Step(instruction: "目的地に到着", points: [to]),
        ]
        let speed = mode == .automobile ? 30_000.0 / 3600 : 4_500.0 / 3600
        let route = Route(steps: steps, expectedTravelTime: 0)
        return Route(steps: steps, expectedTravelTime: route.distance / speed)
    }

    /// モックの現在地: 経路の線を `step` m ずつ進んだ点列(最後は目的地)。
    public static func mockDrive(_ route: Route, every step: Double) -> [Coord] {
        let line = route.polyline
        guard line.count >= 2, step > 0 else { return line }
        var out: [Coord] = [line[0]]
        var carry = 0.0
        for (a, b) in zip(line, line.dropFirst()) {
            let seg = distanceMeters(a, b)
            var t = step - carry
            while t < seg {
                let f = t / seg
                out.append(Coord(lat: a.lat + (b.lat - a.lat) * f, lng: a.lng + (b.lng - a.lng) * f))
                t += step
            }
            carry = seg - (t - step)
        }
        if out.last != line.last { out.append(line[line.count - 1]) }
        return out
    }
}
