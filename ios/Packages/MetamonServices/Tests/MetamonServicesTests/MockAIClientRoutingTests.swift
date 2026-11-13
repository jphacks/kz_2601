import XCTest
@testable import MetamonServices

/// 作り物の AI の会話の分かれ道。UI テストがこの言い回しに頼っているので、変えた時に気づけるよう表にしておく。
final class MockAIClientRoutingTests: XCTestCase {
    private func check(_ f: (String) -> Bool, _ rows: [(String, Bool)], _ label: String,
                       file: StaticString = #filePath, line: UInt = #line) {
        for (text, expected) in rows {
            XCTAssertEqual(f(text), expected, "\(label): \(text)", file: file, line: line)
        }
    }

    func testIsQuestion() {
        check(MockAIClient.isQuestion, [
            ("これって何?", true),
            ("これって何？", true),
            ("  できますか?  \n", true),
            ("WebView ですか", true),
            ("画像もできる", true),
            ("可能", true),
            ("どうなってる", true),
            ("家計簿アプリを作って", false),
            ("金額を大きくして", false),
            ("", false),
            ("?の前の文字 abc", false),
        ], "isQuestion")
    }

    func testIsGoAhead() {
        check(MockAIClient.isGoAhead, [
            ("作って", true),
            ("お任せ", true),
            ("おまかせ", true),
            ("それでいい", true),
            ("よろしく", true),
            ("  作って  ", true),
            ("このまま作って", true),
            ("家計簿アプリを作ってください、お願いします", false),
            ("写真も残す", false),
            ("", false),
        ], "isGoAhead")
    }

    func testIsGoAheadLengthBoundary() {
        let fourteen = "作って" + String(repeating: "あ", count: 11)
        let fifteen = fourteen + "あ"
        XCTAssertEqual(fourteen.count, 14)
        XCTAssertTrue(MockAIClient.isGoAhead(fourteen))
        XCTAssertFalse(MockAIClient.isGoAhead(fifteen))
    }

    func testWantsProposal() {
        check(MockAIClient.wantsProposal, [
            ("もっと見やすく", true),
            ("おすすめは?", true),
            ("オススメある", true),
            ("案を出して", true),
            ("金額を大きくして", false),
            ("家計簿を作って", false),
            ("", false),
        ], "wantsProposal")
    }

    func testWantsMultiProposal() {
        XCTAssertTrue(MockAIClient.wantsMultiProposal("機能を足したい"))
        XCTAssertFalse(MockAIClient.wantsMultiProposal("機能を足して"))
    }

    func testWantsSharedBoard() {
        check(MockAIClient.wantsSharedBoard, [
            ("掲示板を作って", true),
            ("みんなの掲示板", true),
            ("掲示板を作って(無料版で作る: 他のユーザーとデータを共有する機能は入れない)", false),
            ("掲示板は自分だけで使う", false),
            ("家計簿を作って", false),
            ("", false),
        ], "wantsSharedBoard")
    }
}
