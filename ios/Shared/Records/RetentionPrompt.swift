import Foundation
import MetamonDSL

// 記録の保存期間(#482)の確認の中身と、確認した期間の保存先。iOS(`AppRuntime+Retention`)と Mac(#979)で共有する。

/// 確認した期間(appId → entity → 日数)。端末ごと。アプリを消したら一緒に消える程度の情報なので UserDefaults。
enum RetentionConsent {
    private static func key(_ appId: String) -> String { "retention.consent.\(appId)" }

    static func days(appId: String, defaults: UserDefaults = .standard) -> [String: Int] {
        (defaults.dictionary(forKey: key(appId)) as? [String: Int]) ?? [:]
    }

    static func set(_ days: Int, appId: String, entity: String, defaults: UserDefaults = .standard) {
        var all = Self.days(appId: appId, defaults: defaults)
        all[entity] = days
        defaults.set(all, forKey: key(appId))
    }
}

/// 確認ダイアログの中身。
struct RetentionPrompt: Identifiable, Equatable {
    struct Item: Equatable {
        var entity: String
        var name: String
        var days: Int
        var isShared: Bool
    }

    var appId: String
    var version: Int
    var items: [Item]
    var id: String { "\(appId)#\(version)#" + items.map { "\($0.entity)=\($0.days)" }.joined(separator: ",") }

    init(appId: String, version: Int, entities: [Entity]) {
        self.appId = appId
        self.version = version
        items = entities.compactMap { e in e.retentionDays.map { Item(entity: e.id, name: e.name, days: $0, isShared: e.isShared) } }
    }

    static func key(appId: String, entity: Entity) -> String { key(appId: appId, entity: entity.id, days: entity.retentionDays ?? 0) }
    static func key(appId: String, entity: String, days: Int) -> String { "\(appId)/\(entity)=\(days)" }

    /// 「支出」は30日、「日記」は1週間 … たった記録が自動で消えます。
    func message(english: Bool) -> String {
        let parts = items.map { english ? "\($0.name): \(RecordRetention.periodLabel(days: $0.days, english: true))" : "「\($0.name)」は\(RecordRetention.periodLabel(days: $0.days))" }
        let shared = items.contains(where: \.isShared)
        if english {
            return "Entries older than the period will be deleted automatically (\(parts.joined(separator: ", "))). Deleted entries cannot be restored."
                + (shared ? " For shared entries, only the ones you added are deleted." : "")
        }
        return parts.joined(separator: "、") + "たった記録を自動で消します。消えた記録は元に戻せません。" + (shared ? "共有の記録は自分が追加したものだけが消えます。" : "")
    }
}
