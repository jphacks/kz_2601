import SwiftUI
import MetamonDSL

/// `aiButton`(#78): 押すと AI で input(またはフォーム全体 / entity のレコード)を処理し、
/// 結果を output field のドラフトに入れる。保存はユーザーが行う。
/// 端末内 AI(Apple Intelligence)を優先し、使えない端末だけホストのオンライン版(`POST /v1/text`・Pro・同意が要る)に回す。
/// オンライン版を持たないホスト(審査用ビルド・プレビュー)では、使えない時にボタンを無効化して理由を出す。
struct AiButtonLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.scenePhase) private var scenePhase
    let c: Component
    let ctx: RenderContext
    @State private var availability = AIAvailability.current
    @State private var busy = false
    @State private var error: String?
    @State private var done = false
    @State private var usedOnline = false

    private var route: AIGenerateRoute {
        AIGenerateRoute.choose(availability: availability, hostOnline: rt.aiText?.supportsOnlineAIText ?? false, mock: AIGenerateRoute.isMock,
                               long: c.length == "long")
    }

    private var isUnavailable: Bool {
        if case .unavailable = route { return true }
        return false
    }

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
                            Image(systemName: isUnavailable ? "exclamationmark.triangle" : "sparkles")
                        }
                        Text(busy ? "AI が考えています…" : label)
                    }
                    .frame(maxWidth: .infinity)
                }
                .buttonStyle(RenderButtonStyle(kind: .secondary))
                .disabled(busy || isUnavailable)
                .accessibilityHint(hint)
                caption
            }
            .task { refreshAvailability() }
            .onChange(of: scenePhase) { _, phase in
                // 設定で Apple Intelligence を ON にして戻ってきた時に反映する
                if phase == .active { refreshAvailability() }
            }
        } else {
            NeedsForm(c: c)
        }
    }

    private var hint: String {
        switch route {
        case .onDevice: "端末内の AI で処理し、結果を入力欄に入れます"
        case .online: "オンラインの AI で処理し、結果を入力欄に入れます"
        case .unavailable(let reason): reason
        }
    }

    @ViewBuilder private var caption: some View {
        if let error {
            Text(error).font(.caption).foregroundStyle(.red)
        } else if case .unavailable(let reason) = route {
            Text("\(AIAvailability.title): \(reason)").font(.caption).foregroundStyle(.secondary)
        } else if done {
            Text(usedOnline ? "オンラインの AI が入力しました。確認して保存してください" : "端末内の AI が入力しました。確認して保存してください")
                .font(.caption).foregroundStyle(.secondary)
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

    private func refreshAvailability() {
        availability = AIAvailability.current
        rt.aiText?.recordAIButton(.shown(availability.metric)) // #446
    }

    private func run(_ form: FormDraft) async {
        availability = AIAvailability.current
        let route = route
        guard let output = c.output else { return }
        if case .unavailable = route { return }
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
            if route == .onDevice {
                let request = try AIRequestBuilder.build(c, entity: form.entity, values: values, sourceEntity: source, records: records)
                let result = try await OnDeviceAI.run(request)
                rt.aiText?.recordAIButton(.ran(AIButtonMetric.Task(dslTask: c.task), failure: nil))
                withAnimation(RenderTheme.Motion.select) { form.values[output] = .string(result) }
            } else {
                // 端末内 AI が使えない端末: ホストが同意・Pro を確かめてオンライン版を呼ぶ(Pro でなければ課金画面)
                guard let host = rt.aiText else { throw AITextError.unsupported }
                try await AIButtonOnline.fill(c, form: form, values: values, sourceEntity: source, records: records,
                                              host: host, locale: LanguagePreference.shared.code)
            }
            usedOnline = route == .online
            done = true
        } catch {
            // #446: 端末内 AI の失敗だけ数える(入力の文は渡さない)
            if route == .onDevice,
               let reason = (error as? OnDeviceAIError)?.metricReason ?? AIButtonMetric.FailureReason(buildError: error) {
                rt.aiText?.recordAIButton(.ran(AIButtonMetric.Task(dslTask: c.task), failure: reason))
            }
            self.error = error.localizedDescription
        }
    }
}
