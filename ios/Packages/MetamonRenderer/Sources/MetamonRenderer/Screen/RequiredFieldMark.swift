import SwiftUI
import MetamonDSL

/// 保存で弾かれた空の必須欄に、赤い枠と一言を付ける(M10)。
/// 部品の種類に依らないよう `DSLView` が 1 箇所で包む。一言は VoiceOver では項目名付きで読み、最初の欄へフォーカスが移る。
struct RequiredFieldMark: ViewModifier {
    @Environment(\.dslRuntime) private var rt
    let component: Component
    let ctx: RenderContext
    @AccessibilityFocusState private var voiceOverFocused: Bool

    /// `FormDraft` に値を書く部品。`field` が集計など表示用の部品(stat / gauge …)には印を付けない。
    static let inputTypes: Set<ComponentType> = FormFieldOverride.inputTypes.union([
        .audioRecorder, .colorPicker, .drawingField, .locationButton, .locationPicker, .photoField, .qrScanner, .transcribeField, .videoField,
    ])

    /// `c` が入力する、このフォームの field。
    static func boundField(of c: Component, in form: FormDraft) -> String? {
        guard inputTypes.contains(c.type), c.entity == nil || c.entity == form.entity.id else { return nil }
        return c.field
    }

    static func anchor(_ field: String) -> String { "required:\(field)" }

    func body(content: Content) -> some View {
        if Self.inputTypes.contains(component.type) {
            let field = ctx.form.flatMap { Self.boundField(of: component, in: $0) }
            let flagged = field.flatMap { f in ctx.form.map { $0.isFlagged(f) && !$0.hides(component, currentUid: rt.currentUserId) } } ?? false
            VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
                content.overlay {
                    if flagged {
                        RoundedRectangle(cornerRadius: RenderTheme.Radius.control, style: .continuous)
                            .strokeBorder(RenderTheme.Colors.negative, lineWidth: 1.5)
                            .padding(-RenderTheme.Spacing.xs)
                            .allowsHitTesting(false)
                            .accessibilityHidden(true)
                    }
                }
                if flagged, let field {
                    Text(L("入力してください"))
                        .font(.caption)
                        .foregroundStyle(RenderTheme.Colors.negative)
                        .accessibilityLabel(L("\(ctx.form?.entity.field(field)?.name ?? "") を入力してください"))
                        .accessibilityFocused($voiceOverFocused)
                        .accessibilityIdentifier("form.missing.\(field)")
                }
            }
            .id(component.field.map(Self.anchor))
            .onChange(of: ctx.form?.attentionField) { _, target in
                if target != nil, target == field { voiceOverFocused = true }
            }
        } else {
            content
        }
    }
}

/// 保存で弾かれたら、画面で最初の空の必須欄へスクロールする(M10)。`GridScreenLayout` の `ScrollViewReader` の中に付ける。
struct RequiredFieldScroller: ViewModifier {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    let screen: Screen
    let form: FormDraft?
    let proxy: ScrollViewProxy

    /// キーボードが閉じて可視領域が落ち着くまで(保存はキーボードを閉じてから走る)
    private static let settle: Duration = .milliseconds(300)

    func body(content: Content) -> some View {
        content.onChange(of: form?.flagCount) { _, count in
            guard let form, (count ?? 0) > 0 else { return }
            form.attentionField = nil // 同じ欄でも、押すたびに VoiceOver のフォーカスを戻す
            guard let field = Self.firstFlagged(in: screen, form: form, currentUid: rt.currentUserId) else { return }
            Task {
                try? await Task.sleep(for: Self.settle)
                withAnimation(reduceMotion ? nil : .easeOut(duration: 0.25)) {
                    proxy.scrollTo(RequiredFieldMark.anchor(field), anchor: .center)
                }
                form.attentionField = field
            }
        }
    }

    /// 画面の並び順で最初の、印の付いた欄(area の行 → 列、area 無しの部品はその後。入力部品が画面に無い field と隠す field は除く)。
    @MainActor
    static func firstFlagged(in screen: Screen, form: FormDraft, currentUid: String?) -> String? {
        let placement = screen.gridPlacement
        let top = FormFlowLayout.bands(placement.placed).flatMap { $0.items.map(\.component) } + placement.stacked
        for c in top.flatMap(flattened) {
            if let field = RequiredFieldMark.boundField(of: c, in: form), form.isFlagged(field), !form.hides(c, currentUid: currentUid) {
                return field
            }
        }
        return nil
    }

    private static func flattened(_ c: Component) -> [Component] {
        [c] + c.children.flatMap(flattened)
    }
}
