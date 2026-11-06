import XCTest
import SwiftUI
@testable import Metamon

/// #625 横向き: 高さが compact の時だけ飾りを小さくし、縦向きは従来の寸法のまま。
final class LandscapeLayoutTests: XCTestCase {
    func testCompactHeightIsLandscapeOnly() {
        XCTAssertTrue(LandscapeLayout.isCompactHeight(.compact))
        XCTAssertFalse(LandscapeLayout.isCompactHeight(.regular))
        XCTAssertFalse(LandscapeLayout.isCompactHeight(nil))
    }

    func testPortraitKeepsExistingSizes() {
        XCTAssertEqual(LandscapeLayout.micSize(isInitial: true, compactHeight: false), 132)
        XCTAssertEqual(LandscapeLayout.micSize(isInitial: false, compactHeight: false), 104)
        XCTAssertEqual(LandscapeLayout.headingSize(compactHeight: false), 30)
        XCTAssertEqual(LandscapeLayout.tutorialMascotSize(compactHeight: false), 132)
    }

    func testLandscapeShrinksDecorations() {
        XCTAssertLessThan(LandscapeLayout.micSize(isInitial: true, compactHeight: true), 132)
        XCTAssertLessThan(LandscapeLayout.micSize(isInitial: false, compactHeight: true), 104)
        // 初期画面のマイクは改変のシートより大きいまま(主役の順を変えない)
        XCTAssertGreaterThan(LandscapeLayout.micSize(isInitial: true, compactHeight: true),
                             LandscapeLayout.micSize(isInitial: false, compactHeight: true))
        XCTAssertLessThan(LandscapeLayout.headingSize(compactHeight: true), 30)
        XCTAssertLessThan(LandscapeLayout.tutorialMascotSize(compactHeight: true), 132)
    }

    func testReadableWidthFitsLandscapePhones() {
        // 最小の iPhone 横(SE 667pt)でも左右に余白が残り、最大の縦(440pt)より広い
        XCTAssertLessThan(LandscapeLayout.readableWidth, 667)
        XCTAssertGreaterThan(LandscapeLayout.readableWidth, 440)
    }
}
