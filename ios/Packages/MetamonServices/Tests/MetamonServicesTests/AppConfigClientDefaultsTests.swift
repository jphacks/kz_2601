import XCTest
import MetamonDSL
@testable import MetamonServices

/// AppConfigClient の既定の実装: 版が同じなら読まない(fetchDocumentIfChanged)・setAppGlance は未対応。
final class AppConfigClientDefaultsTests: XCTestCase {
    private static func doc(version: Int) -> AppDocument {
        let json = #"{"appId":"a1","version":\#(version),"dsl":{"schemaVersion":1,"app":{"name":"x"},"initialScreen":"home","entities":[],"screens":[]}}"#
        return try! JSONDecoder().decode(AppDocument.self, from: Data(json.utf8))
    }

    /// `fetchDocument` だけを持つ client。何回読まれたかを数える。
    private final class Bare: AppConfigClient, @unchecked Sendable {
        let version: Int
        let fails: Bool
        private let lock = NSLock()
        private var _fetches = 0
        var fetches: Int { lock.withLock { _fetches } }

        init(version: Int, fails: Bool = false) { self.version = version; self.fails = fails }

        func fetchDocument(appId: String) async throws -> AppDocument {
            lock.withLock { _fetches += 1 }
            if fails { throw URLError(.notConnectedToInternet) }
            return AppConfigClientDefaultsTests.doc(version: version)
        }
        func listVersions(appId: String) async throws -> [VersionSummary] { [] }
        func restore(appId: String, toVersion version: Int) async throws -> AppDocument { throw AppListEdit.unsupported }
        func list(appId: String, entity: String) async throws -> [Record] { [] }
        func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record { throw AppListEdit.unsupported }
        func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record { throw AppListEdit.unsupported }
        func delete(appId: String, entity: String, id: String) async throws {}
    }

    func testSameVersionReturnsNil() async throws {
        let client = Bare(version: 3)
        let r = try await client.fetchDocumentIfChanged(appId: "a1", currentVersion: 3)
        XCTAssertNil(r)
        XCTAssertEqual(client.fetches, 1, "既定は取って比べるだけ")
    }

    func testNewerVersionReturnsDocument() async throws {
        let client = Bare(version: 4)
        let r = try await client.fetchDocumentIfChanged(appId: "a1", currentVersion: 3)
        XCTAssertEqual(r?.version, 4)
        XCTAssertEqual(r?.appId, "a1")
    }

    func testOlderServerVersionAlsoCountsAsChanged() async throws {
        let client = Bare(version: 2)
        let r = try await client.fetchDocumentIfChanged(appId: "a1", currentVersion: 3)
        XCTAssertEqual(r?.version, 2, "≠ で判定する(古い版へ戻した時も渡す。進めるかは呼び出し側が決める)")
    }

    func testFetchFailureIsThrown() async {
        let client = Bare(version: 3, fails: true)
        do {
            _ = try await client.fetchDocumentIfChanged(appId: "a1", currentVersion: 3)
            XCTFail("読めない時は throw する")
        } catch {
            XCTAssertTrue(error is URLError)
        }
    }

    func testSetAppGlanceDefaultIsUnsupported() async {
        do {
            _ = try await Bare(version: 1).setAppGlance(appId: "a1", glanceStat: "total")
            XCTFail("既定は unsupported を投げる")
        } catch {
            XCTAssertTrue(error is AppListEdit.Unsupported)
        }
    }
}
