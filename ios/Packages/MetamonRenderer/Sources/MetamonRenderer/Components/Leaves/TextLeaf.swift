import SwiftUI
import MetamonDSL

/// `text`: style(title|headline|body|caption)と align(leading|center|trailing)。
/// screen.entity の画面では `{{fieldId}}` を開いているレコードの値にする(`TextHighlight.substitutes` の field だけ・他は文字のまま)。
/// `highlight` の語に印(marker: 背景 + 太字 / bold)。印は見た目だけで、VoiceOver はふつうの文として読む。
/// 記録の文字(string / text の値)は長押しで「翻訳」(#2000・iOS 18+・秘密の field は出さない)。
struct TextLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    var ctx: RenderContext? = nil
    @State private var translating = false

    var body: some View {
        let attributed = content
        let text = String(attributed.characters)
        Text(attributed)
            .font(font)
            .multilineTextAlignment(alignment)
            .macTextSelection { text } // Mac: 選んでコピーできる(#941)
            .frame(maxWidth: .infinity, alignment: frameAlignment)
            .modifier(TextTranslateMenu(offers: offersTranslation(text), text: text, isPresented: $translating))
    }

    /// 記録の画面で、文字の field を置き換えた文字だけ(固定の文言は訳さない)
    private func offersTranslation(_ text: String) -> Bool {
        guard RecordTranslation.osSupports, let entity = ctx?.form?.entity else { return false }
        return RecordTranslation.offers(text: text, fields: RecordTranslation.substitutedFields(template: c.text ?? "", entity: entity))
    }

    private var content: AttributedString {
        let form = ctx?.form
        let text = TextContent.resolve(c.text ?? "", form: form) { rt.evaluator() }
        guard let h = c.highlight else { return AttributedString(text) }
        let fieldValue = h.field.flatMap { form?.values[$0]?.displayString }
        return TextHighlightStyle.attributed(text, words: h.resolvedWords(fieldValue: fieldValue), style: h.style)
    }

    private var font: Font {
        switch c.style { case "title": .title.bold(); case "headline": .headline; case "caption": .caption; default: .body }
    }
    private var alignment: TextAlignment {
        switch c.align { case "center": .center; case "trailing": .trailing; default: .leading }
    }
    private var frameAlignment: Alignment {
        switch c.align { case "center": .center; case "trailing": .trailing; default: .leading }
    }
}

/// 「翻訳」を出す時だけ長押しのメニューを付ける(出さない文字は長押しで浮かせない)
private struct TextTranslateMenu: ViewModifier {
    let offers: Bool
    let text: String
    @Binding var isPresented: Bool

    func body(content: Content) -> some View {
        if offers {
            content
                .contextMenu { RecordTranslateMenuItem(offers: true) { isPresented = true } }
                .recordTranslation(isPresented: $isPresented, text: text)
        } else {
            content
        }
    }
}

enum TextContent {
    /// `{{id}}` を置き換える field だけ値にする(hidden の値は決して入れない)。保存値はそのまま、computed は評価。
    @MainActor
    static func resolve(_ text: String, form: FormDraft?, evaluator: () -> ExprEvaluator) -> String {
        guard let form, text.contains("{{") else { return text }
        var s = text
        for f in form.entity.fields where TextHighlight.substitutes(form.entity, f.id) && s.contains("{{\(f.id)}}") {
            let value: String
            if f.computed != nil {
                let row = ExprEvaluator.Row(entity: form.entity, record: Record(id: form.recordId ?? "draft", data: form.values), memoizable: false)
                let v = evaluator().fieldValue(f.id, row: row)
                // datetime は ExprValue.text だと日付だけになる(addMinutes の時刻が消える)→ 表示の書式で時刻まで
                value = f.type == .datetime ? v.map { displayText($0.json(for: f), field: f) } ?? "" : v?.text ?? ""
            } else {
                value = (form.values[f.id] ?? .null).displayString
            }
            s = s.replacingOccurrences(of: "{{\(f.id)}}", with: value)
        }
        return s
    }
}

enum TextHighlightStyle {
    /// 蛍光ペンの色。ライト / ダークのどちらでも文字色(primary)が読める薄さ。
    static let marker = Color.yellow.opacity(0.4)

    static func attributed(_ text: String, words: [String], style: String?) -> AttributedString {
        var out = AttributedString(text)
        for r in TextHighlightMatcher.ranges(in: text, words: words) {
            guard let lo = AttributedString.Index(r.lowerBound, within: out),
                  let hi = AttributedString.Index(r.upperBound, within: out) else { continue }
            out[lo..<hi].inlinePresentationIntent = .stronglyEmphasized
            if style != "bold" { out[lo..<hi].backgroundColor = marker }
        }
        return out
    }
}
