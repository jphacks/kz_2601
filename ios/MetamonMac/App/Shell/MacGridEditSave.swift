import Foundation
import MetamonDSL
import MetamonServices

/// 画面の並びの編集(#2323)の保存。iOS の `finishGridEdit` と同じく、サーバーにあるアプリは appconfig の JSON Patch(`PATCH /dsl`)、
/// それ以外(端末だけのアプリ・同梱の例)は端末で当てて版を 1 つ上げる。どちらも端末に残す封筒は同じ書き換えで作る。
enum MacGridEditSave {
    static var summary: String { L10n.tr("部品の配置を変えました") }

    enum Outcome: Equatable {
        case saved
        /// 先にほかの版が保存されていた(409)。取り直して開き直してもらう
        case conflict
        case failed
    }

    enum SaveError: Error, Equatable {
        case conflict
        case failed
    }

    /// 保存した版と、端末に残す封筒(作れなければ nil)
    static func save(_ draft: GridEditDraft, base: AppDocument, baseEnvelope: Data?,
                     server: (any AppConfigClient)?) async throws -> (document: AppDocument, envelope: Data?) {
        var next: AppDocument
        if let server {
            do {
                next = try await server.patchDSL(appId: base.appId, baseVersion: base.version, patch: draft.patch,
                                                 summary: summary)
            } catch {
                print("[Metamon] Mac: grid edit save failed: \(error)")
                throw (error as? APIError)?.status == 409 ? SaveError.conflict : SaveError.failed
            }
            next.appId = base.appId
        } else {
            next = base
            next.dsl = draft.applied(to: base.dsl)
            next.version = base.version + 1
        }
        let envelope = baseEnvelope.flatMap { applied(draft, to: $0, version: next.version) }
        return (next, envelope)
    }

    /// 封筒(`{appId, version, dsl}`)の DSL に配置を当てる。画面・部品の id が合わなければ nil(古い封筒に当てない)
    static func applied(_ draft: GridEditDraft, to envelope: Data, version: Int) -> Data? {
        guard var root = try? JSONSerialization.jsonObject(with: envelope) as? [String: Any],
              var dsl = root["dsl"] as? [String: Any],
              var screens = dsl["screens"] as? [[String: Any]],
              draft.screenIndex < screens.count,
              screens[draft.screenIndex]["id"] as? String == draft.screenId,
              var components = screens[draft.screenIndex]["components"] as? [[String: Any]] else { return nil }
        for item in draft.items {
            guard item.index < components.count, components[item.index]["id"] as? String == item.componentId else { return nil }
            if !item.removed, item.area != item.original { components[item.index]["area"] = item.area.notation }
        }
        for item in draft.items.filter(\.removed).sorted(by: { $0.index > $1.index }) {
            components.remove(at: item.index)
        }
        screens[draft.screenIndex]["components"] = components
        dsl["screens"] = screens
        root["dsl"] = dsl
        root["version"] = version
        return try? JSONSerialization.data(withJSONObject: root)
    }
}

/// 保存すると中央はランタイムごと作り直される(最初のタブに戻る)。編集していた画面を作り直した後に開き直す
@MainActor
enum MacGridEditReturn {
    private static var pending: [String: String] = [:]

    static func set(appId: String, screen: String) { pending[appId] = screen }

    static func take(appId: String) -> String? { pending.removeValue(forKey: appId) }
}
