#if !APP_STORE_REVIEW
import XCTest
import MetamonServices
@testable import Metamon

/// #2851: 共有シートの「みんなに公開」(#2070)の状態の移り変わり(続けて操作した時)と、審査用ビルドで外れていること。
/// 1 回ずつの公開・やめる・失敗は MacPublishTests にある。
@MainActor
final class MacPublishSectionTests: XCTestCase {
    private actor FakeSharing: SharingClient {
        var listings: [PublicAppListing] = []
        var failure: Error?
        /// publish を待たせる時間(続けて押された時の確かめ)
        var publishDelay: Duration = .zero
        private(set) var calls: [String] = []

        func setFailure(_ error: Error?) { failure = error }
        func setPublishDelay(_ d: Duration) { publishDelay = d }

        func publish(appId: String, description: String, tags: [String], authorName: String?) async throws -> PublicAppListing {
            calls.append("publish \(appId)")
            if publishDelay != .zero { try await Task.sleep(for: publishDelay) }
            if let failure { throw failure }
            let listing = PublicAppListing(id: appId, name: "家計簿", description: description, tags: tags, installs: 0, mine: true)
            listings.append(listing)
            return listing
        }
        func unpublish(appId: String) async throws {
            calls.append("unpublish \(appId)")
            if let failure { throw failure }
            listings.removeAll { $0.id == appId }
        }
        func searchPublicApps(query: String, sort: String) async throws -> [PublicAppListing] { listings }
        func share(appId: String) async throws -> ShareInfo { throw URLError(.unsupportedURL) }
        func revokeShare(token: String) async throws {}
        func importShare(token: String) async throws -> InstalledApp { throw URLError(.unsupportedURL) }
        func install(publicAppId: String) async throws -> InstalledApp { throw URLError(.unsupportedURL) }
    }

    private func model(_ fake: FakeSharing) -> MacPublishModel {
        MacPublishModel(appId: "app-1", appName: "家計簿", sharing: fake)
    }

    // MARK: - 移り変わり

    /// 未公開 → 公開 → 公開中 → やめる → 未公開 → 失敗 → 未公開のまま一言
    func testPublishUnpublishRoundTrip() async {
        let fake = FakeSharing()
        let m = model(fake)
        await m.load()
        XCTAssertEqual(m.phase, .unpublished)
        XCTAssertFalse(m.isPublished)

        m.description = "毎日の出費"
        let published = await m.publish()
        XCTAssertEqual(published, L10n.tr("「探す」に公開しました"))
        guard case .published(let listing) = m.phase else { return XCTFail("公開中") }
        XCTAssertEqual(listing.description, "毎日の出費")
        XCTAssertEqual(listing.installs, 0)

        let stopped = await m.unpublish()
        XCTAssertEqual(stopped, L10n.tr("公開をやめました"))
        XCTAssertEqual(m.phase, .unpublished)
        XCTAssertEqual(m.description, "毎日の出費", "やめても入れた説明は残す(もう一度公開しやすい)")

        await fake.setFailure(URLError(.notConnectedToInternet))
        let notice = await m.publish()
        XCTAssertTrue(notice.hasPrefix(L10n.tr("公開できませんでした")), notice)
        XCTAssertEqual(m.phase, .unpublished, "失敗は元の表示のまま")

        await fake.setFailure(nil)
        let retried = await m.publish()
        XCTAssertEqual(retried, L10n.tr("「探す」に公開しました"), "失敗の後にもう一度押せば公開できる")
        XCTAssertTrue(m.isPublished)
        await m.load()
        XCTAssertTrue(m.isPublished, "読み直しても公開中")
        let calls = await fake.calls
        XCTAssertEqual(calls, ["publish app-1", "unpublish app-1", "publish app-1", "publish app-1"])
    }

    func testFlagClearsAfterSuccessfulRetry() async {
        let fake = FakeSharing()
        let m = model(fake)
        await m.load()
        await fake.setFailure(APIError.server(status: 422, code: "text_not_allowed", message: "x",
                                              details: ["field": .string("description")]))
        _ = await m.publish()
        XCTAssertEqual(m.flaggedField, "description")

        await fake.setFailure(nil)
        _ = await m.publish()
        XCTAssertNil(m.flaggedField, "公開できたら印を外す")
        XCTAssertTrue(m.isPublished)
    }

    func testSecondPressWhileBusyIsIgnored() async {
        let fake = FakeSharing()
        await fake.setPublishDelay(.milliseconds(200))
        let m = model(fake)
        await m.load()

        async let first = m.publish()
        for _ in 0..<1_000 where !m.busy { await Task.yield() }
        XCTAssertTrue(m.busy, "公開の途中")
        let again = await m.publish()
        XCTAssertEqual(again, "", "途中で押しても何もしない")
        let stop = await m.unpublish()
        XCTAssertEqual(stop, "", "やめるも受け付けない")
        let done = await first
        XCTAssertEqual(done, L10n.tr("「探す」に公開しました"))
        XCTAssertFalse(m.busy)
        let calls = await fake.calls
        XCTAssertEqual(calls, ["publish app-1"], "送るのは 1 回だけ")
    }

    // MARK: - 審査用ビルド

    /// 審査用ビルド(APP_STORE_REVIEW)は「探す」への公開を外す。欄と、欄を出す共有シートが条件のコンパイルで囲まれていること。
    func testExcludedFromReviewBuild() throws {
        let ios = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
        for path in ["MetamonMac/App/Links/MacPublishSection.swift", "MetamonMac/App/Links/MacShareAppSheet.swift"] {
            let source = try String(contentsOf: ios.appending(path: path), encoding: .utf8)
            let lines = source.split(separator: "\n", omittingEmptySubsequences: true).map { $0.trimmingCharacters(in: .whitespaces) }
            let code = lines.filter { !$0.hasPrefix("//") }
            XCTAssertEqual(code.first, "#if !APP_STORE_REVIEW", "\(path) は全体を囲む")
            XCTAssertEqual(code.last, "#endif", path)
        }
        let project = try String(contentsOf: ios.appending(path: "project.yml"), encoding: .utf8)
        XCTAssertTrue(project.contains("APP_STORE_REVIEW"), "審査用の構成が条件を立てる")
    }
}
#endif
