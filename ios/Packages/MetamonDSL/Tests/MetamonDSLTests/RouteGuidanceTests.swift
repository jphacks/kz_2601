import XCTest
@testable import MetamonDSL

/// `routeGuide`(#411): 例のデコード・位置を流した時の step の進み方・残りの距離と時間・道を外れた判定・到着・読み上げ。
final class RouteGuidanceTests: XCTestCase {
    typealias G = RouteGuidance

    /// 金沢駅から東へ約 900 m → 北へ約 550 m の L 字(MapKit と同じく最初の step は線の無い出発地)。
    private let a = G.Coord(lat: 36.578, lng: 136.648)
    private let b = G.Coord(lat: 36.578, lng: 136.658)
    private let c = G.Coord(lat: 36.583, lng: 136.658)

    private var route: G.Route {
        G.Route(steps: [
            G.Step(instruction: "", points: [a]),
            G.Step(instruction: "東へ進む", points: [a, b]),
            G.Step(instruction: "左折して北へ", points: [b, c]),
            G.Step(instruction: "目的地は右側です", points: [c]),
        ], expectedTravelTime: 300)
    }

    /// a→b を f(0…1)進んだ点
    private func onAB(_ f: Double) -> G.Coord { G.Coord(lat: a.lat, lng: a.lng + (b.lng - a.lng) * f) }

    func testExampleDecodes() throws {
        let dsl = try ExamplesFixture.load("drive")
        let guides = dsl.screens.flatMap(\.allComponents).filter { $0.type == .routeGuide }
        XCTAssertEqual(guides.count, 2)
        XCTAssertEqual(guides[0].entity, "spot")
        XCTAssertEqual(guides[0].locationField, "place")
        XCTAssertEqual(guides[0].titleField, "name")
        XCTAssertEqual(G.Mode(dsl: guides[0].mode), .automobile)
        XCTAssertNil(guides[1].entity, "フォームの routeGuide は今のレコードへ案内")
        XCTAssertTrue(DSLExamples.names.contains("drive"))
    }

    func testDecodesAddressAndMode() throws {
        let json = #"{"type":"routeGuide","address":"金沢駅","mode":"walking","label":"出発"}"#
        let c = try JSONDecoder().decode(Component.self, from: Data(json.utf8))
        XCTAssertEqual(c.type, .routeGuide)
        XCTAssertEqual(c.address, "金沢駅")
        XCTAssertEqual(G.Mode(dsl: c.mode), .walking)
        XCTAssertEqual(G.Mode(dsl: "bicycle"), .automobile, "不明な mode は車")
        XCTAssertEqual(G.Mode(dsl: nil), .automobile)
    }

    func testPlacesFromRecords() {
        let records = [
            Record(id: "s1", data: ["name": "兼六園", "place": "36.562128,136.662652"]),
            Record(id: "s2", data: ["name": "場所なし"]),
            Record(id: "s3", data: ["name": "壊れた値", "place": "兼六園"]),
        ]
        let places = G.places(records, locationField: "place", titleField: "name")
        XCTAssertEqual(places.map(\.id), ["s1"])
        XCTAssertEqual(places[0].name, "兼六園")
        XCTAssertEqual(places[0].coord.lat, 36.562128, accuracy: 1e-9)
        XCTAssertNil(G.place(id: "draft", values: [:], locationField: "place", titleField: nil), "未入力のフォーム")
    }

    func testRouteLength() {
        XCTAssertEqual(route.distance, G.distanceMeters(a, b) + G.distanceMeters(b, c), accuracy: 0.001)
        XCTAssertEqual(route.distance, 1450, accuracy: 30)
        XCTAssertEqual(route.polyline, [a, b, c])
        XCTAssertEqual(route.destination, c)
    }

    func testStartSkipsEmptyFirstStep() {
        let p = G.progress(route, at: a, from: 0, mode: .automobile)
        XCTAssertEqual(p.stepIndex, 1)
        XCTAssertEqual(p.nextInstruction, "左折して北へ")
        XCTAssertEqual(p.distanceToManeuver, G.distanceMeters(a, b), accuracy: 1)
        XCTAssertEqual(p.remainingDistance, route.distance, accuracy: 1)
        XCTAssertEqual(p.remainingTime, 300, accuracy: 1)
        XCTAssertFalse(p.isOffRoute)
        XCTAssertFalse(p.arrived)
    }

    func testAdvancesAlongTheRoute() {
        var step = 0
        var lastRemaining = Double.infinity
        // 東へ 10% ずつ
        for f in stride(from: 0.1, through: 0.9, by: 0.1) {
            let p = G.progress(route, at: onAB(f), from: step, mode: .automobile)
            XCTAssertEqual(p.stepIndex, 1, "f=\(f)")
            XCTAssertLessThan(p.remainingDistance, lastRemaining)
            XCTAssertEqual(p.distanceToManeuver, G.distanceMeters(a, b) * (1 - f), accuracy: 2)
            lastRemaining = p.remainingDistance
            step = p.stepIndex
        }
        // 角を曲がって北へ 100 m ほど → step 2、次は目的地の案内
        let north = G.Coord(lat: b.lat + 0.0009, lng: b.lng)
        let p = G.progress(route, at: north, from: step, mode: .automobile)
        XCTAssertEqual(p.stepIndex, 2)
        XCTAssertEqual(p.nextInstruction, "目的地は右側です")
        XCTAssertEqual(p.remainingDistance, G.distanceMeters(north, c), accuracy: 2)
        XCTAssertEqual(p.remainingTime, 300 * p.remainingDistance / route.distance, accuracy: 0.5)
        // step は戻らない: 角の手前に戻る位置を渡しても step 2 の線上で数える
        let back = G.progress(route, at: onAB(0.95), from: 2, mode: .automobile)
        XCTAssertEqual(back.stepIndex, 2)
    }

    func testOverlapNearCornerStaysOnCurrentStep() {
        // 角の 5 m 手前: step 2 の始まりとも 5 m 程度 → 今の step のまま(先へ飛ばない)
        let p = G.progress(route, at: onAB(0.995), from: 1, mode: .automobile)
        XCTAssertEqual(p.stepIndex, 1)
        XCTAssertLessThan(p.distanceToManeuver, 10)
    }

    func testOffRouteUsesModeAndAccuracy() {
        // 経路から北へ約 70 m
        let off = G.Coord(lat: onAB(0.5).lat + 0.00063, lng: onAB(0.5).lng)
        let car = G.progress(route, at: off, accuracy: 5, from: 1, mode: .automobile)
        XCTAssertEqual(car.distanceFromRoute, 70, accuracy: 3)
        XCTAssertTrue(car.isOffRoute)
        // 精度が悪い(±40 m)時は 50 + 40 m まで許す
        XCTAssertFalse(G.progress(route, at: off, accuracy: 40, from: 1, mode: .automobile).isOffRoute)
        // 徒歩は 30 m で外れ
        let near = G.Coord(lat: onAB(0.5).lat + 0.00036, lng: onAB(0.5).lng) // 約 40 m
        XCTAssertTrue(G.progress(route, at: near, from: 1, mode: .walking).isOffRoute)
        XCTAssertFalse(G.progress(route, at: near, from: 1, mode: .automobile).isOffRoute)
    }

    func testArrival() {
        let close = G.Coord(lat: c.lat - 0.0002, lng: c.lng) // 約 22 m 手前
        XCTAssertTrue(G.progress(route, at: close, from: 2, mode: .automobile).arrived)
        XCTAssertFalse(G.progress(route, at: close, from: 2, mode: .walking).arrived)
        XCTAssertTrue(G.progress(route, at: c, from: 2, mode: .walking).arrived)
        XCTAssertFalse(G.progress(route, at: c, from: 2, mode: .walking).isOffRoute)
    }

    func testEmptyRouteIsArrived() {
        let p = G.progress(G.Route(steps: [], expectedTravelTime: 0), at: a, from: 0, mode: .walking)
        XCTAssertTrue(p.arrived)
    }

    func testReroutePolicy() {
        let now = Date()
        XCTAssertFalse(G.shouldReroute(offRouteFixes: 2, lastReroute: nil, now: now), "1〜2 回の飛びでは引き直さない")
        XCTAssertTrue(G.shouldReroute(offRouteFixes: 3, lastReroute: nil, now: now))
        XCTAssertFalse(G.shouldReroute(offRouteFixes: 5, lastReroute: now.addingTimeInterval(-5), now: now))
        XCTAssertTrue(G.shouldReroute(offRouteFixes: 5, lastReroute: now.addingTimeInterval(-20), now: now))
    }

    func testCuesWhileDriving() {
        var spoken = Set<String>()
        let ab = G.distanceMeters(a, b) // 約 893 m
        var cues: [G.Cue] = []
        // 出発 → 10 m ずつ角まで → 曲がる → 到着
        var step = 0
        for meters in stride(from: 0.0, through: ab, by: 10) {
            let p = G.progress(route, at: onAB(meters / ab), from: step, mode: .automobile)
            step = p.stepIndex
            if let cue = G.cue(for: p, spoken: &spoken, mode: .automobile) { cues.append(cue) }
        }
        let north = G.Coord(lat: b.lat + 0.0009, lng: b.lng)
        let turned = G.progress(route, at: north, from: step, mode: .automobile)
        if let cue = G.cue(for: turned, spoken: &spoken, mode: .automobile) { cues.append(cue) }
        let done = G.progress(route, at: c, from: turned.stepIndex, mode: .automobile)
        if let cue = G.cue(for: done, spoken: &spoken, mode: .automobile) { cues.append(cue) }
        XCTAssertNil(G.cue(for: done, spoken: &spoken, mode: .automobile), "到着は 1 回だけ")

        guard cues.count == 6 else { return XCTFail("cues = \(cues)") }
        guard case .next(let d0, "左折して北へ") = cues[0] else { return XCTFail("\(cues[0])") }
        XCTAssertEqual(d0, ab, accuracy: 1)
        guard case .approach(let d1, "左折して北へ") = cues[1] else { return XCTFail("\(cues[1])") }
        XCTAssertLessThanOrEqual(d1, 700)
        XCTAssertGreaterThan(d1, 680)
        guard case .approach(let d2, _) = cues[2] else { return XCTFail("\(cues[2])") }
        XCTAssertLessThanOrEqual(d2, 300)
        XCTAssertEqual(cues[3], .now(instruction: "左折して北へ"))
        guard case .next(_, "目的地は右側です") = cues[4] else { return XCTFail("\(cues[4])") }
        XCTAssertEqual(cues[5], .arrived)
    }

    func testCueFromMidStepDoesNotReplayFarMarks() {
        // 角の 250 m 手前で案内を始めた: 「250 m 先」の 1 回だけ。700 / 300 m は読み直さない
        var spoken = Set<String>()
        let ab = G.distanceMeters(a, b)
        let p = G.progress(route, at: onAB((ab - 250) / ab), from: 0, mode: .automobile)
        guard case .next(let d, _) = G.cue(for: p, spoken: &spoken, mode: .automobile) else { return XCTFail() }
        XCTAssertEqual(d, 250, accuracy: 2)
        let q = G.progress(route, at: onAB((ab - 240) / ab), from: p.stepIndex, mode: .automobile)
        XCTAssertNil(G.cue(for: q, spoken: &spoken, mode: .automobile))
    }

    func testRoundedDistance() {
        XCTAssertEqual(G.rounded(4), 10)
        XCTAssertEqual(G.rounded(87), 90)
        XCTAssertEqual(G.rounded(324), 300)
        XCTAssertEqual(G.rounded(330), 350)
        XCTAssertEqual(G.rounded(1_234), 1_200)
        XCTAssertEqual(G.formatDistance(324), "300 m")
        XCTAssertEqual(G.formatDistance(1_250), "1.3 km")
    }

    func testMockRouteAndDrive() {
        let r = G.mockRoute(from: a, to: c, mode: .walking)
        XCTAssertEqual(r.destination, c)
        XCTAssertEqual(r.expectedTravelTime, r.distance / (4_500.0 / 3600), accuracy: 0.01)
        let pts = G.mockDrive(r, every: 100)
        XCTAssertEqual(pts.first, a)
        XCTAssertEqual(pts.last, c)
        XCTAssertEqual(Double(pts.count - 1), (r.distance / 100).rounded(.up), accuracy: 1)
        for (p, q) in zip(pts, pts.dropFirst()) {
            XCTAssertLessThanOrEqual(G.distanceMeters(p, q), 101)
        }
        // 流した位置で最後まで進んで到着する
        var step = 0
        var arrived = false
        for p in pts {
            let pr = G.progress(r, at: p, from: step, mode: .walking)
            XCTAssertFalse(pr.isOffRoute)
            step = pr.stepIndex
            arrived = pr.arrived
        }
        XCTAssertTrue(arrived)
    }
}
