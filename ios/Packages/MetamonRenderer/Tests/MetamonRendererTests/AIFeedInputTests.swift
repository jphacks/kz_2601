#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// aiButton `inputFeeds`: 同じ画面の feed の行を「外部データ」として材料に足す(20 行・2 KB・節を優先して残す)。
@MainActor
final class AIFeedInputTests: XCTestCase {
    private let source = FeedSource(name: "気象庁ホームページ", url: "https://www.jma.go.jp/", license: "公共データ利用規約(第1.0版)")

    private func screen(_ json: String) throws -> Screen {
        try JSONDecoder().decode(Screen.self, from: Data(json.utf8))
    }

    func testFeedsOnSameScreenInOrder() throws {
        let s = try screen(#"""
        {"id":"home","title":"体調","toolbar":[],"components":[
         {"type":"feed","id":"wx","provider":"weather","region":"170000"},
         {"type":"vstack","children":[{"type":"feed","id":"hpa","provider":"pressure","region":"170000"},
          {"type":"text","id":"note","text":"メモ"}]},
         {"type":"aiButton","id":"b","task":"generate","prompt":"体調","inputFeeds":["hpa","wx","note","nope"]}]}
        """#)
        let button = try XCTUnwrap(s.allComponents.first { $0.id == "b" })
        XCTAssertEqual(button.inputFeeds, ["hpa", "wx", "note", "nope"])
        XCTAssertEqual(AIFeedInput.feeds(for: button, in: s).map(\.id), ["hpa", "wx"], "feed だけ・inputFeeds の順")
        XCTAssertTrue(AIFeedInput.feeds(for: button, in: nil).isEmpty)
    }

    func testLines() {
        let wx = FeedItem(date: "2026-11-07", title: "晴れ", subtitle: "晴れ 時々 くもり", max: 13.4, min: 8.6, pop: 10)
        XCTAssertEqual(AIFeedInput.line(wx, provider: "weather", english: false),
                       "- 11/7(土) / 晴れ / 晴れ 時々 くもり / 最高 13° / 最低 9° / 降水確率 10%")
        let hpa = FeedItem(date: "2026-11-09T06:00:00+09:00", title: "6時〜12時", max: 1013.4, min: 1006.2, drop: true)
        XCTAssertEqual(AIFeedInput.line(hpa, provider: "pressure", english: false),
                       "- 11/9(月) / 6時〜12時 / 1006〜1013 hPa / 気圧が大きく下がる")
        XCTAssertEqual(AIFeedInput.line(FeedItem(title: "お知らせ"), provider: "rss", english: true), "- お知らせ")
    }

    func testSectionSharesRowsAndBytes() throws {
        let many = (0..<30).map { FeedItem(date: "2026-11-07", title: "行\($0)") }
        let a = FeedResult(provider: "weather", title: "石川県の天気", items: many, source: source)
        let b = FeedResult(provider: "holidays", title: "国民の祝日", items: many, source: source)
        let s = AIFeedInput.section([("天気", a), ("", b)], english: false)
        XCTAssertTrue(s.hasPrefix("外部データ(アプリが表示している公開データ):\n■ 天気\n"))
        XCTAssertTrue(s.contains("\n■ 国民の祝日\n"), "部品の title が無ければ結果の title")
        XCTAssertEqual(s.split(separator: "\n").filter { $0.hasPrefix("- ") }.count, 20, "合わせて 20 行(1 つ 10 行)")

        let long = (0..<20).map { _ in FeedItem(title: String(repeating: "長", count: 100)) }
        let big = AIFeedInput.section([("", FeedResult(provider: "rss", title: "お知らせ", items: long, source: source))], english: false)
        XCTAssertLessThanOrEqual(big.utf8.count, AIFeedInput.maxBytes)
        XCTAssertTrue(big.hasSuffix(String(repeating: "長", count: 100)), "行の途中で切らない")

        let empty = AIFeedInput.section([("", FeedResult(provider: "warnings", title: "警報", items: [], source: source))], english: false)
        XCTAssertTrue(empty.hasSuffix("- (今は表示するものがありません)"), "取れて 0 行は「今は無い」(警報なし等)")
    }

    /// 取れなかった feed は 1 行で知らせて続ける。1 つも取れなければ「外部データ: なし」(AI がデータを作らないように)。
    func testSectionFailedAndNone() {
        let a = FeedResult(provider: "weather", title: "石川県の天気", items: [FeedItem(title: "晴れ")], source: source)
        let mixed = AIFeedInput.section([("天気", a), ("気圧", nil)], english: false)
        XCTAssertTrue(mixed.hasSuffix("■ 気圧\n- (取得できませんでした)"), mixed)
        XCTAssertTrue(mixed.contains("- 晴れ"))
        XCTAssertTrue(AIFeedInput.section([("天気", a), ("気圧", nil)], english: true).hasSuffix("- (could not be loaded)"))

        let none = "外部データ: なし(添えられるデータがありません。外部のデータを推測・作成しないでください)"
        XCTAssertEqual(AIFeedInput.section([], english: false), none)
        XCTAssertEqual(AIFeedInput.section([("気圧", nil)], english: false), none)
        XCTAssertTrue(AIFeedInput.section([], english: true).hasPrefix("External data: none"))
    }

    func testMergeKeepsSection() {
        XCTAssertEqual(AIFeedInput.merge(input: nil, section: "外部", maxBytes: 4096), "外部")
        XCTAssertEqual(AIFeedInput.merge(input: "記録", section: nil, maxBytes: 4096), "記録")
        XCTAssertEqual(AIFeedInput.merge(input: "記録", section: "外部", maxBytes: 4096), "記録\n\n外部")
        let merged = AIFeedInput.merge(input: String(repeating: "a", count: 5000), section: String(repeating: "b", count: 2000), maxBytes: 4096) ?? ""
        XCTAssertEqual(merged.utf8.count, 4096)
        XCTAssertTrue(merged.hasSuffix(String(repeating: "b", count: 2000)), "はみ出す分は元の材料を削る")
    }

    // MARK: - 読み込み

    final class FakeHost: FeedHost {
        var cached: [String: FeedResult] = [:]
        var delays: [String: Double] = [:]
        var failing: Set<String> = []
        var calls: [String] = []
        let source: FeedSource

        init(source: FeedSource) { self.source = source }

        func feed(_ query: FeedQuery, force: Bool) async throws -> FeedResult {
            calls.append(query.provider)
            try await Task.sleep(nanoseconds: UInt64((delays[query.provider] ?? 0) * 1_000_000_000))
            if failing.contains(query.provider) { throw URLError(.notConnectedToInternet) }
            return FeedResult(provider: query.provider, title: "取得 \(query.provider)", items: [FeedItem(title: "新しい行")], source: source)
        }

        func cachedFeed(_ query: FeedQuery) -> FeedResult? { cached[query.provider] }
    }

    private func feedScreen() throws -> (Screen, Component) {
        let s = try screen(#"""
        {"id":"home","title":"体調","toolbar":[],"components":[
         {"type":"feed","id":"wx","title":"天気","provider":"weather","region":"170000"},
         {"type":"feed","id":"hpa","provider":"pressure","region":"170000"},
         {"type":"feed","id":"hol","provider":"holidays"},
         {"type":"aiButton","id":"b","task":"generate","prompt":"体調","inputFeeds":["wx","hpa","hol"]}]}
        """#)
        return (s, try XCTUnwrap(s.allComponents.first { $0.id == "b" }))
    }

    /// 同時に取りに行く(1 つずつ待たない)。取れない feed は nil で、残りは使う。
    func testLoadConcurrentlyAndKeepsGoingOnFailure() async throws {
        let (s, button) = try feedScreen()
        let host = FakeHost(source: source)
        host.delays = ["weather": 0.4, "pressure": 0.4, "holidays": 0.4]
        host.failing = ["pressure"]
        let started = Date()
        let loaded = await AIFeedInput.load(AIFeedInput.feeds(for: button, in: s), host: host, seconds: 5)
        XCTAssertLessThan(Date().timeIntervalSince(started), 1.0, "3 つ × 0.4 秒を順に待たない")
        XCTAssertEqual(loaded.map(\.title), ["天気", "pressure", ""])
        XCTAssertEqual(loaded.map { $0.result?.title }, ["取得 weather", nil, "取得 holidays"])
    }

    /// 全体で seconds まで。間に合わない feed は「取得できませんでした」になり、取れた分で進む。
    func testLoadHasOverallBudget() async throws {
        let (s, button) = try feedScreen()
        let host = FakeHost(source: source)
        host.delays = ["weather": 0, "pressure": 30, "holidays": 30]
        let started = Date()
        var request = AITextRequest(instruction: "体調", shape: .line)
        try await AIFeedInput.attach(button, screen: s, host: host, locale: "ja", to: &request, seconds: 0.2)
        XCTAssertLessThan(Date().timeIntervalSince(started), 2, "遅い feed を待ち続けない")
        let input = try XCTUnwrap(request.input)
        XCTAssertTrue(input.contains("■ 天気\n- 新しい行"), input)
        XCTAssertTrue(input.contains("■ pressure\n- (取得できませんでした)"), input)
    }

    /// 覚えている行は古くてもそのまま使う(画面が出している行と同じ・取り直さない)。
    func testLoadPrefersCachedRows() async throws {
        let (s, button) = try feedScreen()
        let host = FakeHost(source: source)
        host.cached = ["weather": FeedResult(provider: "weather", title: "前の天気", items: [FeedItem(title: "画面の行")], source: source)]
        let loaded = await AIFeedInput.load(AIFeedInput.feeds(for: button, in: s), host: host, seconds: 5)
        XCTAssertEqual(loaded.first?.result?.items.first?.title, "画面の行")
        XCTAssertEqual(host.calls.sorted(), ["holidays", "pressure"], "覚えている feed は取りに行かない")
    }

    /// inputFeeds があるのに 1 行も添えられない(feed が消えた・ホストが無い)時は「外部データ: なし」を書く。
    func testAttachStatesNoneWhenNothingAttached() async throws {
        let (s, _) = try feedScreen()
        let gone = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"aiButton","id":"b2","task":"generate","prompt":"体調","inputFeeds":["removed"]}"#.utf8))
        var request = AITextRequest(instruction: "体調", shape: .line)
        request.input = "記録"
        try await AIFeedInput.attach(gone, screen: s, host: FakeHost(source: source), locale: "ja", to: &request)
        XCTAssertEqual(request.input, "記録\n\n外部データ: なし(添えられるデータがありません。外部のデータを推測・作成しないでください)")

        let (_, button) = try feedScreen()
        var noHost = AITextRequest(instruction: "体調", shape: .line)
        try await AIFeedInput.attach(button, screen: s, host: nil, locale: "en", to: &noHost)
        XCTAssertTrue(noHost.input?.hasPrefix("External data: none") == true)

        var plain = AITextRequest(instruction: "体調", shape: .line)
        let noFeeds = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"aiButton","id":"b3","task":"generate","prompt":"体調"}"#.utf8))
        try await AIFeedInput.attach(noFeeds, screen: s, host: nil, locale: "ja", to: &plain)
        XCTAssertNil(plain.input, "inputFeeds が無ければ何も足さない")
    }
}
#endif
