// 審査用ビルド(APP_STORE_REVIEW)は共有を外しているので、仲間探し(#507)も外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// 仲間探し(#507)の UI 状態。AppRuntime(@Observable)には拡張で保存プロパティを足せないので別に持つ。
@MainActor
@Observable
final class MatchingUI {
    static let shared = MatchingUI()

    /// 18 歳以上の確認を待っている操作(nil = 出していない)
    var askingAge: PendingInterest?
    @ObservationIgnored var ageGate = MatchingAgeGate()
    /// 「18 歳未満です」と答えた。開いた時・一覧の読み込みでは聞き直さない(ボタンを押した時だけ聞く)。メモリだけ
    @ObservationIgnored var declinedAge = false
    /// 申告をサーバーに合わせている途中(uid → 結果)。一覧が同時に読んでも 1 回にまとめる
    @ObservationIgnored private var syncing: [String: Task<Bool, Error>] = [:]
    /// "appId|uid" → そのアプリの一覧。body から引くので作っても観測を起こさない
    @ObservationIgnored private var stores: [String: MatchingStore] = [:]

    func store(appId: String, uid: String, client: any MatchingClient) -> MatchingStore {
        let key = "\(appId)|\(uid)"
        if let s = stores[key] { return s }
        let s = MatchingStore(appId: appId, client: client)
        stores[key] = s
        return s
    }

    /// サーバーに申告があるか(`MatchingAgeGate.sync`)。同じ uid の同時の呼び出しは 1 回にまとめる。
    func syncAge(uid: String, client: any MatchingClient) async throws -> Bool {
        if ageGate.isSent(uid: uid) { return true }
        let task = syncing[uid] ?? Task { [gate = ageGate] in try await gate.sync(uid: uid, client: client) }
        syncing[uid] = task
        defer { syncing[uid] = nil }
        return try await task.value
    }

    /// テスト用・サインアウト: 全部忘れる。
    func reset() {
        askingAge = nil
        declinedAge = false
        stores = [:]
        syncing = [:]
    }

    /// サインアウト・アカウント削除の後片付け: 申告を送った記録と、前のアカウントの一覧を消す。
    func wipe() {
        ageGate.forgetSent()
        reset()
    }
}

/// 18 歳以上の確認のあとに続ける操作。
enum PendingInterest: Identifiable {
    case send(entity: String, record: Record)
    case accept(MatchInterest)
    /// 仲間探しのアプリを開いた・一覧を読む(box が空 = 開いただけ)
    case refresh(Set<MatchBox>)
    /// サーバーに断られたプロフィールの投稿(400 age_unconfirmed)
    case post(entity: String, data: [String: JSONValue])

    var id: String {
        switch self {
        case .send(let entity, let record): "send/\(entity)/\(record.id)"
        case .accept(let item): "accept/\(item.uid)"
        case .refresh: "refresh" // 足しても同じシートのまま(askAge が box をまとめる)
        case .post(let entity, _): "post/\(entity)"
        }
    }
}

extension AppRuntime: MatchingHost {
    /// このアプリの一覧。live で、プロフィールの entity があり、サインインしている時だけ。
    var matchingStore: MatchingStore? {
        guard let client = services.matching, let me = currentUserId, !me.isEmpty,
              dsl.entities.contains(where: \.isMatchProfile) else { return nil }
        return MatchingUI.shared.store(appId: document.appId, uid: me, client: client)
    }

    // MARK: MatchingHost

    func interestState(entity: String, record: Record) -> InterestState? {
        guard let store = matchingStore, dsl.entity(entity)?.isMatchProfile == true,
              let author = record.author, !author.isEmpty, author != currentUserId,
              ModerationUI.shared.blocked[author] == nil else { return nil }
        return store.state(of: author)
    }

    func interestBusy(record: Record) -> Bool {
        guard let author = record.author else { return false }
        return matchingStore?.isBusy(author) ?? false
    }

    func sendInterest(entity: String, record: Record) async {
        guard let store = matchingStore, let author = record.author, !author.isEmpty, author != currentUserId else { return }
        guard let mine = records(for: entity).first(where: { $0.author == currentUserId }) else {
            toast = L10n.tr("先に自分のプロフィールを書いてください")
            return
        }
        guard MatchingUI.shared.ageGate.isConfirmed else {
            MatchingUI.shared.askingAge = .send(entity: entity, record: record)
            return
        }
        do {
            let result = try await store.send(to: author, recordId: record.id, fromRecordId: mine.id)
            ageRecorded() // ageConfirmed:true はサーバーも記録する
            if result.isMatched { await connected() }
        } catch {
            matchingFailed(error, retry: .send(entity: entity, record: record))
        }
    }

    func matchItems(_ box: MatchBox) -> [MatchInterest]? {
        guard let items = matchingStore?.items(box) else { return nil }
        let blocked = ModerationUI.shared.blocked
        return blocked.isEmpty ? items : items.filter { blocked[$0.uid] == nil }
    }

    func matchLoadFailed(_ box: MatchBox) -> Bool { matchingStore?.failed.contains(box) ?? false }

    func refreshMatches(_ boxes: Set<MatchBox>) async {
        guard let store = matchingStore else { return }
        await prepareMatching()
        for box in MatchBox.allCases where boxes.contains(box) {
            do {
                try await store.load(box)
            } catch {
                // 失敗は一覧の「読み込めませんでした」で出す。申告が無ければ聞いて、同意したら読み直す
                if (error as? APIError)?.isAgeUnconfirmed == true {
                    askAge(.refresh(boxes))
                    return
                }
            }
        }
    }

    func acceptInterest(_ item: MatchInterest) async {
        guard let store = matchingStore else { return }
        guard MatchingUI.shared.ageGate.isConfirmed else {
            MatchingUI.shared.askingAge = .accept(item)
            return
        }
        do {
            try await store.accept(item.uid)
            ageRecorded()
            await connected()
        } catch {
            matchingFailed(error, retry: .accept(item))
        }
    }

    func declineInterest(_ item: MatchInterest, box: MatchBox) async {
        guard let store = matchingStore else { return }
        do {
            if box == .outgoing { try await store.withdraw(item.uid) } else { try await store.decline(item.uid) }
        } catch {
            matchingFailed(error, retry: nil)
        }
    }

    func unmatch(_ item: MatchInterest) async {
        guard let store = matchingStore else { return }
        do {
            try await store.unmatch(item.uid)
            toast = L10n.tr("つながりを解除しました")
            await reloadMatchedRecords()
        } catch {
            matchingFailed(error, retry: nil)
        }
    }

    // MARK: 18 歳以上の確認

    /// 仲間探しのアプリを開いた・一覧を読む前: 端末の申告をまだ送っていなければ送る(`PUT /v1/me/age-confirmation`)。
    /// 端末にも(GET で見た)サーバーにも申告が無ければ確認を出す。通信できない時は何もしない(次に開いた時にまた送る)。
    func prepareMatching() async {
        guard let client = services.matching, let me = currentUserId, !me.isEmpty,
              dsl.entities.contains(where: \.isMatchProfile) else { return }
        guard let declared = try? await MatchingUI.shared.syncAge(uid: me, client: client), !declared else { return }
        askAge(.refresh([]))
    }

    /// 「18 歳以上です」を押した: 端末に覚えてサーバーに送り(年齢そのものは保存しない)、止めていた操作をやり直す。
    func confirmAdult(_ pending: PendingInterest) async {
        let ui = MatchingUI.shared
        ui.askingAge = nil
        ui.declinedAge = false
        ui.ageGate.confirm()
        if let client = services.matching, let me = currentUserId, !me.isEmpty {
            do {
                try await ui.ageGate.declare(uid: me, client: client)
            } catch {
                toast = Self.matchingErrorText(error) // 端末の確認は残す(次に開いた時に送る)
                return
            }
        }
        switch pending {
        case .send(let entity, let record): await sendInterest(entity: entity, record: record)
        case .accept(let item): await acceptInterest(item)
        case .refresh(let boxes): if !boxes.isEmpty { await refreshMatches(boxes) }
        case .post(let entity, let data): _ = await createRecord(entity: entity, data: data)
        }
    }

    /// 「18 歳未満です」: 何も覚えず、送らない。開いた時・読み込みではこのあと聞き直さない。
    func declineAdult() {
        MatchingUI.shared.askingAge = nil
        MatchingUI.shared.declinedAge = true
        toast = L10n.tr("18 歳未満の方は「つながりたい」を使えません")
    }

    /// サーバーがプロフィールの投稿を断った(400 age_unconfirmed・`SyncEvent.ageUnconfirmed`)。同意したら投稿し直す。
    func profilePostNeedsAge(entity: String, data: [String: JSONValue]) {
        askAge(.post(entity: entity, data: data))
    }

    // MARK: 内部

    /// つながった: 連絡先(match "matched")が見えるようになるので読み直す。
    private func connected() async {
        toast = L10n.tr("つながりました。連絡先が見られます")
        await reloadMatchedRecords()
    }

    private func reloadMatchedRecords() async {
        let ids = Set(dsl.entities.filter(\.isMatchMatched).map(\.id))
        if !ids.isEmpty { await load(entities: ids, force: true) }
    }

    /// 失敗: 一覧は MatchingStore が元に戻している。理由を出す。サーバーが確認を求めたら聞き直す。
    private func matchingFailed(_ error: Error, retry: PendingInterest?) {
        if Self.matchingReason(error) == MatchingReason.ageUnconfirmed {
            if let retry { askAge(retry) } else { MatchingUI.shared.ageGate.reset() }
            return
        }
        toast = Self.matchingErrorText(error)
    }

    /// 確認を出す。開いた時・読み込みは、出している最中と「18 歳未満」と答えた後は出さない。
    private func askAge(_ pending: PendingInterest) {
        let ui = MatchingUI.shared
        ui.ageGate.reset() // サーバーに申告が無い: 端末の確認と送った記録を消して聞き直す
        if case .refresh(let boxes) = pending {
            if ui.declinedAge { return }
            switch ui.askingAge {
            case .refresh(let asked)?: ui.askingAge = .refresh(asked.union(boxes)) // 同意したら全部読み直す
            case nil: ui.askingAge = pending
            default: break // 押した操作の確認を出している
            }
            return
        }
        ui.askingAge = pending
    }

    /// 送る・承認に成功した(`ageConfirmed: true` でサーバーも申告を記録した)。
    private func ageRecorded() {
        guard let me = currentUserId, !me.isEmpty else { return }
        MatchingUI.shared.ageGate.markSent(uid: me)
    }

    static func matchingReason(_ error: Error) -> String? {
        if case .server(_, _, _, let details)? = error as? APIError, case .string(let r)? = details["reason"] { return r }
        return nil
    }

    static func matchingErrorText(_ error: Error) -> String {
        if matchingReason(error) == MatchingReason.blocked { return L10n.tr("ブロックを解除してから送れます") }
        if (error as? APIError)?.field == "fromRecordId" { return L10n.tr("プロフィールの送信が終わってから送れます") }
        switch FailureKind.classify(error) {
        case .offline: return L10n.tr("オフラインのあいだは送れません")
        case .cancelled: return L10n.tr("うまくいきませんでした。もう一度お試しください")
        default: break
        }
        switch (error as? APIError)?.status {
        case 429: return L10n.tr("今日はこれ以上送れません。明日またお試しください")
        case 403: return L10n.tr("このアプリでは今は仲間探しを使えません")
        case 404: return L10n.tr("相手が見つかりませんでした。一覧を更新してください")
        default: return L10n.tr("うまくいきませんでした。もう一度お試しください")
        }
    }
}

/// 18 歳以上の確認のシート。`onModal` は ModerationDialogs と同じ(DSL のシートの上か下か)。
struct MatchingSheets: ViewModifier {
    var onModal = false
    @Environment(AppRuntime.self) private var rt
    @State private var ui = MatchingUI.shared

    func body(content: Content) -> some View {
        content
        .task(id: "\(rt.document.appId)/\(rt.document.version)") {
            if !onModal { await rt.prepareMatching() } // 仲間探しのアプリを開いた: 端末の申告をサーバーへ・無ければ確認
        }
        .sheet(item: Binding(
            get: { onModal == (rt.sheet != nil || rt.fullScreen != nil) ? ui.askingAge : nil },
            set: { if $0 == nil, ui.askingAge != nil { ui.askingAge = nil } }
        )) { pending in
            AdultConfirmSheet(
                confirm: { Task { await rt.confirmAdult(pending) } },
                decline: { rt.declineAdult() }
            )
            .presentationDetents([.medium, .large])
        }
    }
}

/// 「18 歳以上ですか?」(#507)。仲間探しのアプリを開いた時(端末にもサーバーにも申告が無い)・初回の「つながりたい」・承認の前と、
/// サーバーが申告を求めた(400 age_unconfirmed)時に出す。
struct AdultConfirmSheet: View {
    let confirm: () -> Void
    let decline: () -> Void

    var body: some View {
        ScrollView {
            VStack(spacing: RenderTheme.Spacing.md) {
                Image(systemName: "person.badge.shield.checkmark")
                    .font(.system(size: 44))
                    .foregroundStyle(RenderTheme.Colors.brandA)
                    .accessibilityHidden(true)
                Text("18 歳以上ですか?")
                    .font(.title2.weight(.bold))
                    .multilineTextAlignment(.center)
                Text("仲間探しでは、知らない人と「つながりたい」を送り合い、お互いに承認すると連絡先が見えるようになります。18 歳以上の方だけが使えます。")
                    .font(.body)
                    .multilineTextAlignment(.center)
                Text("年齢や生年月日は保存しません。18 歳以上と答えたことだけを、あなたのアカウントに記録します。")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                VStack(spacing: RenderTheme.Spacing.sm) {
                    Button(action: confirm) {
                        Text("18 歳以上です").font(RenderTheme.Fonts.button).frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(RenderTheme.Colors.brandA)
                    .controlSize(.large)
                    .accessibilityIdentifier("matching.adult.confirm")
                    Button(action: decline) {
                        Text("18 歳未満です").font(RenderTheme.Fonts.button).frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.bordered)
                    .controlSize(.large)
                    .accessibilityIdentifier("matching.adult.decline")
                }
                .padding(.top, RenderTheme.Spacing.sm)
            }
            .padding(RenderTheme.Spacing.lg)
        }
    }
}
#endif
