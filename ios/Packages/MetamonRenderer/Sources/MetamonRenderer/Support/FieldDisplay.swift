import Foundation
import MetamonDSL

extension FieldCoding.Words {
    /// 現在の UI 言語の語(#150: 英語 UI で「はい / いいえ」が残っていた)。
    static var localized: Self { .init(yes: L("はい"), no: L("いいえ"), photo: L("写真"), video: L("動画"), audio: L("録音")) }
}

/// Renderer から値を表示する時の入口。日付の書式と boolean / photo の語を UI 言語に合わせる。
func displayText(_ value: JSONValue, field: Field?) -> String {
    FieldCoding.display(value, field: field, locale: LanguagePreference.shared.locale, words: .localized)
}
