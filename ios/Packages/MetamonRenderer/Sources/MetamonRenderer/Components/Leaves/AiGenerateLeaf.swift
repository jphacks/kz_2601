import SwiftUI
import MetamonDSL

/// `aiButton` の `task: "generate"`(#417): 押すと決まった指示で AI が書き、結果を形(1 行 / 箇条書き / 項目つき)でボタンの下に出す。
/// output があればフォームのドラフトにも入れる(保存は利用者)。fields + outputs なら項目ごとに型を直して各 field に入れる(AIOutputs)。list + saveTo なら各行を 1 件の記録として保存できる(AiSaveToList)。端末内 AI を優先し、無ければホストのオンライン版(Pro)。
struct AiGenerateLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.scenePhase) private var scenePhase
    let c: Component
    let ctx: RenderContext
    @State private var availability = AIAvailability.current
    @State private var busy = false
    @State private var result: AITextResult?
    @State private var usedOnline = false
    @State private var notice: AIUnavailableNotice? // #1640: 使えない時に押したら理由を出す
    @State private var error: String?
    @State private var runs = 0
    @State private var filled: [AIOutputs.Fill] = [] // outputs: 今回の項目(読めなかったものも)
    @State private var applied = AIOutputs.Applied() // outputs: 入れた / 入力済みのため入れなかった項目
    @State private var aiValues: [String: JSONValue] = [:] // outputs: 前回 AI が入れた値(そのままなら次は入れ替えてよい)

    private var route: AIGenerateRoute {
        AIGenerateRoute.choose(availability: availability, hostOnline: rt.aiText?.supportsOnlineAIText ?? false, mock: AIGenerateRoute.isMock,
                               long: c.length == "long", photo: c.inputPhoto != nil)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            Button {
                if let n = tapNotice() { notice = n } else { Task { await run() } }
            } label: {
                HStack(spacing: RenderTheme.Spacing.sm) {
                    if busy {
                        ProgressView().controlSize(.small)
                    } else {
                        Image(systemName: isUnavailable ? "exclamationmark.triangle" : "sparkles")
                    }
                    Text(busy ? L("AI が考えています…") : label)
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(RenderButtonStyle(kind: .secondary, dimmed: isUnavailable))
            .disabled(busy) // 使えない時も押せる(押すと理由のアラート・#1640)
            .accessibilityIdentifier("aiGenerate.\(c.id)")
            .accessibilityHint(route.unavailableNotice?.message ?? "")
            if let result, result.shape == .list, let saveTo = c.saveTo {
                AiSaveToList(c: c, saveTo: saveTo, items: result.items ?? []) // 1 項目 = 1 件(押した時だけ保存)
                    .id(runs) // 新しい結果は「保存済み」を持ち越さない
                    .transition(.opacity)
            } else if let result {
                AITextResultView(result: result, long: c.length == "long")
                    .transition(.opacity)
            }
            caption
        }
        .aiUnavailableAlert($notice)
        .task { refreshAvailability() }
        .onChange(of: scenePhase) { _, phase in
            if phase == .active { refreshAvailability() }
        }
    }

    private var isUnavailable: Bool {
        if case .unavailable = route { return true }
        return false
    }

    private var label: String {
        if let l = c.label, !l.isEmpty { return l }
        return result == nil ? L("AI に書いてもらう") : L("もう一度")
    }

    @ViewBuilder private var caption: some View {
        if let error {
            Text(error).font(.caption).foregroundStyle(.red)
        } else if case .unavailable(let reason) = route {
            Text("\(AIAvailability.title): \(reason)").font(.caption).foregroundStyle(.secondary)
        } else if result != nil, !filled.isEmpty {
            outputsCaption
        } else if result != nil, c.output != nil, ctx.form != nil {
            Text(L("入力欄にも入れました。確認して保存してください")).font(.caption).foregroundStyle(.secondary)
        } else if result != nil, usedOnline {
            Text(L("オンラインの AI が書きました")).font(.caption).foregroundStyle(.secondary)
        }
    }

    @ViewBuilder private var outputsCaption: some View {
        let ok = applied.applied.count
        let kept = applied.kept.map(\.fieldName)
        let missed = filled.filter { $0.value == nil }.map(\.fieldName)
        VStack(alignment: .leading, spacing: 2) {
            if ok > 0 { Text(L("\(ok) 項目を入力欄に入れました。確認して保存してください")) }
            if !kept.isEmpty { Text(L("入力済みのため入れなかった: \(kept.joined(separator: "、"))")) }
            if !missed.isEmpty { Text(L("読み取れず空のまま: \(missed.joined(separator: "、"))")) }
        }
        .font(.caption).foregroundStyle(.secondary)
        .accessibilityIdentifier("aiGenerate.outputs")
    }

    /// outputs: 結果の各項目を field の型に直してドラフトに入れる。空の field だけに入れ、利用者が入れた値は上書きしない
    /// (新規で開いたままの初期値と、前回 AI が入れたままの値は入れ替える)。読めない項目は触らない。
    private func fillOutputs(_ out: AITextResult, form: FormDraft) {
        guard out.shape == .fields, let outputs = c.outputs, !outputs.isEmpty else {
            filled = []
            applied = AIOutputs.Applied()
            return
        }
        filled = AIOutputs.fills(out.fields ?? [], outputs: outputs, entity: form.entity)
        var values = form.values
        let previous = aiValues
        applied = AIOutputs.apply(filled, to: &values) { id, cur in
            if previous[id] == cur { return true }
            guard !form.isEditing, let f = form.entity.field(id) else { return false }
            return FieldCoding.normalize(cur, for: f) == form.baseline[id] // 新規で開いたままの初期値
        }
        for f in applied.applied {
            form.values[f.field] = values[f.field]
            aiValues[f.field] = values[f.field]
        }
        let names = applied.applied.map(\.fieldName)
        if !names.isEmpty {
            AccessibilityNotification.Announcement(L("入力欄に入れました: \(names.joined(separator: "、"))")).post()
        }
    }

    /// 押した時点の状態で判定し、使えなければ理由を返す(何もせずに戻らない・#1640)。
    private func tapNotice() -> AIUnavailableNotice? {
        availability = AIAvailability.current
        return route.unavailableNotice
    }

    private func refreshAvailability() {
        availability = AIAvailability.current
        rt.aiText?.recordAIButton(.shown(availability.metric)) // #446
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
                records = try AIInputRecords.select(records, in: e, route: route, uid: rt.currentUserId) // クラウドへは自分の記録だけ
            }
            var values: [String: JSONValue] = [:]
            if let form {
                values = form.showsHiddenFields(currentUid: rt.currentUserId) ? form.values : HiddenFields.redact(form.values, in: form.entity)
            }
            var request = try AIGenerateBuilder.build(c, form: form?.entity, values: values, sourceEntity: source, records: records,
                                                      locale: LanguagePreference.shared.code)
            if form != nil { try await AIPhotoInput.attach(c, values: values, to: &request) } // inputPhoto: オンラインだけ
            try await AIFeedInput.attach(c, screen: rt.dsl.screen(ctx.route.screen), rt: rt, locale: request.locale ?? "ja", to: &request) // inputFeeds: 画面の feed の行
            let out: AITextResult
            switch route {
            case .onDevice: out = try await OnDeviceAI.generate(request)
            default:
                guard let host = rt.aiText else { throw AITextError.unsupported }
                out = try await host.generateAIText(request)
            }
            usedOnline = route == .online
            if route == .onDevice { rt.aiText?.recordAIButton(.ran(.generate, failure: nil)) }
            withAnimation(RenderTheme.Motion.select) {
                result = out
                runs += 1
                if let output = c.output, let form { form.values[output] = .string(out.text) }
                if let form { fillOutputs(out, form: form) }
            }
        } catch {
            // #446: 端末内 AI の失敗だけ数える(オンライン版は #426 の判断材料ではない)
            if route == .onDevice,
               let reason = (error as? OnDeviceAIError)?.metricReason ?? AIButtonMetric.FailureReason(buildError: error) {
                rt.aiText?.recordAIButton(.ran(.generate, failure: reason))
            }
            self.error = error.localizedDescription
        }
    }
}

/// 結果の表示。line = 1 文 / list = 箇条書き / fields = 項目名と値。セルに収まらない時だけ中でスクロールする。
/// long(length long)は段落のある長い文: 本文の太さで出し、選択してコピーできる。
struct AITextResultView: View {
    let result: AITextResult
    var long = false

    var body: some View {
        ViewThatFits(in: .vertical) {
            content
            ScrollView { content }.scrollBounceBehavior(.basedOnSize)
        }
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.secondary.opacity(0.08)))
        .textSelection(.enabled)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier(long ? "aiGenerate.result.long" : "aiGenerate.result")
    }

    private var content: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            switch result.shape {
            case .line:
                Text(result.text).font(long ? .body : .body.weight(.medium))
                    .lineSpacing(long ? 4 : 0)
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
