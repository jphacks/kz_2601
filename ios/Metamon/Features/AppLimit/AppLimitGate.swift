import SwiftUI
import MetamonServices

/// 持てるアプリの数の上限(#630・402 `app_limit_reached`)の置き換えシートを出す司令塔。
/// 作る・複製・取り込み・参加・生成のどこで断られても `present` に渡し、解決したら `retry` で同じ操作をやり直す。
/// シートはいちばん手前の presenter(根・音声シート・参加の確認)から出す(シートの上には別のシートを重ねられないため)。
@MainActor
@Observable
final class AppLimitGate {
    static let shared = AppLimitGate()

    struct Request: Identifiable {
        let id = UUID()
        var presenter: UUID
        var limit: AppLimit
        /// 置き換え・枠の購入の後にやり直す操作
        var retry: @MainActor () async -> Void
    }

    private(set) var request: Request?
    @ObservationIgnored private var presenters: [UUID] = []

    /// 出せたら true(presenter が無い = 画面の外。呼び出し側が文言で知らせる)
    @discardableResult
    func present(_ limit: AppLimit, retry: @escaping @MainActor () async -> Void) -> Bool {
        print("[Metamon] app limit: held=\(limit.held) limit=\(limit.limit) canBuySlot=\(limit.canBuySlot)")
        guard let top = presenters.last else { return false }
        request = Request(presenter: top, limit: limit, retry: retry)
        return true
    }

    func dismiss() { request = nil }

    func presenterAppeared(_ id: UUID) {
        presenters.removeAll { $0 == id }
        presenters.append(id)
    }

    func presenterDisappeared(_ id: UUID) {
        presenters.removeAll { $0 == id }
        if request?.presenter == id { request = nil }
    }
}

/// 置き換えシートの present 元。
struct AppLimitSheetPresenter: ViewModifier {
    @State private var id = UUID()

    func body(content: Content) -> some View {
        let gate = AppLimitGate.shared
        return content
            .onAppear { gate.presenterAppeared(id) }
            .onDisappear { gate.presenterDisappeared(id) }
            .sheet(item: Binding(
                get: { gate.request?.presenter == id ? gate.request : nil },
                set: { if $0 == nil, gate.request?.presenter == id { gate.dismiss() } }
            )) { request in
                AppLimitSheet(request: request)
            }
    }
}

extension View {
    /// この画面(シート)の上に、持てるアプリの置き換えシートを出せるようにする
    func appLimitSheets() -> some View { modifier(AppLimitSheetPresenter()) }
}

extension AppRuntime {
    /// 402 `app_limit_reached` を置き換えシートに渡す。出せなければトーストで知らせる。
    func presentAppLimit(_ limit: AppLimit, retry: @escaping @MainActor () async -> Void) {
        if !AppLimitGate.shared.present(limit, retry: retry) {
            toast = AppLimitCopy.toast(limit)
        }
    }

    /// 置き換えシートの 1 行を実行する。自分のアプリ = 削除(今の削除の流れ: 端末のデータ・通知も消す)、
    /// 参加したアプリ = 共有から抜けてから自分の写しを消す。true = 1 つ空いた。
    func replaceHeldApp(_ row: AppReplacePlan.Row) async -> Bool {
        if case .leave(let appId) = AppReplacePlan.action(for: row) {
            #if !APP_STORE_REVIEW
            if let membership = services.sharing as? any MembershipClient {
                do { try await membership.leave(appId: appId) } catch {
                    ErrorLog.record(error, context: "app limit leave") // 抜けられなくても写しは消す(一覧から外れれば数えない)
                }
            }
            #endif
        }
        let summary = apps.first { $0.id == row.id } ?? AppSummary(id: row.id, name: row.name, currentVersion: 2)
        let removed = await deleteApp(summary)
        if removed { await appsList() }
        return removed
    }

    #if !APP_STORE_REVIEW
    /// 枠を 1 つ買う(`requestId` は押した 1 回ごと・送り直しは同じ値)。足りなければお店を開いて false。
    func buyAppSlot(requestId: String) async -> Bool {
        guard let coins = services.coins else { return false }
        do {
            let p = try await coins.buyAppSlot(requestId: requestId)
            await CoinGate.shared.store.refreshWallet()
            toast = AppLimitCopy.slotBought(total: p.appSlots)
            return true
        } catch {
            if let denial = CoinDenial(error) {
                CoinGate.shared.handle(denial) // 足りない → お店
            } else {
                toast = ErrorCopy.failure(AppLimitCopy.slotFailed, error, context: "buy app slot")
            }
            return false
        }
    }
    #endif
}
