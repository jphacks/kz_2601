import XCTest
@testable import MetamonDSL

/// `dashcam`(#410)の純粋ロジック: 設定の丸め・区間のファイル名・ループで消す区間・衝撃・自動開始。
final class DashcamTests: XCTestCase {
    private let t0 = Date(timeIntervalSince1970: 1_800_000_000)
    private func at(_ s: TimeInterval) -> Date { t0.addingTimeInterval(s) }
    private func seg(_ start: TimeInterval, mb: Int64 = 50, saved: Bool = false) -> DashcamSegment {
        DashcamSegment(start: at(start), duration: 60, bytes: mb * 1_000_000, saved: saved)
    }

    // MARK: - config

    func testConfigDefaultsAndClamps() {
        let d = DashcamConfig()
        XCTAssertEqual(d.keepSeconds, 600)
        XCTAssertEqual(d.maxBytes, 2_000_000_000)
        XCTAssertEqual(d.impactG, 2.5)
        XCTAssertEqual(d.autoStart, [])
        XCTAssertFalse(d.audio)

        let c = DashcamConfig(keepMinutes: 999, maxStorageMB: 1, impactG: 0.1, autoStart: ["landscape", "moving", "charging"], audio: true)
        XCTAssertEqual(c.keepSeconds, 3600)
        XCTAssertEqual(c.maxBytes, 200_000_000)
        XCTAssertEqual(c.impactG, 1.2)
        XCTAssertEqual(c.autoStart, [.landscape, .charging], "未知の条件は無視")
        XCTAssertTrue(c.audio)
    }

    func testConfigFromDSL() throws {
        let json = #"{"type":"dashcam","keepMinutes":5,"maxStorageMB":1000,"impactG":3,"autoStart":["landscape","still"],"audio":true}"#
        let comp = try JSONDecoder().decode(Component.self, from: Data(json.utf8))
        XCTAssertEqual(comp.type, .dashcam)
        let c = DashcamConfig(comp)
        XCTAssertEqual(c.keepSeconds, 300)
        XCTAssertEqual(c.maxBytes, 1_000_000_000)
        XCTAssertEqual(c.impactG, 3)
        XCTAssertEqual(c.autoStart, [.landscape, .still])
        XCTAssertTrue(c.audio)
    }

    func testBundledExampleUsesDashcam() throws {
        let dsl = try ExamplesFixture.load("dashcam")
        let comp = try XCTUnwrap(dsl.screens.first?.components.first)
        XCTAssertEqual(comp.type, .dashcam)
        XCTAssertEqual(DashcamConfig(comp).autoStart, [.landscape, .still])
        XCTAssertTrue(ClientCapabilities.reviewExcludedComponents.contains(.dashcam))
        XCTAssertTrue(DSLExamples.reviewExcluded.contains("dashcam"))
    }

    // MARK: - segment names

    func testSegmentFileNameRoundTrip() throws {
        let s = DashcamSegment(start: at(0.25), duration: 59.5, bytes: 10, saved: true)
        XCTAssertEqual(s.fileName, "seg-1800000000250-59500.mov")
        let back = try XCTUnwrap(DashcamSegment(fileName: s.fileName, bytes: 10, saved: true))
        XCTAssertEqual(back, s)
        for bad in ["rec-1800000000250.mov", "seg-1-2.mp4", "seg-abc-1.mov", "seg-1.mov", "seg--1-2.mov", "../seg-1-2.mov"] {
            XCTAssertNil(DashcamSegment(fileName: bad), bad)
        }
    }

    // MARK: - loop

    /// 直近 10 分だけ残す: 終わりが 10 分より前の区間を古い順に消す。保存済みは消さない。
    func testLoopKeepsOnlyRecentMinutes() {
        // 0,60,...,720 秒に始まる 13 区間。now = 780(最後の区間の終わり)
        var segs = (0..<13).map { seg(TimeInterval($0) * 60) }
        segs[2].saved = true
        let drop = DashcamLoop.expired(segs, now: at(780), keepSeconds: 600, maxBytes: .max)
        // cutoff = 180: end < 180 は 0(60), 60(120) の 2 つ。120(180) は境界で残す。保存済み 120 も残る
        XCTAssertEqual(drop.map(\.start), [at(0), at(60)])
    }

    /// 容量: 合計が上限を超える分を古いループ区間から消す。
    func testLoopRespectsStorageCap() {
        let segs = [seg(0), seg(60, saved: true), seg(120), seg(180), seg(240)] // 50MB × 5 = 250MB
        let drop = DashcamLoop.expired(segs, now: at(300), keepSeconds: 3600, maxBytes: 160_000_000)
        XCTAssertEqual(drop.map(\.start), [at(0), at(120)], "保存済み(60)を飛ばして古い順に 2 つ消すと 150MB")
        XCTAssertTrue(drop.allSatisfy { !$0.saved })
    }

    func testFullWhenSavedAloneReachesCap() {
        let segs = [seg(0, saved: true), seg(60, saved: true), seg(120)]
        XCTAssertFalse(DashcamLoop.isFull(segs, maxBytes: 200_000_000, reserve: 50_000_000))
        XCTAssertTrue(DashcamLoop.isFull(segs, maxBytes: 140_000_000, reserve: 50_000_000))
        XCTAssertEqual(DashcamLoop.reserve(segs), 50_000_000)
        XCTAssertEqual(DashcamLoop.reserve([]), 60_000_000)
    }

    // MARK: - impact

    func testImpactThresholdAndCooldown() {
        var d = DashcamImpact(thresholdG: 2.5)
        XCTAssertFalse(d.feed(x: 1, y: 1, z: 1, at: at(10)), "√3 ≈ 1.7G は衝撃ではない")
        XCTAssertTrue(d.feed(x: 2, y: 1.5, z: 0, at: at(100)), "2.5G で衝撃")
        XCTAssertFalse(d.feed(x: 0, y: 0, z: -4, at: at(102)), "5 秒以内の揺れ戻しは数えない")
        XCTAssertTrue(d.feed(x: 0, y: 0, z: -4, at: at(106)))
        XCTAssertEqual(d.windows.count, 2)
        XCTAssertEqual(d.lastImpact, at(106))
    }

    /// 衝撃(100 秒)の前 60 秒〜後 30 秒にかかる区間を保存済みにする。
    func testImpactProtectsSurroundingSegments() {
        var d = DashcamImpact(thresholdG: 2.5)
        _ = d.feed(x: 3, y: 0, z: 0, at: at(100))
        let protected = [0.0, 60, 120, 180].filter { d.protects(start: at($0), end: at($0 + 60)) }
        XCTAssertEqual(protected, [0, 60, 120], "窓は 40〜130 秒: 0-60・60-120・120-180 にかかる")
        d.forget(before: at(200))
        XCTAssertTrue(d.windows.isEmpty)
    }

    func testManualMarkProtectsLikeImpact() {
        var d = DashcamImpact(thresholdG: 8)
        d.mark(at: at(500))
        XCTAssertTrue(d.protects(start: at(480), end: at(540)))
        XCTAssertFalse(d.protects(start: at(300), end: at(360)))
    }

    // MARK: - auto start

    private func sample(_ s: TimeInterval, landscape: Bool = true, still: Bool = true, charging: Bool = false) -> DashcamAutoStart.Sample {
        .init(gravity: landscape ? (x: -0.95, y: 0.1, z: -0.3) : (x: 0.05, y: -0.98, z: -0.1),
              acceleration: still ? 0.01 : 0.3, rotation: still ? 0.05 : 1.0, charging: charging, at: at(s))
    }

    func testAutoStartAfterHoldThenOnlyOnce() {
        var a = DashcamAutoStart(conditions: [.landscape, .still])
        XCTAssertFalse(a.feed(sample(0)))
        XCTAssertEqual(a.remaining(at: at(2)), 3)
        XCTAssertFalse(a.feed(sample(4.9)))
        XCTAssertTrue(a.feed(sample(5)), "5 秒続いたら開始")
        XCTAssertFalse(a.feed(sample(30)), "開始した後は条件が続いても 2 回目を出さない")
        XCTAssertNil(a.remaining(at: at(30)))
    }

    func testAutoStartResetsWhenConditionBreaks() {
        var a = DashcamAutoStart(conditions: [.landscape, .still])
        _ = a.feed(sample(0))
        XCTAssertFalse(a.feed(sample(3, still: false)), "持ち上げたら数え直し")
        XCTAssertFalse(a.feed(sample(4)))
        XCTAssertFalse(a.feed(sample(8.5)))
        XCTAssertTrue(a.feed(sample(9)))
    }

    /// 手で止めた後は、条件が一度外れて(持ち上げて)また置くまで始めない。
    func testManualStopDisarmsUntilConditionsBreak() {
        var a = DashcamAutoStart(conditions: [.landscape])
        a.disarm()
        XCTAssertFalse(a.feed(sample(0)))
        XCTAssertFalse(a.feed(sample(10)))
        XCTAssertFalse(a.feed(sample(11, landscape: false)))
        XCTAssertFalse(a.feed(sample(12)))
        XCTAssertTrue(a.feed(sample(17)))
    }

    func testChargingAndNoConditions() {
        var a = DashcamAutoStart(conditions: [.charging])
        XCTAssertFalse(a.feed(sample(0, landscape: false, still: false)))
        XCTAssertFalse(a.feed(sample(10, landscape: false, still: false)), "充電していなければ始めない")
        XCTAssertFalse(a.feed(sample(11, landscape: false, still: false, charging: true)))
        XCTAssertTrue(a.feed(sample(16, landscape: false, still: false, charging: true)))

        var manual = DashcamAutoStart(conditions: [])
        XCTAssertFalse(manual.feed(sample(0)))
        XCTAssertFalse(manual.feed(sample(100)), "条件なし = 手動だけ")
    }

    func testLandscapeFromGravity() {
        XCTAssertTrue(DashcamAutoStart.isLandscape((x: 0.9, y: 0.2, z: -0.3)))
        XCTAssertTrue(DashcamAutoStart.isLandscape((x: -0.6, y: 0.1, z: -0.8)), "スタンドで後ろに傾いていても横向き")
        XCTAssertFalse(DashcamAutoStart.isLandscape((x: 0.1, y: -0.95, z: -0.2)), "縦向き")
        XCTAssertFalse(DashcamAutoStart.isLandscape((x: 0.1, y: 0.1, z: -0.99)), "机に平置き")
    }

    func testStoreKeyIsStableAndPathSafe() {
        let k = DashcamStoreKey.make(appName: "置くだけ/ドラレコ ../")
        XCTAssertEqual(k, DashcamStoreKey.make(appName: "置くだけ/ドラレコ ../"))
        XCTAssertNotEqual(k, DashcamStoreKey.make(appName: "別のアプリ"))
        XCTAssertTrue(k.allSatisfy { $0.isHexDigit })
    }
}
