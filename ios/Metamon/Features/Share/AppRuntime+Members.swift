import MetamonDSL
import MetamonRenderer
import MetamonServices
import Observation

/// 表示中のアプリの参加者と、自分が管理者か(#272)。AppRuntime(@Observable)には拡張で保存プロパティを足せないので別に持つ。
@MainActor
@Observable
final class AppMembersStore {
    static let shared = AppMembersStore()

    struct Entry: Equatable {
        var members: [AppMember]
        /// nil = 分からない(読めなかった)
        var isAdmin: Bool?
    }

    /// appId → 読んだ結果
    var entries: [String: Entry] = [:]
}

extension AppRuntime {
    /// 参加者を使う DSL か(type member・editableBy・deleteBy・visibleTo)。使わないアプリでは members API を呼ばない。
    nonisolated static func usesMembers(_ dsl: AppDSL) -> Bool {
        let fields = dsl.entities.contains { e in
            e.deleteBy != nil || e.fields.contains { $0.type == .member || $0.isAdminOnly }
        }
        return fields || dsl.screens.contains { s in s.components.contains(where: Self.hasVisibleTo) }
    }

    private nonisolated static func hasVisibleTo(_ c: Component) -> Bool {
        c.visibleTo != nil || c.children.contains(where: hasVisibleTo)
    }

    /// DSL を当てた時に呼ぶ。参加者を使うアプリなら読み直す(審査用の版は共有を外しているので何もしない)。
    func noteAppMembers() {
        #if !APP_STORE_REVIEW
        guard Self.usesMembers(dsl) else { return }
        Task { await reloadAppMembers() }
        #endif
    }

    /// 参加者と自分の立場を読み直す(管理者にした・外した後にも呼ぶ)。参加していなければ空・管理者でない。
    func reloadAppMembers() async {
        #if !APP_STORE_REVIEW
        guard let client = services.sharing as? any MembershipClient else { return }
        let appId = document.appId
        do {
            let list = try await client.members(appId: appId)
            AppMembersStore.shared.entries[appId] = .init(
                members: list.items.map { AppMember(uid: $0.uid, displayName: $0.label, isAdmin: $0.isAdmin) },
                isAdmin: list.isAdmin)
        } catch {
            if SharedAccessDenial(error) == .notMember {
                AppMembersStore.shared.entries[appId] = .init(members: [], isAdmin: false)
            }
            print("[Metamon] members load failed: appId=\(appId) \(error)")
        }
        #endif
    }
}

#if !APP_STORE_REVIEW
extension AppRuntime: MembersHost {
    var appMembers: [AppMember] { AppMembersStore.shared.entries[document.appId]?.members ?? [] }
    var isAppAdmin: Bool? { AppMembersStore.shared.entries[document.appId]?.isAdmin }
}
#endif
