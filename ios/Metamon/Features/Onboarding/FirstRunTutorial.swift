import Foundation
import Observation

/// 初回のチュートリアル(#274)。カードの中身と「見たか」の保存。
/// 初回の順序は 同意(#246)→ チュートリアル(1 枚)。音声モデルのダウンロード確認は自動では出さない(初期画面の案内とマイク・M14)。
/// β の案内は最初のアプリの後(`BetaWelcomeStore`)。
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

    /// 初回の関門を減らすため 1 枚だけ(UX 点検 H1・2026-11-06 オーナーの決定 U6)。
    /// 「直せる・戻せる」は最初のアプリの直後の `FirstEditHintCard`、一覧は変身一覧・音声シートの上のボタン(H10)に任せる。
    static func cards(voiceModelMegabytes: Int) -> [Card] {
        [
            Card(id: "create", symbol: "mic.fill", pose: .happy,
                 title: L10n.pick(L10n.tr("話すと、アプリができる"), review: L10n.tr("話すと、記録帳が用意できる")),
                 body: L10n.pick(L10n.tr("マイクをタップして「家計簿が欲しい」のように話すだけ。文字で入力しても作れます。"), review: L10n.tr("マイクをタップして「家計簿をつけたい」のように話すだけ。文字で入力しても用意できます。")),
                 note: L10n.tr("声を文字にする準備(初回だけ約\(voiceModelMegabytes)MB)が終わる前でも、文字ならすぐに作れます。")),
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

    /// 保存先を直接消した後(アカウント削除)に、画面の状態を保存先に合わせ直す。
    func reload() {
        hasSeenTutorial = defaults.bool(forKey: Self.seenKey)
        hasShownEditHint = defaults.bool(forKey: Self.editHintKey)
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
