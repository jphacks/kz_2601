import XCTest
@testable import Metamon

/// 規約・ポリシー・お問い合わせの公開ページ(#246)。App Store Connect に登録する URL と同じ。
final class LegalDocumentURLTests: XCTestCase {
    func testPublicURLs() {
        XCTAssertEqual(LegalDocument.privacyPolicy.publicURL?.absoluteString, "https://tailor-it.app/privacy")
        XCTAssertEqual(LegalDocument.terms.publicURL?.absoluteString, "https://tailor-it.app/terms")
        XCTAssertEqual(LegalDocument.contactURL.absoluteString, "https://tailor-it.app/contact")
    }

    func testBundledTextStillAvailable() {
        for doc in LegalDocument.allCases {
            XCTAssertNotNil(doc.bundledText, "同梱の \(doc.rawValue).md が無い")
        }
    }
}
