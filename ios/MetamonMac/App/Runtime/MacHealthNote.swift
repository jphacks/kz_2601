import Foundation
import MetamonDSL
import MetamonRenderer

/// `Field.health`(#167)の Mac 版(#958)。Mac には HealthKit が無いので、記録は保存してヘルスケアには書かない。
/// そのことをアプリごとに 1 回だけ一言で知らせる。印は UserDefaults(サインアウトの `MacLocalWipe` で消える)。
struct MacHealthNote {
    var defaults: UserDefaults = .standard

    static var message: String { L10n.tr("この Mac ではヘルスケアに書き込みません") }

    /// iOS の `writeHealthFields` が書く時と同じ条件: 新しい記録で、health のある項目に値がある
    static func applies(entity: Entity?, data: [String: JSONValue], isNew: Bool) -> Bool {
        guard isNew, let entity else { return false }
        return entity.fields.contains { field in
            guard field.health != nil, let value = data[field.id] else { return false }
            return value != .null
        }
    }

    /// 出すなら一言を返し、出したと覚える。2 回目以降は nil
    func noteOnce(appId: String) -> String? {
        let key = Self.key(appId)
        guard !defaults.bool(forKey: key) else { return nil }
        defaults.set(true, forKey: key)
        return Self.message
    }

    static func key(_ appId: String) -> String { "macHealthNoteShown.\(appId)" }
}
