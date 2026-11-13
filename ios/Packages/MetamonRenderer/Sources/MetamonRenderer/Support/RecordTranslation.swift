import SwiftUI
import MetamonDSL
#if canImport(Translation)
import Translation
#endif

/// 記録の文字の長押し(Mac は右クリック・#2273)の「翻訳」(#2000)。OS の翻訳のシート(`.translationPresentation`・iOS 18+ / macOS 15+)に
/// 文字を渡すだけで、記録は変えない。訳す処理(端末内か外か)は OS 側の翻訳の機能と設定による。
/// 出す: 文字の field(string / text)の値を含む文字。
/// 出さない: iOS 17・macOS 14・空・秘密の field(`encrypted`・`FieldCipher`)を含む文字・計算の field(`computed`)を含む文字。
/// 計算の field は式が秘密の field を参照しうる(他の entity・設定値経由も)ので、依存を辿らずまとめて出さない。
enum RecordTranslation {
    /// OS の翻訳のシートが使えるか(iOS 18+・macOS 15+)
    static var osSupports: Bool {
        #if canImport(Translation)
        if #available(iOS 18, macOS 15, *) { return true }
        #endif
        return false
    }

    /// OS と版から使えるかを決める(#2273・判定の単体テスト用。`osSupports` と同じ境目)
    static func osSupports(isMac: Bool, majorVersion: Int) -> Bool {
        majorVersion >= (isMac ? 15 : 18)
    }

    /// 訳す意味のある field: 文字(string / text)
    static func isText(_ field: Field) -> Bool { field.type == .string || field.type == .text }

    /// 「翻訳」を出すか。`fields` は `text` を作った field。
    static func offers(text: String, fields: [Field], osSupports: Bool = osSupports) -> Bool {
        guard osSupports, !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return false }
        if fields.contains(where: { $0.encrypted || $0.isComputed }) { return false }
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
    /// OS の翻訳のシート(iOS 18+・macOS 15+)。iOS 17・macOS 14 では何もしない
    @ViewBuilder
    func recordTranslation(isPresented: Binding<Bool>, text: String) -> some View {
        #if canImport(Translation)
        if #available(iOS 18, macOS 15, *) {
            translationPresentation(isPresented: isPresented, text: text)
        } else {
            self
        }
        #else
        self
        #endif
    }
}
