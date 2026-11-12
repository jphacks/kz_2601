import SwiftUI
import MetamonDSL
#if os(iOS) && canImport(Translation)
import Translation
#endif

/// 記録の文字の長押しの「翻訳」(#2000)。OS の翻訳のシート(`.translationPresentation`・iOS 18+)を出すだけで、記録は変えない。
/// 出す: 文字の field(string / text)の値を含む文字。出さない: iOS 17・Mac・空・秘密の field(`encrypted`・`FieldCipher`)を含む文字。
/// 審査用の版(APP_STORE_REVIEW)でも出す: OS の機能だけで、外への通信も新しい画面の決まりも無い。
enum RecordTranslation {
    /// OS の翻訳のシートが使えるか(iOS 18+ のみ)
    static var osSupports: Bool {
        #if os(iOS) && canImport(Translation)
        if #available(iOS 18, *) { return true }
        #endif
        return false
    }

    static var isReviewBuild: Bool {
        #if APP_STORE_REVIEW
        true
        #else
        false
        #endif
    }

    /// 訳す意味のある field: 文字(string / text)
    static func isText(_ field: Field) -> Bool { field.type == .string || field.type == .text }

    /// 「翻訳」を出すか。`fields` は `text` を作った field。
    static func offers(text: String, fields: [Field], osSupports: Bool = osSupports, reviewBuild: Bool = isReviewBuild) -> Bool {
        _ = reviewBuild // 審査用の版でも出す(上の説明)
        guard osSupports, !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return false }
        if fields.contains(where: \.encrypted) { return false }
        return fields.contains(where: isText)
    }

    /// 記録の画面の `text` 部品: `{{id}}` で置き換えた field
    static func substitutedFields(template: String, entity: Entity) -> [Field] {
        entity.fields.filter { TextHighlight.substitutes(entity, $0.id) && template.contains("{{\($0.id)}}") }
    }

    /// 一覧の行: title / subtitle の field(trailing は数値・日付が多いので訳さない)
    static func rowFields(item: ListItem?, entity: Entity?) -> [Field] {
        guard let entity else { return [] }
        return [item?.title ?? entity.displayTitleField, item?.subtitle].compactMap { $0.flatMap(entity.field) }
    }
}

/// 長押しのメニューの「翻訳」。`offers` が false なら何も出さない
struct RecordTranslateMenuItem: View {
    let offers: Bool
    let action: () -> Void

    var body: some View {
        if offers {
            Button(L("翻訳"), systemImage: "translate", action: action)
        }
    }
}

extension View {
    /// OS の翻訳のシート(iOS 18+)。iOS 17・Mac では何もしない
    @ViewBuilder
    func recordTranslation(isPresented: Binding<Bool>, text: String) -> some View {
        #if os(iOS) && canImport(Translation)
        if #available(iOS 18, *) {
            translationPresentation(isPresented: isPresented, text: text)
        } else {
            self
        }
        #else
        self
        #endif
    }
}
