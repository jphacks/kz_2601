#if DEBUG
import Foundation
import MetamonServices

/// UI テスト(#165)用の DEBUG フック `METAMON_MOCK_SHARING=1`(`METAMON_MOCK=1` と併用)。
/// モックはサーバーが無いので変身一覧が空・シェアボタンも出ない。ここで一覧に 2 件入れ、シェアを端末内で完結させる。
enum DebugMockSharing {
    static var isEnabled: Bool { ProcessInfo.processInfo.environment["METAMON_MOCK_SHARING"] == "1" }

    static let apps: [AppSummary] = [
        AppSummary(id: "uitest-kakeibo", name: "家計簿", currentVersion: 3, updatedAt: "2026-10-24T09:00:00Z"),
        AppSummary(id: "uitest-shopping", name: "買い物リスト", currentVersion: 2, updatedAt: "2026-10-23T09:00:00Z"),
    ]
}

/// 通信しない `SharingClient`。リンクは固定トークン。
struct DebugSharingClient: SharingClient {
    func share(appId: String) async throws -> ShareInfo {
        let name = DebugMockSharing.apps.first { $0.id == appId }?.name ?? appId
        return ShareInfo(token: "uitest-token", appId: appId, name: name, dslVersion: 1,
                         url: "tailorit://import/uitest-token", webUrl: "https://example.com/s/uitest-token")
    }
    func revokeShare(token: String) async throws {}
    func importShare(token: String) async throws -> InstalledApp { InstalledApp(appId: "uitest-imported", name: "インポート") }
    func publish(appId: String, description: String, tags: [String], authorName: String?) async throws -> PublicAppListing {
        PublicAppListing(id: "pub-\(appId)", name: appId, description: description, tags: tags, mine: true)
    }
    func unpublish(appId: String) async throws {}
    func searchPublicApps(query: String, sort: String) async throws -> [PublicAppListing] { [] }
    func install(publicAppId: String) async throws -> InstalledApp { InstalledApp(appId: publicAppId, name: publicAppId) }
}
#endif
