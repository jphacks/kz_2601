import XCTest
import AppIntents
@testable import Metamon

/// ショートカット(App Intents)の文言: 審査用の版は「作ったアプリ・変身」を避けて記録帳と言う(#247・docs/store/wording.md)。
/// 通常の版は今の文言のまま。
final class ReviewWordingIntentsTests: XCTestCase {
    func testOpenIntentWording() {
        // 端末の言語に左右されないよう、訳す前のキー(ソースの日本語)で見る
        let title = OpenTailorAppIntent.title.key
        let type = TailorAppEntity.typeDisplayRepresentation.name.key
        let notFound = TailorIntentError.targetNotFound("家計簿").localizedStringResource.key
        #if APP_STORE_REVIEW
        XCTAssertEqual(title, "記録帳を開く")
        XCTAssertEqual(type, "記録帳")
        XCTAssertTrue(notFound.hasSuffix("Tailor It で記録帳を確認してください"), notFound)
        #else
        XCTAssertEqual(title, "アプリを開く")
        XCTAssertEqual(type, "アプリ")
        XCTAssertTrue(notFound.hasSuffix("Tailor It でアプリを確認してください"), notFound)
        #endif
    }

    /// 数字を読む(#1994): 審査用の版でも「作ったアプリ・変身」を言わない
    func testStatIntentWording() {
        let words = [GetStatValueIntent.title.key, GetStatNumberIntent.title.key, StatEntity.typeDisplayRepresentation.name.key,
                     TailorIntentError.statNotFound.localizedStringResource.key]
        for w in words {
            XCTAssertFalse(w.contains("作ったアプリ") || w.contains("変身"), w)
        }
        #if APP_STORE_REVIEW
        XCTAssertTrue(words[3].contains("記録帳"), words[3])
        #else
        XCTAssertTrue(words[3].contains("アプリ"), words[3])
        #endif
    }
}
