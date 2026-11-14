import SwiftUI
import MetamonDSL

/// フォーム内の入力部品が束縛する field の型で描画を差し替える(#101)。部品の種類(numberField / picker / textField …)に依らず:
/// - computed field → 読み取り専用の行(ドラフトでライブ更新)
/// - type=ref field → 参照先レコードの Picker
/// - type=refs field → 参照先レコードの複数選択(#231)
/// - type=member field → 参加者の Picker(#272)
/// - type=members field → 参加者の複数選択(#271)
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
            } else if field.type == .refs {
                RefsPickerRow(c: c, field: field, form: form)
            } else if field.type == .member {
                MemberPickerRow(c: c, field: field, form: form)
            } else if field.type == .members {
                MembersPickerRow(c: c, field: field, form: form)
            }
        }
    }

    @MainActor
    static func applies(to c: Component, ctx: RenderContext) -> Bool {
        guard let form = ctx.form, inputTypes.contains(c.type), let fid = c.field, let field = form.entity.field(fid) else { return false }
        return field.isComputed || field.type == .ref || field.type == .refs || field.type == .member || field.type == .members
    }
}

/// computed field の読み取り専用行。行コンテキスト = 入力中のドラフト(金額を打つと「1人あたり」が追従)。
struct ComputedFieldRow: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let field: Field
    let form: FormDraft

    /// 計算結果と表示の文字。適合テストの描画の木(`RenderTree`)も同じ値を使う。
    @MainActor
    static func resolve(field: Field, form: FormDraft, rt: any DSLRuntime) -> (value: JSONValue?, text: String) {
        let row = ExprEvaluator.Row(entity: form.entity, record: Record(id: form.recordId ?? "draft", data: form.payload), memoizable: false)
        let value = field.computed.flatMap { rt.evaluator().evaluate($0, row: row) }
        return (value?.json(for: field), displayText(value?.json(for: field) ?? .null, field: field))
    }

    var body: some View {
        let (value, text) = Self.resolve(field: field, form: form, rt: rt)
        LabeledInput(label: c.label ?? field.name) {
            HStack {
                Text(text)
                    .font(.body.monospacedDigit().weight(.semibold))
                    .contentTransition(.numericText())
                    .animation(.snappy, value: text)
                Spacer(minLength: 0)
            }
            .renderInputChrome()
            .opacity(0.9)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(c.label ?? field.name)
        .accessibilityValue(value == nil ? L("計算できません") : text)
        .accessibilityHint(L("自動で計算されます"))
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
                Label(L("先に\(target?.name ?? L("参照先"))を追加してください"), systemImage: "info.circle")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .renderInputChrome()
            } else {
                // #131: 素の menu だと青文字だけが浮いて他の入力と揃わないので、picker(menu)と同じ器にする
                MenuSelect(icon: "link", label: c.label ?? field.name, selection: form.string(field.id),
                           options: [("", L("選択してください"))] + options.map { ($0.id, rt.refTitle($0.id, entity: field.refEntity) ?? $0.id) })
            }
        }
    }
}

/// type=member field の Picker(#272)。共有データの参加者を表示名で選ぶ。保存値は uid。
/// 参加者を読めない時(読む前・参加していない・審査用の版)は選べない旨を出す。今の値は一覧に無くても残す(抜けた人)。
struct MemberPickerRow: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let field: Field
    let form: FormDraft

    var body: some View {
        let members = rt.membersHost?.appMembers ?? []
        let selection = form.string(field.id)
        LabeledInput(label: c.label ?? field.name) {
            if members.isEmpty {
                Label(L("参加している人を読み込めていません"), systemImage: "person.2")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .renderInputChrome()
            } else {
                let current = selection.wrappedValue
                let listed: [(value: String, title: String)] = members.map { (value: $0.uid, title: rt.memberName(.string($0.uid))) }
                let known = current.isEmpty || members.contains(where: { $0.uid == current })
                let kept: [(value: String, title: String)] = known ? [] : [(value: current, title: rt.memberName(.string(current)))]
                MenuSelect(icon: "person", label: c.label ?? field.name, selection: selection,
                           options: [(value: "", title: L("選択してください"))] + listed + kept)
            }
        }
    }
}

/// type=members field の複数選択(#271)。参加者を表示名のチップで ON/OFF。保存値は選んだ uid の配列。
/// 宛先に使うので自分は並べない。今の値に一覧に無い人(抜けた人)がいても値は残す。
struct MembersPickerRow: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let field: Field
    let form: FormDraft

    var body: some View {
        let me = rt.currentUserId
        let members = (rt.membersHost?.appMembers ?? []).filter { $0.uid != me }
        let selected = Set(Aggregator.refIds(form.values[field.id] ?? .null))
        LabeledInput(label: c.label ?? field.name) {
            if members.isEmpty {
                Label(L("参加している人を読み込めていません"), systemImage: "person.2")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .renderInputChrome()
            } else {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: RenderTheme.Spacing.sm) {
                        ForEach(members) { m in
                            let title = rt.memberName(.string(m.uid))
                            let on = selected.contains(m.uid)
                            Button { toggle(m.uid, in: members) } label: {
                                Label(title, systemImage: on ? "checkmark.circle.fill" : "circle")
                                    .font(.subheadline.weight(on ? .semibold : .regular))
                                    .padding(.horizontal, 12)
                                    .padding(.vertical, 8)
                                    .background(Capsule().fill(on ? RenderTheme.Colors.brandA.opacity(0.18) : Color.secondary.opacity(0.1)))
                                    .foregroundStyle(on ? RenderTheme.Colors.brandA : .primary)
                            }
                            .buttonStyle(.plain)
                            .accessibilityIdentifier("members.\(field.id).\(m.uid)")
                            .accessibilityAddTraits(on ? .isSelected : [])
                        }
                    }
                }
                .renderInputChrome()
            }
        }
    }

    /// 並びは参加者の順に保つ。一覧に無い uid(抜けた人)は後ろに残す。
    private func toggle(_ uid: String, in members: [AppMember]) {
        let current = Aggregator.refIds(form.values[field.id] ?? .null)
        var ids = Set(current)
        if ids.contains(uid) { ids.remove(uid) } else { ids.insert(uid) }
        let listed = members.map(\.uid)
        let ordered = listed.filter { ids.contains($0) } + current.filter { ids.contains($0) && !listed.contains($0) }
        form.values[field.id] = .array(ordered.map { .string($0) })
    }
}

/// type=refs field の複数選択(#231)。参照先レコードをチップで ON/OFF。保存値は選んだ id の配列、未選択 = 全員。
struct RefsPickerRow: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let field: Field
    let form: FormDraft

    var body: some View {
        let target = field.refEntity.flatMap { rt.dsl.entity($0) }
        let options = field.refEntity.map { rt.records(for: $0) } ?? []
        let selected = Set(Aggregator.refIds(form.values[field.id] ?? .null))
        LabeledInput(label: c.label ?? field.name) {
            if options.isEmpty {
                Label(L("先に\(target?.name ?? L("参照先"))を追加してください"), systemImage: "info.circle")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .renderInputChrome()
            } else {
                VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: RenderTheme.Spacing.sm) {
                            ForEach(options, id: \.id) { r in
                                let title = rt.refTitle(r.id, entity: field.refEntity) ?? r.id
                                let on = selected.contains(r.id)
                                Button { toggle(r.id, in: options) } label: {
                                    Label(title, systemImage: on ? "checkmark.circle.fill" : "circle")
                                        .font(.subheadline.weight(on ? .semibold : .regular))
                                        .padding(.horizontal, 12)
                                        .padding(.vertical, 8)
                                        .background(Capsule().fill(on ? RenderTheme.Colors.brandA.opacity(0.18) : Color.secondary.opacity(0.1)))
                                        .foregroundStyle(on ? RenderTheme.Colors.brandA : .primary)
                                }
                                .buttonStyle(.plain)
                                .accessibilityIdentifier("refs.\(field.id).\(title)")
                                .accessibilityAddTraits(on ? .isSelected : [])
                            }
                        }
                    }
                    Text(selected.isEmpty ? L("未選択 = 全員で割ります") : L("\(selected.count)人で割ります"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                .renderInputChrome()
            }
        }
    }

    /// 並びは参照先の順に保つ(選んだ順にしない)。
    private func toggle(_ id: String, in options: [Record]) {
        var ids = Set(Aggregator.refIds(form.values[field.id] ?? .null))
        if ids.contains(id) { ids.remove(id) } else { ids.insert(id) }
        let ordered = options.map(\.id).filter { ids.contains($0) }
        form.values[field.id] = .array(ordered.map { .string($0) })
    }
}

/// menu 型の選択入力(#131)。入力と同じ器の全幅ボタン「アイコン + 選択中の値 … chevron」をタップでメニュー。
/// 素の `Picker(.menu)` は値の横にシステムの ⌃⌄ を出すので、器の chevron と二重になっていた。
struct MenuSelect: View {
    let icon: String
    let label: String
    @Binding var selection: String
    /// (保存値, 表示名)
    let options: [(value: String, title: String)]

    private var current: String? { options.first { $0.value == selection }?.title }

    var body: some View {
        Menu {
            Picker(label, selection: $selection) {
                ForEach(options, id: \.value) { Text($0.title).tag($0.value) }
            }
            .pickerStyle(.inline)
        } label: {
            HStack(spacing: RenderTheme.Spacing.sm + 2) {
                InputIcon(name: icon)
                Text(current ?? L("選択してください"))
                    .font(RenderTheme.Fonts.inputValue)
                    .foregroundStyle(current == nil || selection.isEmpty ? .secondary : .primary)
                    .lineLimit(1)
                Spacer(minLength: 0)
                Image(systemName: "chevron.up.chevron.down")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(RenderTheme.Colors.brandA)
                    .accessibilityHidden(true)
            }
            .contentShape(Rectangle())
            .renderInputChrome()
        }
        .accessibilityLabel(label)
        .accessibilityValue(current ?? "")
    }
}
