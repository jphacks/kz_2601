import Foundation

/// 「ショートカットで自動にする」(#1995)の案内の中身。今ある Intent(記録を追加・アプリを開く・話しかける)だけを使う。
/// - オートメーションは利用者がショートカット App で作る(アプリからは作れない)。ここは例と手順を見せるだけ
/// - 文はここにだけ置く。キーは日本語(英語は `Localizable.xcstrings`)
/// - 審査用の版でも出すので「アプリを作る」系の言い方をしない(docs/store/wording.md)
enum AutomationGuide {
    /// きっかけ × 動きの例。
    struct Example: Identifiable, Equatable {
        var id: String
        var symbol: String
        var triggerKey: String
        var actionKey: String

        var trigger: String { L10n.tr(String.LocalizationValue(triggerKey)) }
        var action: String { L10n.tr(String.LocalizationValue(actionKey)) }
    }

    /// どれも iPhone を使っている(ロックを解除している)時に起きるきっかけ。「記録を追加」はロック解除が要るため。
    static let examples: [Example] = [
        Example(id: "appOpen", symbol: "apps.iphone",
                triggerKey: "ゲームアプリを開いた時",
                actionKey: "「記録を追加」で、記録先「ゲーム時間」に「開始」を入れる"),
        Example(id: "appClose", symbol: "xmark.app",
                triggerKey: "ゲームアプリを閉じた時",
                actionKey: "「記録を追加」で、記録先「ゲーム時間」に「終了」を入れる"),
        Example(id: "nfc", symbol: "wave.3.right",
                triggerKey: "植木鉢に貼った NFC タグにかざした時",
                actionKey: "「記録を追加」で、記録先「水やり」に「水やり」を入れる"),
        Example(id: "nfcAsk", symbol: "tag",
                triggerKey: "冷蔵庫に貼った NFC タグにかざした時",
                actionKey: "「記録を追加」で記録先を「食事」にし、内容を「毎回尋ねる」にしておくと、その場で入れられます"),
    ]

    /// 作り方の手順(ショートカット App の中での操作)。
    static let stepKeys = [
        "ショートカット App を開き、下の「オートメーション」を選ぶ",
        "右上の「+」を押し、きっかけ(App・NFC など)を選ぶ",
        "「すぐに実行」を選ぶ(動かす前に確かめたい時は「確認後に実行」)",
        "アクションの検索で「Tailor It」と入れ、「記録を追加」を選ぶ",
        "記録先と内容を決めて「完了」を押す",
    ]

    static let titleKey = "ショートカットで自動にする"
    static let introKey = "App を開いた時や NFC タグにかざした時など、iPhone を使っている時に Tailor It の「記録を追加」を自動で動かせます。オートメーションはショートカット App で作ります。"
    static let examplesHeaderKey = "きっかけの例"
    static let stepsHeaderKey = "作り方"
    static let otherActionsKey = "「記録を追加」のほかに「話しかける」(音声入力を開く)も、きっかけに合わせて動かせます。"
    static let openButtonKey = "ショートカット App を開く"
    static let openFailedKey = "ショートカット App が見つかりません。App Store から入れ直してください。"
    static let noteKeys = [
        "「記録を追加」で記録するには、iPhone のロックを解除している必要があります。ロック中に起きるきっかけ(夜の決まった時刻など)ではなく、iPhone を使っている時に起きるきっかけを選んでください。",
        "先に Tailor It を一度開いておいてください。開いたことが無いと記録できません。",
    ]

    /// ショートカット App を開く URL。
    static let shortcutsURL = URL(string: "shortcuts://")!

    static var title: String { L10n.tr(String.LocalizationValue(titleKey)) }
    static var intro: String { L10n.tr(String.LocalizationValue(introKey)) }
    static var steps: [String] { stepKeys.map { L10n.tr(String.LocalizationValue($0)) } }
    static var notes: [String] { noteKeys.map { L10n.tr(String.LocalizationValue($0)) } }

    /// 文言のキーすべて(英訳・言い回しのテスト用)。
    static var allKeys: [String] {
        [titleKey, introKey, examplesHeaderKey, stepsHeaderKey, otherActionsKey, openButtonKey, openFailedKey]
            + examples.flatMap { [$0.triggerKey, $0.actionKey] } + stepKeys + noteKeys
    }
}
