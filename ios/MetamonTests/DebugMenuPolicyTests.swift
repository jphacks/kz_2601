import XCTest
@testable import Metamon

#if METAMON_STG
/// #2079: 開発用メニューを出すかの判定。`METAMON_STG` が無い版はコードごと無いので、ここでは
/// 「stg の構成でも prod に向いていたら出さない」取りこぼし防止の分かれ目を確かめる。
final class DebugMenuPolicyTests: XCTestCase {
    private let stgAI = URL(string: "https://ai-stg-378953239426.asia-northeast1.run.app/")
    private let stgAppConfig = URL(string: "https://appconfig-stg-378953239426.asia-northeast1.run.app/")
    private let prodAI = URL(string: "https://ai-prod-378953239426.asia-northeast1.run.app/")
    private let prodAppConfig = URL(string: "https://appconfig-prod-378953239426.asia-northeast1.run.app/")

    func testStagingURLsShowMenu() {
        XCTAssertTrue(DebugMenuPolicy.isAvailable(aiURL: stgAI, appConfigURL: stgAppConfig))
    }

    func testAllMockShowsMenu() {
        XCTAssertTrue(DebugMenuPolicy.isAvailable(aiURL: nil, appConfigURL: nil))
    }

    func testLocalhostShowsMenu() {
        XCTAssertTrue(DebugMenuPolicy.isAvailable(aiURL: URL(string: "http://127.0.0.1:8081/"),
                                                  appConfigURL: URL(string: "http://127.0.0.1:8080/")))
    }

    /// 片方だけ prod でも出さない(取りこぼし防止)
    func testProdAIHidesMenu() {
        XCTAssertFalse(DebugMenuPolicy.isAvailable(aiURL: prodAI, appConfigURL: stgAppConfig))
    }

    func testProdAppConfigHidesMenu() {
        XCTAssertFalse(DebugMenuPolicy.isAvailable(aiURL: stgAI, appConfigURL: prodAppConfig))
    }

    func testBothProdHidesMenu() {
        XCTAssertFalse(DebugMenuPolicy.isAvailable(aiURL: prodAI, appConfigURL: prodAppConfig))
    }

    /// 目印はホスト名で見る(パスに `-prod-` が入っていても prod 扱いにしない)
    func testMarkerIsMatchedOnHostOnly() {
        XCTAssertFalse(DebugMenuPolicy.isProd(URL(string: "https://ai-stg-1.run.app/-prod-/v1")))
        XCTAssertTrue(DebugMenuPolicy.isProd(URL(string: "https://AI-PROD-1.run.app/v1")))
        XCTAssertFalse(DebugMenuPolicy.isProd(nil))
    }
}

/// #2079: メニューの先頭に出す「今の環境」。
final class DebugEnvironmentTests: XCTestCase {
    private func rows(uid: String?, isAnonymous: Bool?) -> [DebugEnvironmentRow] {
        DebugEnvironment.rows(configuration: "Staging", version: "1.0.0", build: "2026103001",
                              aiURL: URL(string: "https://ai-stg-1.run.app/"),
                              appConfigURL: URL(string: "https://appconfig-stg-1.run.app/"),
                              isLiveAppConfig: true, firebaseEnv: "stg",
                              uid: uid, isAnonymous: isAnonymous, appId: "app-1")
    }

    func testRowsHoldEveryItemTheIssueAsksFor() {
        let labels = rows(uid: "uid-1", isAnonymous: true).map(\.label)
        XCTAssertEqual(labels, ["構成", "版 / ビルド", "ai", "appconfig", "Firebase", "uid", "アカウント", "appId"])
    }

    func testValues() {
        let by = Dictionary(uniqueKeysWithValues: rows(uid: "uid-1", isAnonymous: true).map { ($0.label, $0.value) })
        XCTAssertEqual(by["構成"], "Staging")
        XCTAssertEqual(by["版 / ビルド"], "1.0.0 (2026103001)")
        XCTAssertEqual(by["ai"], "https://ai-stg-1.run.app/")
        XCTAssertEqual(by["Firebase"], "stg")
        XCTAssertEqual(by["uid"], "uid-1")
        XCTAssertEqual(by["アカウント"], "匿名")
        XCTAssertEqual(by["appId"], "app-1")
    }

    func testSignedInAndUnknownValues() {
        let by = Dictionary(uniqueKeysWithValues: rows(uid: nil, isAnonymous: nil).map { ($0.label, $0.value) })
        XCTAssertEqual(by["uid"], "未サインイン")
        XCTAssertEqual(by["アカウント"], "-")
        XCTAssertEqual(Dictionary(uniqueKeysWithValues: rows(uid: "u", isAnonymous: false).map { ($0.label, $0.value) })["アカウント"],
                       "Apple でサインイン")
    }

    /// 全モック・インメモリの時も「-」ではなく読んで分かる言葉にする
    func testMockValues() {
        let rows = DebugEnvironment.rows(configuration: "Debug", version: nil, build: nil,
                                         aiURL: nil, appConfigURL: nil, isLiveAppConfig: false,
                                         firebaseEnv: nil, uid: nil, isAnonymous: nil, appId: "app-1")
        let by = Dictionary(uniqueKeysWithValues: rows.map { ($0.label, $0.value) })
        XCTAssertEqual(by["ai"], "モック")
        XCTAssertEqual(by["appconfig"], "インメモリ")
        XCTAssertEqual(by["版 / ビルド"], "- (-)")
        XCTAssertEqual(by["Firebase"], "-")
    }

    /// appconfig の URL があっても live でなければインメモリ
    func testAppConfigNotLive() {
        let rows = DebugEnvironment.rows(configuration: "Debug", version: "1", build: "2",
                                         aiURL: nil, appConfigURL: URL(string: "https://appconfig-stg-1.run.app/"),
                                         isLiveAppConfig: false, firebaseEnv: "stg",
                                         uid: nil, isAnonymous: nil, appId: "a")
        XCTAssertEqual(rows.first { $0.label == "appconfig" }?.value, "インメモリ")
    }

    func testCopyableRowsAreUidAndAppId() {
        XCTAssertEqual(rows(uid: "u", isAnonymous: true).filter(\.copyable).map(\.label), ["uid", "appId"])
    }

    func testClipboardTextIsOneLinePerRow() {
        let text = DebugEnvironment.clipboardText(rows(uid: "uid-1", isAnonymous: true))
        XCTAssertEqual(text.split(separator: "\n").count, 8)
        XCTAssertTrue(text.contains("uid: uid-1"))
    }

    /// 環境変数が Info.plist より優先。展開されていない `$(…)` は値として読まない
    func testFirebaseEnvPrefersEnvironment() {
        XCTAssertEqual(DebugEnvironment.firebaseEnv(bundle: .main, environment: ["METAMON_FIREBASE_ENV": "prod"]), "prod")
        XCTAssertEqual(DebugEnvironment.firebaseEnv(bundle: .main, environment: ["METAMON_FIREBASE_ENV": "$(METAMON_FIREBASE_ENV)"]),
                       DebugEnvironment.firebaseEnv(bundle: .main, environment: [:]))
    }
}
#endif
