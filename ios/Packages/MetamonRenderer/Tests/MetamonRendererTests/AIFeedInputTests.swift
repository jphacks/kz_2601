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
        let s = try XCTUnwrap(AIFeedInput.section([("天気", a), ("", b)], english: false))
        XCTAssertTrue(s.hasPrefix("外部データ(アプリが表示している公開データ):\n■ 天気\n"))
        XCTAssertTrue(s.contains("\n■ 国民の祝日\n"), "部品の title が無ければ結果の title")
        XCTAssertEqual(s.split(separator: "\n").filter { $0.hasPrefix("- ") }.count, 20, "合わせて 20 行(1 つ 10 行)")

        let long = (0..<20).map { _ in FeedItem(title: String(repeating: "長", count: 100)) }
        let big = try XCTUnwrap(AIFeedInput.section([("", FeedResult(provider: "rss", title: "お知らせ", items: long, source: source))], english: false))
        XCTAssertLessThanOrEqual(big.utf8.count, AIFeedInput.maxBytes)
        XCTAssertTrue(big.hasSuffix(String(repeating: "長", count: 100)), "行の途中で切らない")

        let empty = try XCTUnwrap(AIFeedInput.section([("", FeedResult(provider: "warnings", title: "警報", items: [], source: source))], english: false))
        XCTAssertTrue(empty.hasSuffix("- (今は表示するものがありません)"))
        XCTAssertNil(AIFeedInput.section([], english: false))
    }

    func testMergeKeepsSection() {
        XCTAssertEqual(AIFeedInput.merge(input: nil, section: "外部", maxBytes: 4096), "外部")
        XCTAssertEqual(AIFeedInput.merge(input: "記録", section: nil, maxBytes: 4096), "記録")
        XCTAssertEqual(AIFeedInput.merge(input: "記録", section: "外部", maxBytes: 4096), "記録\n\n外部")
        let merged = AIFeedInput.merge(input: String(repeating: "a", count: 5000), section: String(repeating: "b", count: 2000), maxBytes: 4096) ?? ""
        XCTAssertEqual(merged.utf8.count, 4096)
        XCTAssertTrue(merged.hasSuffix(String(repeating: "b", count: 2000)), "はみ出す分は元の材料を削る")
    }

    func testWithinTimesOut() async {
        let savedLanguage = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja // 文言は日本語で確かめる
        defer { LanguagePreference.shared.setting = savedLanguage }
        do {
            _ = try await AIFeedInput.within(0.05) { () async throws -> Int in
                try await Task.sleep(nanoseconds: 5_000_000_000)
                return 1
            }
            XCTFail("should time out")
        } catch {
            XCTAssertEqual(error as? AIFeedInput.FeedInputError, .notLoaded)
            XCTAssertEqual(error.localizedDescription, "データを読み込んでから押してください")
        }
        let v = try? await AIFeedInput.within(5) { () async throws -> Int in 7 }
        XCTAssertEqual(v, 7)
        do {
            _ = try await AIFeedInput.within(5) { () async throws -> Int in throw URLError(.notConnectedToInternet) }
            XCTFail("should fail")
        } catch {
            XCTAssertEqual(error as? AIFeedInput.FeedInputError, .notLoaded, "取れなければ同じ案内")
        }
    }
}
#endif
