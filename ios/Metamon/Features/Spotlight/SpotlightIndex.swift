import AppIntents
import CoreSpotlight
import UIKit
import MetamonServices

/// Spotlight の索引(#1996)。今は作ったアプリの名前とアイコンだけ。記録は #1775 の決定まで入れない。
@MainActor
protocol SpotlightIndex: AnyObject {
    func index(apps: [SpotlightAppItem]) async
    func remove(appIds: [String]) async
    func removeAll() async
}

struct SpotlightAppItem: Equatable, Sendable {
    var id: String
    var name: String
    /// DSL の `app.icon`(SF Symbols 名)
    var icon: String?
}

/// domain と項目の id。記録を足す時は `records.<appId>` の domain に入れる(アプリを消す時に domain ごと消せる)。
enum SpotlightID {
    static let appsDomain = "apps"
    static func recordsDomain(appId: String) -> String { "records.\(appId)" }

    private static let appPrefix = "apps/"
    static func app(_ appId: String) -> String { appPrefix + appId }

    static func appId(fromItem identifier: String) -> String? {
        guard identifier.hasPrefix(appPrefix) else { return nil }
        let id = String(identifier.dropFirst(appPrefix.count))
        return id.isEmpty ? nil : id
    }

    /// Spotlight の結果をタップして届いた `NSUserActivity` から appId
    static func appId(from activity: NSUserActivity) -> String? {
        guard activity.activityType == CSSearchableItemActionType,
              let identifier = activity.userInfo?[CSSearchableItemActivityIdentifier] as? String else { return nil }
        return appId(fromItem: identifier)
    }
}

/// 端末の索引(`CSSearchableIndex.default()`)。失敗は記録だけ(検索に出ないだけで本体の動作は変わらない)。
final class CoreSpotlightIndex: SpotlightIndex {
    private var searchIndex: CSSearchableIndex { .default() }

    func index(apps: [SpotlightAppItem]) async {
        let items = apps.map(Self.searchableItem)
        do {
            try await searchIndex.indexSearchableItems(items)
        } catch {
            print("[Metamon] spotlight: index failed: \(error)")
        }
    }

    func remove(appIds: [String]) async {
        do {
            try await searchIndex.deleteSearchableItems(withIdentifiers: appIds.map(SpotlightID.app))
            // 後で記録を入れた時も、アプリと一緒に消える
            try await searchIndex.deleteSearchableItems(withDomainIdentifiers: appIds.map(SpotlightID.recordsDomain))
        } catch {
            print("[Metamon] spotlight: remove failed: \(error)")
        }
    }

    func removeAll() async {
        do {
            try await searchIndex.deleteAllSearchableItems()
        } catch {
            print("[Metamon] spotlight: removeAll failed: \(error)")
        }
    }

    static func searchableItem(_ app: SpotlightAppItem) -> CSSearchableItem {
        let attributes = CSSearchableItemAttributeSet(contentType: .content)
        attributes.title = app.name
        attributes.displayName = app.name
        if let icon = app.icon, let image = UIImage(systemName: icon, withConfiguration: UIImage.SymbolConfiguration(pointSize: 60)) {
            attributes.thumbnailData = image.withTintColor(.tintColor, renderingMode: .alwaysOriginal).pngData()
        }
        if #available(iOS 18, *) { // 開く Intent(OpenTailorAppIntent)とつなぐ
            attributes.associateAppEntity(TailorAppEntity(AppSummary(id: app.id, name: app.name, currentVersion: 0)))
        }
        return CSSearchableItem(uniqueIdentifier: SpotlightID.app(app.id), domainIdentifier: SpotlightID.appsDomain,
                                attributeSet: attributes)
    }
}

@available(iOS 18, *)
extension TailorAppEntity: IndexedEntity {}

/// 何もしない(ユニットテスト等の既定)。本物は `MetamonApp` が入れる。
final class NoopSpotlightIndex: SpotlightIndex {
    func index(apps: [SpotlightAppItem]) async {}
    func remove(appIds: [String]) async {}
    func removeAll() async {}
}
