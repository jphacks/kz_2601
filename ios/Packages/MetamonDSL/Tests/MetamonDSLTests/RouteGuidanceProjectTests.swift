import XCTest
@testable import MetamonDSL

/// 道案内の線への投影(`RouteGuidance.project`)と進み具合(`progress`)の距離・沿った距離・次の案内の段。
final class RouteGuidanceProjectTests: XCTestCase {
    typealias G = RouteGuidance

    /// 東へ約 900 m(a→b)→ 北へ約 550 m(b→c)の L 字
    private let a = G.Coord(lat: 36.578, lng: 136.648)
    private let b = G.Coord(lat: 36.578, lng: 136.658)
    private let c = G.Coord(lat: 36.583, lng: 136.658)

    private var ab: Double { G.distanceMeters(a, b) }
    private var bc: Double { G.distanceMeters(b, c) }

    private var route: G.Route {
        G.Route(steps: [
            G.Step(instruction: "", points: [a]),
            G.Step(instruction: "東へ進む", points: [a, b]),
            G.Step(instruction: "左折して北へ", points: [b, c]),
            G.Step(instruction: "目的地は右側です", points: [c]),
        ], expectedTravelTime: 600)
    }

    /// a→b を f 進み、北へ dLat 度ずらした点
    private func onAB(_ f: Double, north dLat: Double = 0) -> G.Coord {
        G.Coord(lat: a.lat + dLat, lng: a.lng + (b.lng - a.lng) * f)
    }

    // MARK: - project

    func testProjectOnTheLine() {
        for f in [0.0, 0.25, 0.5, 0.75, 1.0] {
            let hit = G.project(onAB(f), onto: [a, b])
            XCTAssertEqual(hit.distance, 0, accuracy: 0.5, "f=\(f)")
            XCTAssertEqual(hit.along, ab * f, accuracy: 1, "f=\(f)")
        }
    }

    func testProjectOffTheLine() {
        let cases: [(name: String, f: Double, dLat: Double)] = [
            ("北へ約 55 m", 0.5, 0.0005),
            ("南へ約 55 m", 0.5, -0.0005),
            ("北へ約 11 m・始まり寄り", 0.1, 0.0001),
            ("南へ約 222 m・終わり寄り", 0.9, -0.002),
        ]
        for c in cases {
            let p = onAB(c.f, north: c.dLat)
            let hit = G.project(p, onto: [a, b])
            XCTAssertEqual(hit.distance, abs(c.dLat) * G.metersPerDegree, accuracy: 1, c.name)
            XCTAssertEqual(hit.along, ab * c.f, accuracy: 1, c.name)
        }
    }

    /// 端より先は端に止める(沿った距離は 0 か線の長さ・距離は端からの直線)。
    func testProjectBeyondTheEnds() {
        let west = G.Coord(lat: a.lat + 0.0003, lng: a.lng - 0.002)
        let w = G.project(west, onto: [a, b])
        XCTAssertEqual(w.along, 0, accuracy: 0.001)
        XCTAssertEqual(w.distance, G.distanceMeters(west, a), accuracy: 1)

        let east = G.Coord(lat: b.lat - 0.0003, lng: b.lng + 0.002)
        let e = G.project(east, onto: [a, b])
        XCTAssertEqual(e.along, ab, accuracy: 0.5)
        XCTAssertEqual(e.distance, G.distanceMeters(east, b), accuracy: 1)
    }

    /// 折れ線: 一番近い区間を選び、沿った距離は前の区間の長さを足す。角の外側は角に止める。
    func testProjectOntoPolyline() {
        let north = G.Coord(lat: b.lat + 0.002, lng: b.lng + 0.0001) // b→c の途中・東へ少し
        let n = G.project(north, onto: [a, b, c])
        XCTAssertEqual(n.along, ab + 0.002 * G.metersPerDegree, accuracy: 1)
        XCTAssertEqual(n.distance, G.distanceMeters(north, G.Coord(lat: north.lat, lng: b.lng)), accuracy: 1)

        let outside = G.Coord(lat: b.lat - 0.0005, lng: b.lng + 0.0005) // 角の南東
        let o = G.project(outside, onto: [a, b, c])
        XCTAssertEqual(o.along, ab, accuracy: 0.5)
        XCTAssertEqual(o.distance, G.distanceMeters(outside, b), accuracy: 1)

        let near = G.project(onAB(0.3, north: 0.0001), onto: [a, b, c])
        XCTAssertEqual(near.along, ab * 0.3, accuracy: 1, "近い方(最初の区間)で数える")
    }

    func testProjectDegenerateLines() {
        let empty = G.project(a, onto: [])
        XCTAssertEqual(empty.distance, .infinity)
        XCTAssertEqual(empty.along, 0)

        let single = G.project(b, onto: [a])
        XCTAssertEqual(single.distance, ab, accuracy: 0.001)
        XCTAssertEqual(single.along, 0)

        // 同じ点が続く区間(長さ 0)で割り算しない
        let dup = G.project(onAB(0.5, north: 0.0001), onto: [a, a, b, b])
        XCTAssertFalse(dup.distance.isNaN)
        XCTAssertFalse(dup.along.isNaN)
        XCTAssertEqual(dup.along, ab * 0.5, accuracy: 1)
        XCTAssertEqual(dup.distance, 0.0001 * G.metersPerDegree, accuracy: 1)
    }

    // MARK: - progress

    /// 線の外の位置: 経路からの距離は投影の距離、曲がる所までは step の長さ - 沿った距離。
    func testProgressDistancesMatchProjection() {
        let p = onAB(0.4, north: 0.0003) // 北へ約 33 m
        let pr = G.progress(route, at: p, from: 1, mode: .automobile)
        let hit = G.project(p, onto: [a, b])
        XCTAssertEqual(pr.stepIndex, 1)
        XCTAssertEqual(pr.distanceFromRoute, hit.distance, accuracy: 0.001)
        XCTAssertEqual(pr.distanceToManeuver, ab - hit.along, accuracy: 0.001)
        XCTAssertEqual(pr.remainingDistance, ab - hit.along + bc, accuracy: 0.001)
        XCTAssertEqual(pr.remainingTime, 600 * pr.remainingDistance / (ab + bc), accuracy: 0.01)
        XCTAssertFalse(pr.isOffRoute, "車は 50 m まで")
        XCTAssertTrue(G.progress(route, at: p, from: 1, mode: .walking).isOffRoute, "歩きは 30 m まで")
    }

    /// 次の案内の段: 走っている step の次の instruction。最後の線の step では目的地の案内。
    func testProgressNextInstructionStage() {
        let cases: [(name: String, at: G.Coord, from: Int, step: Int, next: String?)] = [
            ("出発(線の無い step は飛ばす)", a, 0, 1, "左折して北へ"),
            ("東の途中", onAB(0.6), 1, 1, "左折して北へ"),
            ("北の途中", G.Coord(lat: b.lat + 0.002, lng: b.lng), 1, 2, "目的地は右側です"),
        ]
        for c in cases {
            let pr = G.progress(route, at: c.at, from: c.from, mode: .automobile)
            XCTAssertEqual(pr.stepIndex, c.step, c.name)
            XCTAssertEqual(pr.nextInstruction, c.next, c.name)
        }
    }

    /// 空の instruction の線の無い step は飛ばし、空の instruction の線のある step で止める(次は目的地)。
    func testNextInstructionSkipsBlankSteps() {
        let steps = [
            G.Step(instruction: "東へ進む", points: [a, b]),
            G.Step(instruction: "  ", points: [b]),
            G.Step(instruction: " 左折して北へ ", points: [b, c]),
            G.Step(instruction: "", points: [c, a]),
            G.Step(instruction: "着いた", points: [a]),
        ]
        XCTAssertEqual(G.nextInstruction(after: 0, in: steps), "左折して北へ", "前後の空白は外す")
        XCTAssertNil(G.nextInstruction(after: 2, in: steps), "空の instruction の線は目的地まで続く扱い")
        XCTAssertEqual(G.nextInstruction(after: 3, in: steps), "着いた")
        XCTAssertNil(G.nextInstruction(after: 4, in: steps))
    }

    /// 目的地の先: 車は到着(残り 0・経路から 50 m 以内)、歩きは 30 m を超えるので道を外れた。
    func testProgressBeyondTheDestination() {
        let past = G.Coord(lat: c.lat + 0.0004, lng: c.lng) // 北へ約 44 m
        let car = G.progress(route, at: past, from: 2, mode: .automobile)
        XCTAssertEqual(car.stepIndex, 2)
        XCTAssertEqual(car.distanceToManeuver, 0, accuracy: 0.5, "端に止めるので負にならない")
        XCTAssertEqual(car.remainingDistance, 0, accuracy: 0.5)
        XCTAssertEqual(car.distanceFromRoute, G.distanceMeters(past, c), accuracy: 1)
        XCTAssertTrue(car.arrived)
        XCTAssertFalse(car.isOffRoute)

        let walk = G.progress(route, at: past, from: 2, mode: .walking)
        XCTAssertFalse(walk.arrived)
        XCTAssertTrue(walk.isOffRoute)
    }

    /// 線のある step が残っていない(最後の点だけの step から): 目的地までの直線で数える。
    func testProgressWithoutLineStepsLeft() {
        let p = G.Coord(lat: c.lat - 0.001, lng: c.lng) // 南へ約 111 m
        let pr = G.progress(route, at: p, from: 3, mode: .automobile)
        XCTAssertEqual(pr.stepIndex, 3)
        XCTAssertNil(pr.nextInstruction)
        XCTAssertEqual(pr.distanceToManeuver, G.distanceMeters(p, c), accuracy: 0.001)
        XCTAssertEqual(pr.remainingDistance, pr.distanceToManeuver, accuracy: 0.001)
        XCTAssertEqual(pr.remainingTime, 0)
        XCTAssertFalse(pr.arrived)
        XCTAssertTrue(pr.isOffRoute)
        XCTAssertTrue(G.progress(route, at: c, from: 3, mode: .walking).arrived)
    }
}
