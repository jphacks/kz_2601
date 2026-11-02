import Foundation
import MetamonDSL

/// 適合テスト(#352)の「描画の木」。画面を描かずに、DSL + レコード → 何が・どこに・どの値で・押すと何が起きるか を JSON にする。
/// 形は `docs/dsl/conformance/README.md`。ピクセル・フォント・色・アイコンの絵は入れない。
/// 値は View と同じ解決の関数(`gridPlacement` / `localGridPlacement` / `FormFlowLayout.bands` / 各 Leaf の `static` な resolve)から取る。
@MainActor
public struct RenderTree {
    let rt: any DSLRuntime
    /// 描ける type(レジストリの登録 ∩ 端末の能力)。外れた既知の type は `unavailable`、知らない type は `unknown`
    let available: Set<String>

    public init(runtime: any DSLRuntime, registry: ComponentRegistry = .standard, components: Set<String>? = nil) {
        rt = runtime
        available = components.map { registry.registeredTypes.intersection($0) } ?? registry.registeredTypes
    }

    // MARK: - 画面

    /// `route` の画面 1 枚。`form` は `ScreenView.makeDraft` で作ったもの(フォーム画面でなければ nil)。
    public func screen(_ route: Route, form: FormDraft?, locals: ScreenLocals? = nil) -> JSONValue {
        guard let screen = rt.dsl.screen(route.screen) else {
            return .object(["route": Self.route(route), "missing": true])
        }
        let ctx = RenderContext(route: route, form: form, locals: locals)
        let placement = screen.gridPlacement
        let isForm = screen.entity != nil && !placement.placed.isEmpty
        // グリッドに置く部品は (行, 列) の順。同じ位置は DSL の順
        let placed = placement.placed.enumerated()
            .sorted { ($0.element.area.row1, $0.element.area.col1, $0.offset) < ($1.element.area.row1, $1.element.area.col1, $1.offset) }
            .map(\.element)
        var bandOf: [String: Int] = [:]
        if isForm {
            for (i, band) in FormFlowLayout.bands(placement.placed).enumerated() {
                for item in band.items { bandOf[item.component.id] = i }
            }
        }
        var nodes: [JSONValue] = placed.map { item in
            var extra: [String: JSONValue] = [:]
            if let b = bandOf[item.component.id] { extra["band"] = .number(Double(b)) }
            return node(item.component, ctx: ctx, cell: item.area, extra: extra)
        }
        nodes += placement.stacked.map { node($0, ctx: ctx, cell: nil, extra: ["stacked": true]) }

        var out: [String: JSONValue] = [
            "id": .string(screen.id),
            "title": .string(screen.title),
            "route": Self.route(route),
            "layout": .string(screen.isVoiceScreen ? "voice" : isForm ? "form" : "grid"),
            "nodes": .array(nodes),
        ]
        let pinned = FormSaveBar.pinned(in: screen)
        out["toolbar"] = .array(DuplicateAddControl.toolbar(of: screen, in: rt.dsl)
            .filter { tb in !pinned.contains { $0.id == tb.id } && ButtonLeaf.isVisible(action: tb.action, form: form) }
            .map { .object(["label": .string($0.label), "action": Self.action($0.action)]) })
        if form?.isReadOnly == true, FormSaveBar.hasSave(in: screen) {
            out["saveBar"] = "readOnly"
        } else if let save = pinned.first {
            out["saveBar"] = .object(["label": .string(save.label), "action": Self.action(save.action)])
        }
        if let form {
            out["form"] = .object([
                "entity": .string(form.entity.id),
                "recordId": form.recordId.map(JSONValue.string) ?? .null,
                "readOnly": .bool(form.isReadOnly),
                "values": .object(form.payload),
            ])
        }
        return .object(out)
    }

    static func route(_ r: Route) -> JSONValue {
        .object(["screen": .string(r.screen), "stack": .string(r.stack), "recordId": r.recordId.map(JSONValue.string) ?? .null])
    }

    // MARK: - 部品

    func node(_ c: Component, ctx: RenderContext, cell: GridArea?, extra: [String: JSONValue] = [:]) -> JSONValue {
        var out = extra
        out["id"] = Self.isGeneratedId(c.id) ? .null : .string(c.id)
        out["type"] = .string(c.rawType)
        if let cell { out["cell"] = Self.cell(cell) }
        let state = state(of: c, ctx: ctx)
        out["state"] = .string(state)
        guard state == "rendered" else { return .object(out) }

        var props = declared(c)
        for (k, v) in resolved(c, ctx: ctx) { props[k] = v }
        if !props.isEmpty { out["props"] = .object(props) }
        let actions = Self.actions(of: c)
        if !actions.isEmpty { out["actions"] = .array(actions) }
        if c.type.isContainer {
            if let local = c.localGridPlacement {
                out["localGrid"] = true
                out["children"] = .array(local.map { node($0.component, ctx: ctx, cell: $0.area) })
            } else {
                out["children"] = .array(c.children.map { node($0, ctx: ctx, cell: nil) })
            }
        }
        return .object(out)
    }

    /// rendered / hidden / needsForm / unavailable / unknown(`DSLView` と各 Leaf の分岐と同じ順)。
    func state(of c: Component, ctx: RenderContext) -> String {
        if let form = ctx.form, form.hides(c, currentUid: rt.currentUserId) { return "hidden" }
        if FormFieldOverride.applies(to: c, ctx: ctx) { return "rendered" }
        guard available.contains(c.rawType) else { return c.type == .unknown ? "unknown" : "unavailable" }
        if c.type == .button, !ButtonLeaf.isVisible(action: c.action, form: ctx.form) { return "hidden" }
        if Self.formBound.contains(c.type), ctx.form == nil || c.field == nil { return "needsForm" }
        return "rendered"
    }

    /// `if let form = ctx.form, let field = c.field` で描き、無ければ `NeedsForm` を出す Leaf。
    static let formBound: Set<ComponentType> = [
        .textField, .numberField, .picker, .toggle, .datePicker, .slider, .stepper, .textEditor, .secureField, .timePicker,
        .rating, .colorPicker, .photoField, .videoField, .audioRecorder, .locationPicker, .locationButton, .qrScanner,
    ]

    /// ラベルを `c.label ?? field.name` で補う入力(`InputRow`)。他の入力(`LabeledInput`)は `c.label` だけ。
    static let labelFallsBackToFieldName: Set<ComponentType> = [.toggle, .datePicker, .colorPicker, .stepper, .timePicker]

    /// DSL に書いてあり、OS が変わっても意味が同じ値(見た目のキー・アイコン・色は入れない)。
    func declared(_ c: Component) -> [String: JSONValue] {
        var p: [String: JSONValue] = [:]
        func put(_ k: String, _ v: String?) { if let v { p[k] = .string(v) } }
        func put(_ k: String, _ v: Double?) { if let v { p[k] = .number(v) } }
        func put(_ k: String, _ v: Int?) { if let v { p[k] = .number(Double(v)) } }
        func put(_ k: String, _ v: Bool?) { if let v { p[k] = .bool(v) } }
        put("title", c.title)
        put("text", c.text)
        put("label", c.label)
        put("message", c.message)
        put("placeholder", c.placeholder)
        put("url", c.url)
        put("field", c.field)
        put("entity", c.entity)
        put("aggregate", c.aggregate)
        put("format", c.format)
        put("groupBy", c.groupBy)
        put("bucket", c.bucket)
        put("kind", c.kind)
        put("mode", c.mode)
        put("metric", c.metric)
        put("period", c.period)
        put("sensor", c.sensor)
        put("task", c.task)
        put("output", c.output)
        put("address", c.address)
        put("unit", c.unit)
        put("min", c.min)
        put("max", c.max)
        put("step", c.step)
        put("target", c.target)
        put("seconds", c.seconds)
        put("limit", c.limit)
        put("expanded", c.expanded)
        return p
    }

    /// 解決した値(計算・書式・束縛の結果)。View と同じ関数を呼ぶ。
    func resolved(_ c: Component, ctx: RenderContext) -> [String: JSONValue] {
        if FormFieldOverride.applies(to: c, ctx: ctx), let form = ctx.form, let fid = c.field, let field = form.entity.field(fid) {
            var p: [String: JSONValue] = ["label": .string(c.label ?? field.name), "value": form.values[fid] ?? .null]
            if field.isComputed {
                p["input"] = "computed"
                p["value"] = .string(ComputedFieldRow.resolve(field: field, form: form, rt: rt).text)
            } else {
                p["input"] = .string(field.type.rawValue)
                if let ref = field.refEntity {
                    p["options"] = .array(rt.records(for: ref).map { r in
                        .object(["id": .string(r.id), "title": .string(rt.refTitle(r.id, entity: ref) ?? r.id)])
                    })
                }
            }
            return p
        }
        if Self.formBound.contains(c.type), let form = ctx.form, let fid = c.field {
            let def = form.entity.field(fid)
            var p: [String: JSONValue] = ["value": form.values[fid] ?? .null]
            let label = Self.labelFallsBackToFieldName.contains(c.type) ? (c.label ?? def?.name ?? (c.type == .timePicker ? "" : fid)) : c.label
            p["label"] = label.map(JSONValue.string) ?? .null
            if c.type == .picker {
                let options = PickerLeaf.options(base: def?.options ?? [], current: (form.values[fid] ?? .null).displayString)
                p["options"] = .array(options.map(JSONValue.string))
                p["style"] = .string(PickerLeaf.style(c, optionCount: options.count))
            }
            return p
        }
        switch c.type {
        case .stat:
            let v = StatLeaf.resolve(c, rt: rt)
            return ["value": .string(v.text), "unavailable": .bool(v.unavailable)]
        case .list:
            let entity = rt.dsl.entity(c.entity ?? "")
            let rows = ListLeaf.rows(c, rt: rt, locals: ctx.locals)
            let toggleField = ListToggle.field(c.item, in: entity)
            let today = DSLClock.now()
            var p: [String: JSONValue] = ["rows": .array(rows.map { r in
                let t = ListRow.texts(item: c.item, record: r, entity: entity, rt: rt)
                var row: [String: JSONValue] = ["recordId": .string(r.id), "title": .string(t.title)]
                if let s = t.subtitle { row["subtitle"] = .string(s) }
                if let s = t.trailing { row["trailing"] = .string(s) }
                if let f = toggleField {
                    row["checked"] = .bool(ListToggle.isOn(r, field: f.id, daily: ListToggle.isDaily(c.item), overrides: [:], today: today))
                }
                return .object(row)
            })]
            if rows.isEmpty { p["emptyText"] = .string(c.emptyText ?? L("まだありません")) }
            return p
        case .table:
            let entity = rt.dsl.entity(c.entity ?? "")
            return [
                "columns": .array((c.columns ?? []).map { .string(TableLeaf.header(for: $0, entity: entity)) }),
                "rows": .array(TableLeaf.cellRows(c, rt: rt).map { .array($0.texts.map(JSONValue.string)) }),
            ]
        case .timeline:
            let entity = rt.dsl.entity(c.entity ?? "")
            let text = { (r: Record, f: String?) -> String in rt.display(r[f ?? ""], field: entity?.field(f ?? "")) }
            return ["rows": .array(TimelineLeaf.rows(c, rt: rt).map { r in
                var row: [String: JSONValue] = ["recordId": .string(r.id), "date": .string(text(r, c.dateField)), "title": .string(text(r, c.titleField))]
                if let sub = c.subtitleField, case let s = text(r, sub), !s.isEmpty { row["subtitle"] = .string(s) }
                return .object(row)
            })]
        case .chart:
            return ["buckets": .array(ChartLeaf.buckets(c, rt: rt).map { .object(["label": .string($0.label), "value": .number($0.value)]) })]
        case .progressBar:
            let raw = ProgressBarLeaf.ratio(c, rt: rt)
            return ["ratio": .number(raw), "percent": .string(ProgressBarLeaf.percentText(raw: raw, tone: c.tone))]
        case .gauge:
            let (value, unit) = GaugeLeaf.value(c, rt: rt)
            var p: [String: JSONValue] = ["value": .number(value)]
            if let unit { p["unit"] = .string(unit) }
            return p
        case .settlement:
            return ["title": .string(c.title ?? L("精算")), "transfers": .array(SettlementLeaf.transfers(c, rt: rt).map {
                .object(["from": .string($0.from), "to": .string($0.to), "amount": .number($0.amount)])
            })]
        case .link:
            let url = c.url.flatMap(URL.init(string:))
            return ["text": .string(c.text ?? url?.absoluteString ?? ""), "opens": .bool(url != nil)]
        case .disclosure:
            return ["title": .string(c.title ?? L("詳細")), "expanded": .bool(c.expanded ?? false)]
        case .grid:
            return ["columns": .number(Double(min(max(c.columnCount ?? 2, 2), 4)))]
        case .searchBar:
            let key = c.field ?? ""
            return ["query": .string(ctx.locals?.values[key] ?? "")]
        case .image:
            let value = c.field.flatMap { ctx.form?.values[$0] } ?? .null
            return ["hasImage": .bool(!value.isNull)]
        default:
            return [:]
        }
    }

    // MARK: - 動き

    static func actions(of c: Component) -> [JSONValue] {
        var out: [JSONValue] = []
        if let a = c.action { out.append(.object(["on": "tap", "action": action(a)])) }
        if let a = c.onItemTap { out.append(.object(["on": "rowTap", "action": action(a)])) }
        if let a = c.onDateTap { out.append(.object(["on": "dateTap", "action": action(a)])) }
        if let a = c.threshold?.action { out.append(.object(["on": "threshold", "action": action(a)])) }
        return out
    }

    /// アクションをデータで(type と行き先・対象)。
    static func action(_ a: Action) -> JSONValue {
        var p: [String: JSONValue] = ["type": .string(a.rawType)]
        if let v = a.screen { p["screen"] = .string(v) }
        if let v = a.presentation { p["presentation"] = .string(v.rawValue) }
        if let v = a.prefillRef { p["prefillRef"] = .string(v) }
        if let v = a.entity { p["entity"] = .string(v) }
        if let v = a.url { p["url"] = .string(v) }
        if let v = a.confirmMessage { p["confirmMessage"] = .string(v) }
        if let v = a.format { p["format"] = .string(v) }
        if let v = a.notificationId { p["notificationId"] = .string(v) }
        return .object(p)
    }

    // MARK: - 補助

    /// 0 始まりの行・列と幅。`GridArea` は行が 1 始まりなので 1 引く。
    static func cell(_ a: GridArea) -> JSONValue {
        .object(["row": .number(Double(a.row1 - 1)), "col": .number(Double(a.col1)),
                 "rowSpan": .number(Double(a.rowSpan)), "colSpan": .number(Double(a.colSpan))])
    }

    /// `id` の無い部品はデコード時に UUID が入る。描画の木では null にする(実行ごとに変わるため)。
    static func isGeneratedId(_ id: String) -> Bool {
        id.count == 36 && UUID(uuidString: id) != nil
    }
}
