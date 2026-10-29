import SwiftUI
import MetamonDSL

/// `aiButton` の `task: "generate"`(#417): 押すと決まった指示で AI が書き、結果を形(1 行 / 箇条書き / 項目つき)でボタンの下に出す。
/// output があればフォームのドラフトにも入れる(保存は利用者)。端末内 AI を優先し、無ければホストのオンライン版(Pro)。
struct AiGenerateLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.scenePhase) private var scenePhase
    let c: Component
    let ctx: RenderContext
    @State private var availability = AIAvailability.current
    @State private var busy = false
    @State private var result: AITextResult?
    @State private var usedOnline = false
    @State private var error: String?

    private var route: AIGenerateRoute {
        AIGenerateRoute.choose(availability: availability, hostOnline: rt.supportsOnlineAIText, mock: AIGenerateRoute.isMock)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            Button {
                Task { await run() }
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
            .accessibilityIdentifier("aiGenerate.\(c.id)")
            if let result {
                AITextResultView(result: result)
                    .transition(.opacity)
            }
            caption
        }
        .task { availability = AIAvailability.current }
        .onChange(of: scenePhase) { _, phase in
            if phase == .active { availability = AIAvailability.current }
        }
    }

    private var isUnavailable: Bool {
        if case .unavailable = route { return true }
        return false
    }

    private var label: String {
        if let l = c.label, !l.isEmpty { return l }
        return result == nil ? "AI に書いてもらう" : "もう一度"
    }

    @ViewBuilder private var caption: some View {
        if let error {
            Text(error).font(.caption).foregroundStyle(.red)
        } else if case .unavailable(let reason) = route {
            Text("\(AIAvailability.title): \(reason)").font(.caption).foregroundStyle(.secondary)
        } else if result != nil, c.output != nil, ctx.form != nil {
            Text("入力欄にも入れました。確認して保存してください").font(.caption).foregroundStyle(.secondary)
        } else if result != nil, usedOnline {
            Text("オンラインの AI が書きました").font(.caption).foregroundStyle(.secondary)
        }
    }

    private func run() async {
        availability = AIAvailability.current
        let route = route
        if case .unavailable = route { return }
        busy = true
        error = nil
        defer { busy = false }
        do {
            let form = ctx.form
            var source: Entity?
            var records: [Record] = []
            if let id = c.entity, let e = rt.dsl.entity(id) {
                await rt.load(entities: [id])
                source = e
                records = HiddenFields.redact(rt.records(for: id), in: e) // #262 回答者に見せない項目は渡さない
            }
            var values: [String: JSONValue] = [:]
            if let form {
                values = form.showsHiddenFields(currentUid: rt.currentUserId) ? form.values : HiddenFields.redact(form.values, in: form.entity)
            }
            let request = try AIGenerateBuilder.build(c, form: form?.entity, values: values, sourceEntity: source, records: records,
                                                      locale: LanguagePreference.shared.code)
            let out: AITextResult
            switch route {
            case .onDevice: out = try await OnDeviceAI.generate(request)
            default: out = try await rt.generateAIText(request)
            }
            usedOnline = route == .online
            withAnimation(RenderTheme.Motion.select) {
                result = out
                if let output = c.output, let form { form.values[output] = .string(out.text) }
            }
        } catch {
            self.error = error.localizedDescription
        }
    }
}

/// 結果の表示。line = 1 文 / list = 箇条書き / fields = 項目名と値。セルに収まらない時だけ中でスクロールする。
struct AITextResultView: View {
    let result: AITextResult

    var body: some View {
        ViewThatFits(in: .vertical) {
            content
            ScrollView { content }.scrollBounceBehavior(.basedOnSize)
        }
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.secondary.opacity(0.08)))
        .textSelection(.enabled)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("aiGenerate.result")
    }

    private var content: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            switch result.shape {
            case .line:
                Text(result.text).font(.body.weight(.medium))
            case .list:
                ForEach(Array((result.items ?? []).enumerated()), id: \.offset) { _, item in
                    HStack(alignment: .firstTextBaseline, spacing: RenderTheme.Spacing.sm) {
                        Text("・").foregroundStyle(.secondary)
                        Text(item)
                    }
                }
            case .fields:
                ForEach(Array((result.fields ?? []).enumerated()), id: \.offset) { _, f in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(f.name).font(.caption).foregroundStyle(.secondary)
                        Text(f.value)
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(RenderTheme.Spacing.md)
    }
}
