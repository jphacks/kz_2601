import XCTest
import MetamonServices
@testable import Metamon

/// 共有シートの「みんなに公開」(#2070): 公開・公開をやめる・失敗したら元の表示のまま一言
@MainActor
final class MacPublishTests: XCTestCase {
    private actor FakeSharing: SharingClient {
        var listings: [PublicAppListing] = []
        var failure: Error?
        var searchFails = false
        private(set) var calls: [String] = []
        private(set) var published: (description: String, tags: [String], authorName: String?)?

        func setFailure(_ error: Error?) { failure = error }
        func setListings(_ items: [PublicAppListing]) { listings = items }
        func setSearchFails() { searchFails = true }

        func publish(appId: String, description: String, tags: [String], authorName: String?) async throws -> PublicAppListing {
            calls.append("publish \(appId)")
            if let failure { throw failure }
            published = (description, tags, authorName)
            let listing = PublicAppListing(id: appId, name: "家計簿", description: description, tags: tags, mine: true)
            listings.append(listing)
            return listing
        }
        func unpublish(appId: String) async throws {
            calls.append("unpublish \(appId)")
            if let failure { throw failure }
            listings.removeAll { $0.id == appId }
        }
        func searchPublicApps(query: String, sort: String) async throws -> [PublicAppListing] {
            calls.append("search \(sort)")
            if searchFails { throw URLError(.notConnectedToInternet) }
            return listings
        }
        func share(appId: String) async throws -> ShareInfo { throw URLError(.unsupportedURL) }
        func revokeShare(token: String) async throws {}
        func importShare(token: String) async throws -> InstalledApp { throw URLError(.unsupportedURL) }
        func install(publicAppId: String) async throws -> InstalledApp { throw URLError(.unsupportedURL) }
    }

    private func model(_ fake: FakeSharing) -> MacPublishModel {
        MacPublishModel(appId: "app-1", appName: "家計簿", sharing: fake)
    }

    // MARK: - 読み込み

    func testLoadFindsOwnListing() async {
        let fake = FakeSharing()
        await fake.setListings([PublicAppListing(id: "app-1", name: "n", installs: 3, mine: false),
                                PublicAppListing(id: "app-1", name: "n", installs: 5, mine: true)])
        let m = model(fake)
        XCTAssertEqual(m.phase, .loading)
        await m.load()
        guard case .published(let listing) = m.phase else { return XCTFail("公開中") }
        XCTAssertEqual(listing.installs, 5, "自分の掲載だけを見る")
        let calls = await fake.calls
        XCTAssertEqual(calls, ["search new"], "iOS と同じく新着から探す")
    }

    func testLoadWithoutListingOrOnFailureIsUnpublished() async {
        let fake = FakeSharing()
        let m = model(fake)
        await m.load()
        XCTAssertEqual(m.phase, .unpublished)

        await fake.setListings([PublicAppListing(id: "app-1", name: "n", mine: true)])
        await fake.setSearchFails()
        await m.load()
        XCTAssertEqual(m.phase, .unpublished, "読めなければ公開していない扱い(iOS と同じ)")
    }

    // MARK: - 公開

    func testPublishSendsDescriptionAndTags() async {
        let fake = FakeSharing()
        let m = model(fake)
        await m.load()
        m.description = "毎日の出費"
        m.tags = "家計 習慣、お金,節約　週"
        let notice = await m.publish()
        XCTAssertEqual(notice, L10n.tr("「探す」に公開しました"))
        XCTAssertTrue(m.isPublished)
        XCTAssertFalse(m.busy)
        let sent = await fake.published
        XCTAssertEqual(sent?.description, "毎日の出費")
        XCTAssertEqual(sent?.tags, ["家計", "習慣", "お金", "節約", "週"], "半角・全角スペース・「,」「、」で区切る")
        XCTAssertNil(sent?.authorName)
    }

    func testPublishFailureKeepsFormAndSaysSo() async {
        let fake = FakeSharing()
        let m = model(fake)
        await m.load()
        m.description = "説明"
        await fake.setFailure(APIError.server(status: 500, code: "x", message: "boom"))
        let notice = await m.publish()
        XCTAssertEqual(m.phase, .unpublished, "元の表示のまま")
        XCTAssertEqual(m.description, "説明", "入れた説明は残す")
        XCTAssertTrue(notice.hasPrefix(L10n.tr("公開できませんでした")), notice)
        XCTAssertFalse(notice.contains("boom"), "サーバーの文は出さない")
        XCTAssertFalse(m.busy)
    }

    func testPublishTextNotAllowedFlagsField() async {
        let fake = FakeSharing()
        let m = model(fake)
        await m.load()
        await fake.setFailure(APIError.server(status: 422, code: "text_not_allowed", message: "x",
                                              details: ["field": .string("tags")]))
        let notice = await m.publish()
        XCTAssertEqual(notice, ErrorCopy.textNotAllowed(fieldName: L10n.tr("タグ")))
        XCTAssertEqual(m.flaggedField, "tags")
        XCTAssertEqual(m.phase, .unpublished)
        m.edited("description")
        XCTAssertEqual(m.flaggedField, "tags", "別の欄を直しても印は残す")
        m.edited("tags")
        XCTAssertNil(m.flaggedField)
    }

    // MARK: - 公開をやめる

    func testUnpublish() async {
        let fake = FakeSharing()
        await fake.setListings([PublicAppListing(id: "app-1", name: "n", mine: true)])
        let m = model(fake)
        await m.load()
        XCTAssertTrue(m.isPublished)
        let notice = await m.unpublish()
        XCTAssertEqual(notice, L10n.tr("公開をやめました"))
        XCTAssertEqual(m.phase, .unpublished)
        let calls = await fake.calls
        XCTAssertEqual(calls.last, "unpublish app-1")
    }

    func testUnpublishFailureKeepsPublished() async {
        let fake = FakeSharing()
        await fake.setListings([PublicAppListing(id: "app-1", name: "n", installs: 2, mine: true)])
        let m = model(fake)
        await m.load()
        await fake.setFailure(URLError(.notConnectedToInternet))
        let notice = await m.unpublish()
        XCTAssertTrue(notice.hasPrefix(L10n.tr("非公開にできませんでした")), notice)
        XCTAssertTrue(m.isPublished, "元の表示のまま")
        XCTAssertFalse(m.busy)
    }

    // MARK: - 共有シートとのつなぎ・注意の文言

    func testShareModelHasPublishingOnlyWithSharing() {
        XCTAssertNil(MacShareAppModel(appId: "a", appName: "n", sharing: nil).publishing)
        XCTAssertNotNil(MacShareAppModel(appId: "a", appName: "n", sharing: FakeSharing()).publishing)
    }

    func testConfirmCopyIsSharedWithIOS() {
        XCTAssertEqual(PublishConfirmCopy.points.count, 4, "Android #1297・iOS と同じ 4 つ")
        XCTAssertTrue(PublishConfirmCopy.title(appName: "家計簿").contains("家計簿"))
    }
}
