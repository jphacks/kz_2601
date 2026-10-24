import XCTest
@testable import MetamonServices

/// #182: 生成後の画面からの「〇〇アプリを作って」を、全画面の生成中表示にする判定。
final class WholeAppRequestTests: XCTestCase {
    func testCreatePhrases() {
        for text in ["筋トレの記録アプリを作って", "家計簿アプリが欲しい", "日記アプリをつくって。", "買い物リストのアプリがほしいです",
                     "散歩の記録アプリにして", "服薬チェックのアプリに変えて", "Make a budget app", "build a workout tracker app for me",
                     "I want a diary app"] {
            XCTAssertTrue(VoiceIntent.isWholeAppRequest(text), text)
        }
    }

    func testEditPhrasesStayInline() {
        for text in ["合計をもっと大きく", "アプリの色を青にして", "グラフを作って", "右下に追加ボタンを置いて",
                     "make the total bigger", "", "アプリ"] {
            XCTAssertFalse(VoiceIntent.isWholeAppRequest(text), text)
        }
    }
}
