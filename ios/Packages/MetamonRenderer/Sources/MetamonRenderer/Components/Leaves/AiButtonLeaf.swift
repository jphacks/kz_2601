import SwiftUI
import MetamonDSL

/// `aiButton`(#78): 押すとオンデバイス AI(Apple Intelligence)で input(またはフォーム全体 / entity のレコード)を処理し、
/// 結果を output field のドラフトに入れる。保存はユーザーが行う。データは端末外に出ない。
/// 非対応端末・Apple Intelligence OFF・モデル未準備の時はボタンを無効化して理由を出す。
struct AiButtonLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.scenePhase) private var scenePhase
    let c: Component
    let ctx: RenderContext
    @State private var availability = AIAvailability.current
    @State private var busy = false
    @State private var error: String?
    @State private var done = false

    var body: some View {
        if c.task == "generate" {
            AiGenerateLeaf(c: c, ctx: ctx) // #417: フォーム不要・結果をボタンの下に出す
        } else if let form = ctx.form {
            VStack(alignment: .leading, spacing: 4) {
                Button {
                    Task { await run(form) }
                } label: {
                    HStack(spacing: RenderTheme.Spacing.sm) {
                        if busy {
                            ProgressView().controlSize(.small)
                        } else {
                            Image(systemName: availability.isAvailable ? "sparkles" : "exclamationmark.triangle")
                        }
                        Text(busy ? "AI が考えています…" : label)
                    }
                    .frame(maxWidth: .infinity)
                }
                .buttonStyle(RenderButtonStyle(kind: .secondary))
                .disabled(busy || !availability.isAvailable)
                .accessibilityHint(availability.isAvailable ? "端末内の AI で処理し、結果を入力欄に入れます" : (availability.message ?? ""))
                caption
            }
            .task { availability = AIAvailability.current }
            .onChange(of: scenePhase) { _, phase in
                // 設定で Apple Intelligence を ON にして戻ってきた時に反映する
                if phase == .active { availability = AIAvailability.current }
            }
        } else {
            NeedsForm(c: c)
        }
    }

    @ViewBuilder private var caption: some View {
        if let message = availability.message {
            Text("\(AIAvailability.title): \(message)").font(.caption).foregroundStyle(.secondary)
        } else if let error {
            Text(error).font(.caption).foregroundStyle(.red)
        } else if done {
            Text("端末内の AI が入力しました。確認して保存してください").font(.caption).foregroundStyle(.secondary)
        }
    }

    private var label: String {
        if let l = c.label, !l.isEmpty { return l }
        switch c.task {
        case "summarize": return "AI で要約"
        case "classify": return "AI で分類"
        case "rewrite": return "AI で書き直す"
        case "extract": return "AI で抜き出す"
        case "suggest": return "AI に提案してもらう"
        default: return "AI で処理"
        }
    }

    private func run(_ form: FormDraft) async {
        availability = AIAvailability.current
        guard availability.isAvailable, let output = c.output else { return }
        busy = true
        error = nil
        done = false
        defer { busy = false }
        do {
            var source: Entity?
            var records: [Record] = []
            if let id = c.entity, let e = rt.dsl.entity(id) {
                await rt.load(entities: [id])
                source = e
                records = HiddenFields.redact(rt.records(for: id), in: e) // 回答者に見せない項目(#262)は AI の出力に混ぜない
            }
            let values = form.showsHiddenFields(currentUid: rt.currentUserId) ? form.values : HiddenFields.redact(form.values, in: form.entity)
            let request = try AIRequestBuilder.build(c, entity: form.entity, values: values, sourceEntity: source, records: records)
            let result = try await OnDeviceAI.run(request)
            withAnimation(RenderTheme.Motion.select) { form.values[output] = .string(result) }
            done = true
        } catch {
            self.error = error.localizedDescription
        }
    }
}
