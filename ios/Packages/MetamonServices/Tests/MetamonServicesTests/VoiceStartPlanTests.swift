import XCTest
@testable import MetamonServices

/// #966: Mac で声で頼む時、録音の前にモデルのダウンロードを聞くか。
final class VoiceStartPlanTests: XCTestCase {
    private func plan(needsDownload: Bool = true, approved: Bool = false, expensive: Bool = false,
                      slow: Bool = false, provisional: Bool = true, asked: Bool = false) -> VoiceStartPlan {
        VoiceStartPlan.decide(needsDownload: needsDownload, isApproved: approved, isExpensiveNetwork: expensive,
                              slowHardware: slow, provisionalAvailable: provisional, askedThisLaunch: asked)
    }

    func testModelOnDiskRecords() {
        XCTAssertEqual(plan(needsDownload: false, provisional: false), .record)
    }

    func testApprovedRecordsWhileDownloading() {
        XCTAssertEqual(plan(approved: true), .record)
    }

    func testApprovedButExpensiveNetworkAsksAgain() {
        XCTAssertEqual(plan(approved: true, expensive: true), .askDownload)
    }

    func testAppleSiliconAsksFirst() {
        XCTAssertEqual(plan(), .askDownload)
    }

    func testAfterLaterUsesProvisional() {
        XCTAssertEqual(plan(asked: true), .record)
    }

    func testAfterLaterWithoutProvisionalIsTextOnly() {
        XCTAssertEqual(plan(provisional: false, asked: true), .textOnly)
    }

    func testIntelUsesAppleRecognitionWithoutAsking() {
        XCTAssertEqual(plan(slow: true), .record)
    }

    func testIntelWithoutAppleRecognitionAsksOnce() {
        XCTAssertEqual(plan(slow: true, provisional: false), .askDownload)
        XCTAssertEqual(plan(slow: true, provisional: false, asked: true), .textOnly)
    }
}
