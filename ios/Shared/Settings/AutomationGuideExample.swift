import Foundation

/// 「ショートカットで自動にする」(#1995・Mac は #3080)の、きっかけ × 動きの例 1 件。
/// 例の中身は iOS(`AutomationGuide`)と Mac(`MacAutomationGuide`)で別。文はキーで持ち、英語は各アプリの `Localizable.xcstrings`。
struct AutomationGuideExample: Identifiable, Equatable {
    var id: String
    var symbol: String
    var triggerKey: String
    var actionKey: String

    var trigger: String { L10n.tr(String.LocalizationValue(triggerKey)) }
    var action: String { L10n.tr(String.LocalizationValue(actionKey)) }
}
