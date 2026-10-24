import SwiftUI
import MetamonDSL

/// フォーム内の入力部品が束縛する field の型で描画を差し替える(#101)。部品の種類(numberField / picker / textField …)に依らず:
/// - computed field → 読み取り専用の行(ドラフトでライブ更新)
/// - type=ref field → 参照先レコードの Picker
/// AI がどの入力部品を置いても同じ見た目・挙動になるよう、`DSLView` が 1 箇所で振り分ける。
enum FormFieldOverride {
    static let inputTypes: Set<ComponentType> = [
        .textField, .numberField, .picker, .datePicker, .toggle, .slider, .stepper, .textEditor, .secureField, .timePicker, .rating,
    ]

    @MainActor @ViewBuilder
    static func view(for c: Component, ctx: RenderContext) -> some View {
        if let form = ctx.form, inputTypes.contains(c.type), let fid = c.field, let field = form.entity.field(fid) {
            if field.isComputed {
                ComputedFieldRow(c: c, field: field, form: form)
            } else if field.type == .ref {
                RefPickerRow(c: c, field: field, form: form)
            }
        }
    }

    @MainActor
    static func applies(to c: Component, ctx: RenderContext) -> Bool {
        guard let form = ctx.form, inputTypes.contains(c.type), let fid = c.field, let field = form.entity.field(fid) else { return false }
        return field.isComputed || field.type == .ref
    }
}

/// computed field の読み取り専用行。行コンテキスト = 入力中のドラフト(金額を打つと「1人あたり」が追従)。
struct ComputedFieldRow: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let field: Field
    let form: FormDraft

    var body: some View {
        let row = ExprEvaluator.Row(entity: form.entity, record: Record(id: form.recordId ?? "draft", data: form.payload), memoizable: false)
        let value = field.computed.flatMap { rt.evaluator().evaluate($0, row: row) }
        let text = FieldCoding.display(value?.json(for: field) ?? .null, field: field)
        LabeledInput(label: c.label ?? field.name) {
            HStack {
                Text(text)
                    .font(.body.monospacedDigit().weight(.semibold))
                    .contentTransition(.numericText())
                    .animation(.snappy, value: text)
                Spacer(minLength: 0)
                Image(systemName: "function")
                    .font(.caption)
                    .foregroundStyle(.tertiary)
                    .accessibilityHidden(true)
            }
            .renderInputChrome()
            .opacity(0.9)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(c.label ?? field.name)
        .accessibilityValue(value == nil ? "計算できません" : text)
        .accessibilityHint("自動で計算されます")
    }
}

/// type=ref field の Picker。参照先レコードを見出し(titleField / 最初の string field)で選ぶ。保存値はレコード id。
struct RefPickerRow: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let field: Field
    let form: FormDraft

    var body: some View {
        let target = field.refEntity.flatMap { rt.dsl.entity($0) }
        let options = field.refEntity.map { rt.records(for: $0) } ?? []
        LabeledInput(label: c.label ?? field.name) {
            if options.isEmpty {
                Label("先に\(target?.name ?? "参照先")を追加してください", systemImage: "info.circle")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .renderInputChrome()
            } else {
                // #131: 素の menu だと青文字だけが浮いて他の入力と揃わないので、picker(menu)と同じ器 + chevron にする
                HStack(spacing: RenderTheme.Spacing.sm + 2) {
                    InputIcon(name: "link")
                    Picker(c.label ?? field.name, selection: form.string(field.id)) {
                        Text("選択してください").tag("")
                        ForEach(options) { r in
                            Text(rt.refTitle(r.id, entity: field.refEntity) ?? r.id).tag(r.id)
                        }
                    }
                    .pickerStyle(.menu)
                    .labelsHidden()
                    .tint(.primary)
                    .font(RenderTheme.Fonts.inputValue)
                    Spacer(minLength: 0)
                    Image(systemName: "chevron.up.chevron.down")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(RenderTheme.Colors.brandA)
                        .accessibilityHidden(true)
                }
                .renderInputChrome()
            }
        }
    }
}
