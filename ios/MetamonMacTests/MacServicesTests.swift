import XCTest
import MetamonServices
@testable import Metamon

/// Mac の接続の組み立て(#930)と、エラーの状態カードのサインイン(#1280)
final class MacServicesTests: XCTestCase {
    func testMockWhenForced() {
        let services = MacServices.make(environment: ["METAMON_MOCK": "1", "METAMON_AI_BASE_URL": "https://ai.example.com"])
        XCTAssertFalse(services.isLive)
        XCTAssertTrue(services.ai is MockAIClient)
    }

    /// テストのホストには plist が無い(CI と同じ)。Firebase が無ければ URL があってもモック
    func testMockWithoutFirebase() throws {
        try XCTSkipIf(FirebaseBootstrap.isConfigured, "plist がある手元の版")
        let services = MacServices.make(environment: ["METAMON_AI_BASE_URL": "https://ai.example.com"])
        XCTAssertFalse(services.isLive)
        XCTAssertTrue(services.ai is MockAIClient)
    }

    func testURLReadsEnvironmentAndAddsSlash() {
        let url = MacServices.url(env: "X_URL", plist: "NoSuchKey", environment: ["X_URL": "https://ai.example.com"])
        XCTAssertEqual(url?.absoluteString, "https://ai.example.com/")
        XCTAssertNil(MacServices.url(env: "X_URL", plist: "NoSuchKey", environment: ["X_URL": "$(METAMON_AI_BASE_URL)"]))
        XCTAssertNil(MacServices.url(env: "X_URL", plist: "NoSuchKey", environment: [:]))
    }

    func testAppleSignInOnlyWhenLive() {
        let expired = ErrorPresentation(kind: .sessionExpired, transcript: "t", origin: .generate)
        XCTAssertTrue(MacErrorCard.showsAppleSignIn(expired, isLive: true))
        XCTAssertFalse(MacErrorCard.showsAppleSignIn(expired, isLive: false))
        let failed = ErrorPresentation(kind: .failed(message: "x"), transcript: "t", origin: .generate)
        XCTAssertFalse(MacErrorCard.showsAppleSignIn(failed, isLive: true))
    }
}
