import Foundation
import Observation

/// 初回のチュートリアル(#274)。カードの中身と「見たか」の保存。
/// 表示の順序は 同意(#246)→ チュートリアル → 音声モデルのダウンロード確認。
enum FirstRunTutorial {
    struct Card: Equatable, Identifiable {
        var id: String
        var symbol: String
        var pose: CraneMascot.Pose
        var title: String
        var body: String
        /// 補足(小さい字)。カード 1 は音声モデルの準備中も文字で作れること(#275)
        var note: String?
    }

    /// シェア・探す が使える版か。審査用の版(APP_STORE_REVIEW・#245)では外れるので案内しない。
    static var sharingAvailable: Bool {
        #if APP_STORE_REVIEW
        false
        #else
        true
        #endif
    }

    static func cards(sharingAvailable: Bool = sharingAvailable, voiceModelMegabytes: Int) -> [Card] {
        [
            Card(id: "create", symbol: "mic.fill", pose: .happy,
                 title: L10n.pick(L10n.tr("話すと、アプリができる"), review: L10n.tr("話すと、記録帳が用意できる")),
                 body: L10n.pick(L10n.tr("マイクをタップして「家計簿が欲しい」のように話すだけ。文字で入力しても作れます。"), review: L10n.tr("マイクをタップして「家計簿をつけたい」のように話すだけ。文字で入力しても用意できます。")),
                 note: L10n.tr("声を文字にする準備(初回だけ約\(voiceModelMegabytes)MB)が終わる前でも、文字ならすぐに作れます。")),
            Card(id: "edit", symbol: "wand.and.stars", pose: .idea,
                 title: L10n.tr("話すと、直せる・戻せる"),
                 body: L10n.pick(L10n.tr("できたアプリの画面にあるマイクから「ボタンを大きくして」「メモ欄を足して」と話すと直せます。気に入らなければ「元に戻して」で戻せます。"), review: L10n.tr("記録帳の画面にあるマイクから「ボタンを大きくして」「メモ欄を足して」と話すと直せます。気に入らなければ「元に戻して」で戻せます。"))),
            sharingAvailable
                ? Card(id: "share", symbol: "person.2.fill", pose: .joy,
                       title: L10n.tr("友達と使える・一覧で切り替え"),
                       body: L10n.tr("割り勘や出欠のアプリは、シェアすると友達や家族と一緒に使えます。マイクを長押しすると、作ったアプリの一覧が開きます。"))
                : Card(id: "list", symbol: "square.grid.2x2.fill", pose: .joy,
                       title: L10n.pick(L10n.tr("作ったアプリは一覧で切り替え"), review: L10n.tr("用意した記録帳は一覧で切り替え")),
                       body: L10n.pick(L10n.tr("マイクを長押しすると、作ったアプリの一覧が開きます。「アプリ一覧」と話しても開けます。"), review: L10n.tr("マイクを長押しすると、用意した記録帳の一覧が開きます。"))),
        ]
    }

    /// 最初のアプリができた直後に 1 回だけ出す「こう言うと直せます」の例。
    static var editHintExamples: [String] {
        [L10n.tr("ボタンを大きくして"), L10n.tr("メモ欄を足して"), L10n.tr("元に戻して")]
    }
}

/// チュートリアルと「直し方の例」を見たかを端末に保存する。
@MainActor
@Observable
final class FirstRunTutorialStore {
    nonisolated static let seenKey = "metamon.tutorial.seen"
    nonisolated static let editHintKey = "metamon.tutorial.editHintShown"

    static let shared = FirstRunTutorialStore(defaults: .standard)

    private(set) var hasSeenTutorial: Bool
    private(set) var hasShownEditHint: Bool
    @ObservationIgnored private let defaults: UserDefaults

    init(defaults: UserDefaults) {
        self.defaults = defaults
        hasSeenTutorial = defaults.bool(forKey: Self.seenKey)
        hasShownEditHint = defaults.bool(forKey: Self.editHintKey)
    }

    func markTutorialSeen() {
        defaults.set(true, forKey: Self.seenKey)
        hasSeenTutorial = true
    }

    func markEditHintShown() {
        defaults.set(true, forKey: Self.editHintKey)
        hasShownEditHint = true
    }

    /// 初回の自動表示を出すか。同意の説明(#246)を出し終えてから。
    func shouldAutoPresent(consentPrompted: Bool) -> Bool {
        consentPrompted && !hasSeenTutorial
    }

    /// 最初のアプリができた時に「直し方の例」を出すか(初期画面からの作成の直後・1 回だけ)。
    func shouldShowEditHint(createdFromInitial: Bool) -> Bool {
        createdFromInitial && !hasShownEditHint
    }

    func resetForTesting() {
        defaults.removeObject(forKey: Self.seenKey)
        defaults.removeObject(forKey: Self.editHintKey)
        hasSeenTutorial = false
        hasShownEditHint = false
    }

    /// UI テスト・ヘッドレス E2E 用(DEBUG のみ): `METAMON_TUTORIAL=reset` で初回、`=seen` で見た状態で起動する。
    /// 未指定で `METAMON_AI_CONSENT=granted`(既存の自動テスト)なら見た扱いにして、チュートリアルで画面を塞がない。
    func applyLaunchOverride(environment: [String: String] = ProcessInfo.processInfo.environment) {
        #if DEBUG
        switch environment["METAMON_TUTORIAL"] {
        case "reset":
            resetForTesting()
        case "seen":
            markAllSeen()
        case nil where environment["METAMON_AI_CONSENT"] == "granted":
            markAllSeen()
        default:
            break
        }
        #endif
    }

    private func markAllSeen() {
        markTutorialSeen()
        markEditHintShown()
    }
}
