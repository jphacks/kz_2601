import XCTest
@testable import Metamon

/// #2079: 開発用メニューを出すか(METAMON_STG の版だけ・接続先が prod なら出さない)。
final class DebugMenuPolicyTests: XCTestCase {
    private let stgAI = URL(string: "https://ai-stg-378953239426.asia-northeast1.run.app/")
    private let stgAppConfig = URL(string: "https://appconfig-stg-378953239426.asia-northeast1.run.app/")
    private let prodAI = URL(string: "https://ai-prod-378953239426.asia-northeast1.run.app/")
    private let prodAppConfig = URL(string: "https://appconfig-prod-378953239426.asia-northeast1.run.app/")

    func testShownForStgURLs() {
        XCTAssertTrue(DebugMenuPolicy.isAvailable(compiledIn: true, aiURL: stgAI, appConfigURL: stgAppConfig))
    }

    func testShownForMockAndLocal() {
        XCTAssertTrue(DebugMenuPolicy.isAvailable(compiledIn: true, aiURL: nil, appConfigURL: nil))
        XCTAssertTrue(DebugMenuPolicy.isAvailable(compiledIn: true, aiURL: URL(string: "http://127.0.0.1:8081/"),
                                                  appConfigURL: URL(string: "http://127.0.0.1:8080/")))
    }

    func testHiddenWhenEitherURLIsProd() {
        XCTAssertFalse(DebugMenuPolicy.isAvailable(compiledIn: true, aiURL: prodAI, appConfigURL: stgAppConfig))
        XCTAssertFalse(DebugMenuPolicy.isAvailable(compiledIn: true, aiURL: stgAI, appConfigURL: prodAppConfig))
        XCTAssertFalse(DebugMenuPolicy.isAvailable(compiledIn: true, aiURL: prodAI, appConfigURL: prodAppConfig))
        XCTAssertFalse(DebugMenuPolicy.isAvailable(compiledIn: true, aiURL: nil, appConfigURL: prodAppConfig))
    }

    func testProMatchIgnoresCase() {
        XCTAssertTrue(DebugMenuPolicy.isProd(URL(string: "https://AI-PROD-1.example.com/")))
        XCTAssertFalse(DebugMenuPolicy.isProd(URL(string: "https://production.example.com/")))
        XCTAssertFalse(DebugMenuPolicy.isProd(nil))
    }

    /// METAMON_STG の無い版(Release / Review)は、接続先に関係なく出さない
    func testHiddenWithoutCompilationCondition() {
        XCTAssertFalse(DebugMenuPolicy.isAvailable(compiledIn: false, aiURL: stgAI, appConfigURL: stgAppConfig))
        XCTAssertFalse(DebugMenuPolicy.isAvailable(compiledIn: false, aiURL: nil, appConfigURL: nil))
    }

    /// アプリとテストのコンパイル条件が揃っている(テストは Debug で回すので true。Release の条件で回せば false 同士)
    func testCompiledInMatchesCondition() {
        #if METAMON_STG
        XCTAssertTrue(DebugMenuPolicy.isCompiledIn)
        #else
        XCTAssertFalse(DebugMenuPolicy.isCompiledIn)
        XCTAssertFalse(DebugMenuPolicy.isAvailable)
        #endif
    }

    /// この構成の接続先(project.yml の Debug = stg)ではメニューが出る
    func testAvailableInThisBuild() throws {
        #if METAMON_STG
        try XCTSkipIf(DebugMenuPolicy.isProd(BuildConfig.aiBaseURL) || DebugMenuPolicy.isProd(BuildConfig.appConfigBaseURL),
                      "prod に向けてビルドしている")
        XCTAssertTrue(DebugMenuPolicy.isAvailable)
        #endif
    }
}
