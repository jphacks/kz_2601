import XCTest
import MetamonServices
import MetamonRenderer
@testable import Metamon

/// 話して入力(`transcribeField`・aiChat のマイク)の音声モデルの状態。モデルを使っている間は録音を始めない。
final class DictationReadinessTests: XCTestCase {
    func testRecognizingIsBusyEvenWhenProvisionalIsAvailable() {
        // 録音ファイルの文字起こし中: 本命は recognizing・仮の聞き取りがあるので isReady は true
        XCTAssertEqual(AppRuntime.dictationReadiness(isReady: true, status: .recognizing), .preparing)
        XCTAssertEqual(AppRuntime.dictationReadiness(isReady: false, status: .recognizing), .preparing)
    }

    func testOtherStates() {
        XCTAssertEqual(AppRuntime.dictationReadiness(isReady: true, status: .ready), .ready)
        XCTAssertEqual(AppRuntime.dictationReadiness(isReady: true, status: .loading), .ready, "準備中でも仮の聞き取りで話せる")
        XCTAssertEqual(AppRuntime.dictationReadiness(isReady: false, status: .loading), .preparing)
        XCTAssertEqual(AppRuntime.dictationReadiness(isReady: false, status: .downloading), .preparing)
        XCTAssertEqual(AppRuntime.dictationReadiness(isReady: false, status: .notDownloaded), .needsModel)
        XCTAssertEqual(AppRuntime.dictationReadiness(isReady: false, status: .failed("x")), .failed)
    }
}
