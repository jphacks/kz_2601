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

    static let examples: [Example] = [
        Example(id: "time", symbol: "clock",
                triggerKey: "毎晩 21 時になった時",
                actionKey: "「記録を追加」で、内容を「毎回尋ねる」にしておくと、その場で声で入れられます"),
        Example(id: "arrive", symbol: "location",
                triggerKey: "ジムに着いた時",
                actionKey: "「記録を追加」で、記録先「筋トレ」に「ジム 1回」を入れる"),
        Example(id: "nfc", symbol: "wave.3.right",
                triggerKey: "植木鉢に貼った NFC タグにかざした時",
                actionKey: "「記録を追加」で、記録先「水やり」に「水やり」を入れる"),
        Example(id: "charging", symbol: "battery.100.bolt",
                triggerKey: "寝る前に充電を始めた時",
                actionKey: "「記録を追加」で、記録先「睡眠」に「就寝」を入れる"),
    ]

    /// 作り方の手順(ショートカット App の中での操作)。
    static let stepKeys = [
        "ショートカット App を開き、下の「オートメーション」を選ぶ",
        "右上の「+」を押し、きっかけ(時刻・場所に到着・NFC・充電器)を選ぶ",
        "「すぐに実行」を選ぶ(動かす前に確かめたい時は「確認後に実行」)",
        "アクションの検索で「Tailor It」と入れ、「記録を追加」を選ぶ",
        "記録先と内容を決めて「完了」を押す",
    ]

    static let titleKey = "ショートカットで自動にする"
    static let introKey = "決まった時刻や場所に着いた時などに、Tailor It の「記録を追加」を自動で動かせます。オートメーションはショートカット App で作ります。"
    static let examplesHeaderKey = "きっかけの例"
    static let stepsHeaderKey = "作り方"
    static let otherActionsKey = "「記録を追加」のほかに「話しかける」(音声入力を開く)も、きっかけに合わせて動かせます。"
    static let openButtonKey = "ショートカット App を開く"
    static let openFailedKey = "ショートカット App が見つかりません。App Store から入れ直してください。"
    static let noteKeys = [
        "iPhone がロック中だと、動かす前に確認が出ることがあります。",
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
