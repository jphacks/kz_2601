import SwiftUI
import Charts
import PhotosUI
import MetamonDSL

// MARK: - Container (vstack / hstack / section)

struct ContainerView: View {
    let c: Component
    let ctx: RenderContext

    private var spacing: CGFloat {
        switch c.spacing { case "none": 0; case "small": 6; case "large": 24; default: 12 }
    }

    var body: some View {
        switch c.type {
        case .hstack:
            HStack(alignment: .top, spacing: spacing) {
                ForEach(c.children) { DSLView(component: $0, ctx: ctx).frame(maxWidth: .infinity) }
            }
        case .section:
            VStack(alignment: .leading, spacing: spacing) {
                if let title = c.title, !title.isEmpty {
                    Text(title).font(.caption).foregroundStyle(.secondary).padding(.horizontal, 4)
                }
                VStack(alignment: .leading, spacing: spacing) {
                    ForEach(c.children) { DSLView(component: $0, ctx: ctx) }
                }
                .padding(16)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 16))
            }
        default:
            VStack(alignment: .leading, spacing: spacing) {
                ForEach(c.children) { DSLView(component: $0, ctx: ctx) }
            }
        }
    }
}

// MARK: - Text / Button

struct TextLeaf: View {
    let c: Component

    var body: some View {
        Text(c.text ?? "")
            .font(font)
            .multilineTextAlignment(alignment)
            .frame(maxWidth: .infinity, alignment: frameAlignment)
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

struct ButtonLeaf: View {
    @Environment(AppRuntime.self) private var rt
    let c: Component
    let ctx: RenderContext

    var body: some View {
        styled(Button {
            guard let action = c.action else { return }
            Task { await rt.run(action, ctx.actionContext()) }
        } label: {
            Group {
                if let icon = c.icon { Label(c.label ?? "", systemImage: icon) } else { Text(c.label ?? "") }
            }
            .frame(maxWidth: c.style == "plain" ? nil : .infinity)
        })
    }

    @ViewBuilder
    private func styled<V: View>(_ button: V) -> some View {
        switch c.style {
        case "secondary": button.buttonStyle(.bordered).controlSize(.large)
        case "destructive": button.buttonStyle(.bordered).controlSize(.large).tint(.red)
        case "plain": button.buttonStyle(.plain)
        default: button.buttonStyle(.borderedProminent).controlSize(.large)
        }
    }
}

// MARK: - Inputs (bind to FormDraft via `field`)

extension FormDraft {
    func value(_ field: String) -> Binding<JSONValue> {
        Binding(get: { self.values[field] ?? .null }, set: { self.values[field] = $0 })
    }

    func string(_ field: String) -> Binding<String> {
        Binding(get: { (self.values[field] ?? .null).displayString }, set: { self.values[field] = .string($0) })
    }

    func number(_ field: String) -> Binding<String> {
        Binding(
            get: { (self.values[field] ?? .null).displayString },
            set: { s in
                let cleaned = s.replacingOccurrences(of: ",", with: "")
                self.values[field] = cleaned.isEmpty ? .null : (Double(cleaned).map { .number($0) } ?? .string(cleaned))
            })
    }

    func bool(_ field: String) -> Binding<Bool> {
        Binding(get: { (self.values[field] ?? .null).truthy }, set: { self.values[field] = .bool($0) })
    }

    func date(_ field: String) -> Binding<Date> {
        let f = entity.field(field)
        return Binding(
            get: { FieldCoding.date(from: self.values[field] ?? .null) ?? Date() },
            set: { d in
                guard let f else { return }
                self.values[field] = .string(FieldCoding.encode(d, for: f))
            })
    }
}

/// フォーム外(screen.entity 無し)に入力が置かれた時のプレースホルダ。
private struct NeedsForm: View {
    let c: Component
    var body: some View { UnsupportedComponent(label: "\(c.rawType) は screen.entity が必要です") }
}

struct TextFieldLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            LabeledInput(label: c.label) {
                TextField(c.placeholder ?? "", text: form.string(field), axis: c.multiline == true ? .vertical : .horizontal)
                    .lineLimit(c.multiline == true ? 3 ... 8 : 1 ... 1)
                    .textFieldStyle(.roundedBorder)
            }
        } else {
            NeedsForm(c: c)
        }
    }
}

struct NumberFieldLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let ftype = form.entity.field(field)?.type ?? .number
            LabeledInput(label: c.label) {
                HStack {
                    TextField(c.placeholder ?? "0", text: form.number(field))
                        .keyboardType(ftype == .number ? .decimalPad : .numberPad)
                        .textFieldStyle(.roundedBorder)
                    if let unit = c.unit ?? form.entity.field(field)?.unit {
                        Text(unit).foregroundStyle(.secondary)
                    }
                }
            }
        } else {
            NeedsForm(c: c)
        }
    }
}

struct DatePickerLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let includeTime = c.includeTime ?? (form.entity.field(field)?.type == .datetime)
            DatePicker(c.label ?? "", selection: form.date(field), displayedComponents: includeTime ? [.date, .hourAndMinute] : [.date])
        } else {
            NeedsForm(c: c)
        }
    }
}

struct PickerLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let options = form.entity.field(field)?.options ?? []
            let picker = Picker(c.label ?? "", selection: form.string(field)) {
                ForEach(options, id: \.self) { Text($0).tag($0) }
            }
            switch c.style ?? (options.count <= 4 ? "segmented" : "menu") {
            case "segmented":
                LabeledInput(label: c.label) { picker.pickerStyle(.segmented) }
            case "wheel":
                picker.pickerStyle(.wheel).frame(height: 120)
            default:
                picker.pickerStyle(.menu)
            }
        } else {
            NeedsForm(c: c)
        }
    }
}

struct ToggleLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            Toggle(c.label ?? "", isOn: form.bool(field))
        } else {
            NeedsForm(c: c)
        }
    }
}

struct LabeledInput<Content: View>: View {
    var label: String?
    @ViewBuilder var content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            if let label, !label.isEmpty {
                Text(label).font(.caption).foregroundStyle(.secondary)
            }
            content
        }
    }
}

// MARK: - Photo (P0 カメラ)

/// 撮影/選択 → 800px 以下・JPEG q0.5 に縮小 → "data:image/jpeg;base64,..." を photo フィールドへ。
/// v1 は PhotosPicker(シミュレータでも動く)。実機の `source: camera` は UIImagePickerController(.camera) に差し替え予定。
struct PhotoFieldLeaf: View {
    let c: Component
    let ctx: RenderContext
    @State private var selection: PhotosPickerItem?

    var body: some View {
        if let form = ctx.form, let field = c.field {
            PhotosPicker(selection: $selection, matching: .images) {
                Label(c.label ?? "写真を撮る / 選ぶ", systemImage: "camera")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
            .controlSize(.large)
            .onChange(of: selection) { _, item in
                guard let item else { return }
                Task {
                    if let data = try? await item.loadTransferable(type: Data.self), let encoded = PhotoCodec.dataURL(from: data) {
                        form.values[field] = .string(encoded)
                    }
                }
            }
        } else {
            NeedsForm(c: c)
        }
    }
}

enum PhotoCodec {
    static func dataURL(from data: Data, maxSide: CGFloat = 800, quality: CGFloat = 0.5) -> String? {
        guard let image = UIImage(data: data) else { return nil }
        let scale = min(1, maxSide / max(image.size.width, image.size.height))
        let size = CGSize(width: image.size.width * scale, height: image.size.height * scale)
        let resized = UIGraphicsImageRenderer(size: size).image { _ in image.draw(in: CGRect(origin: .zero, size: size)) }
        guard let jpeg = resized.jpegData(compressionQuality: quality) else { return nil }
        return "data:image/jpeg;base64," + jpeg.base64EncodedString()
    }

    static func image(from value: JSONValue) -> UIImage? {
        guard let s = value.stringValue, s.hasPrefix("data:"), let comma = s.firstIndex(of: ",") else { return nil }
        return Data(base64Encoded: String(s[s.index(after: comma)...])).flatMap(UIImage.init(data:))
    }
}

struct ImageLeaf: View {
    let c: Component
    let ctx: RenderContext

    private var height: CGFloat {
        switch c.height { case "small": 100; case "large": 260; default: 160 }
    }

    var body: some View {
        let value = c.field.flatMap { ctx.form?.values[$0] } ?? .null
        PhotoView(value: value, height: height)
    }
}

struct PhotoView: View {
    let value: JSONValue
    var height: CGFloat

    var body: some View {
        Group {
            if let img = PhotoCodec.image(from: value) {
                Image(uiImage: img).resizable().scaledToFill()
            } else if let s = value.stringValue, s.hasPrefix("http"), let url = URL(string: s) {
                AsyncImage(url: url) { $0.resizable().scaledToFill() } placeholder: { Rectangle().fill(.quaternary) }
            } else {
                Rectangle().fill(.quaternary).overlay(Image(systemName: "photo").foregroundStyle(.secondary))
            }
        }
        .frame(height: height)
        .frame(maxWidth: .infinity)
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }
}

// MARK: - List / Stat / Chart (entity bound, client-side aggregation)

struct ListLeaf: View {
    @Environment(AppRuntime.self) private var rt
    let c: Component
    let ctx: RenderContext

    var body: some View {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let rows = Aggregator.sorted(Aggregator.apply(c.filter, entity: entity, to: rt.records(for: entityId)), by: c.sort, entity: entity)
        if rows.isEmpty {
            Text(rt.loading.contains(entityId) ? "読み込み中…" : (c.emptyText ?? "まだありません"))
                .foregroundStyle(.secondary)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 24)
        } else {
            LazyVStack(spacing: 8) {
                ForEach(rows) { row in
                    ListRow(item: c.item, record: row, entity: entity)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            guard let action = c.onItemTap else { return }
                            Task { await rt.run(action, ctx.actionContext(recordId: row.id)) }
                        }
                        .contextMenu {
                            if c.swipeToDelete == true {
                                Button("削除", systemImage: "trash", role: .destructive) {
                                    Task { await rt.run(Action.delete(entity: entityId), ctx.actionContext(recordId: row.id)) }
                                }
                            }
                        }
                }
            }
        }
    }
}

extension Action {
    /// list の長押し削除用(DSL 外で iOS が合成する)。
    static func delete(entity: String) -> Action { Action(type: .deleteRecord, entity: entity) }
}

struct ListRow: View {
    let item: ListItem?
    let record: Record
    let entity: Entity?

    private func text(_ fieldId: String?) -> String? {
        guard let fieldId else { return nil }
        return FieldCoding.display(record[fieldId], field: entity?.field(fieldId))
    }

    var body: some View {
        HStack(spacing: 12) {
            if let img = item?.image {
                PhotoView(value: record[img], height: 56).frame(width: 56)
            } else if let icon = item?.icon {
                Image(systemName: icon).foregroundStyle(.tint).frame(width: 28)
            }
            VStack(alignment: .leading, spacing: 2) {
                Text(text(item?.title ?? entity?.titleField) ?? "").font(.headline)
                if let sub = text(item?.subtitle), !sub.isEmpty {
                    Text(sub).font(.caption).foregroundStyle(.secondary)
                }
            }
            Spacer()
            if let trailing = text(item?.trailing), !trailing.isEmpty {
                Text(trailing).font(.body.monospacedDigit())
            }
        }
        .padding(12)
        .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 12))
    }
}

struct StatLeaf: View {
    @Environment(AppRuntime.self) private var rt
    let c: Component

    var body: some View {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let rows = Aggregator.apply(c.filter, entity: entity, to: rt.records(for: entityId))
        let value = Aggregator.aggregate(c.aggregate, field: c.field, over: rows)
        let unit = c.field.flatMap { entity?.field($0)?.unit }
        VStack(alignment: .leading, spacing: 4) {
            Text(c.label ?? "").font(.caption).foregroundStyle(.secondary)
            Text(Aggregator.format(value, as: c.format ?? (c.aggregate == "count" ? "plain" : nil), unit: c.aggregate == "count" ? "件" : unit))
                .font(.title2.bold().monospacedDigit())
                .contentTransition(.numericText())
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 16))
    }
}

struct ChartLeaf: View {
    @Environment(AppRuntime.self) private var rt
    let c: Component

    var body: some View {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let rows = Aggregator.apply(c.filter, entity: entity, to: rt.records(for: entityId))
        let buckets = Aggregator.buckets(c, entity: entity, records: rows)
        VStack(alignment: .leading, spacing: 8) {
            if let title = c.title { Text(title).font(.headline) }
            if buckets.isEmpty {
                Text("データがありません").font(.caption).foregroundStyle(.secondary).frame(maxWidth: .infinity).padding(.vertical, 24)
            } else {
                Chart(buckets) { b in
                    switch c.kind {
                    case "line":
                        LineMark(x: .value("x", b.label), y: .value("y", b.value))
                        PointMark(x: .value("x", b.label), y: .value("y", b.value))
                    case "pie":
                        SectorMark(angle: .value("y", b.value), innerRadius: .ratio(0.5), angularInset: 1)
                            .foregroundStyle(by: .value("x", b.label))
                    default:
                        BarMark(x: .value("x", b.label), y: .value("y", b.value))
                    }
                }
                .frame(height: 180)
            }
        }
        .padding(16)
        .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 16))
    }
}

// MARK: - Misc

struct SpacerLeaf: View {
    let c: Component

    var body: some View {
        switch c.size {
        case "small": Spacer().frame(height: 8)
        case "medium": Spacer().frame(height: 16)
        case "large": Spacer().frame(height: 32)
        default: Spacer(minLength: 0)
        }
    }
}

extension Color {
    /// "#RRGGBB" / "#RRGGBBAA"
    init?(hex: String?) {
        guard let hex else { return nil }
        var s = hex.trimmingCharacters(in: .whitespaces)
        if s.hasPrefix("#") { s.removeFirst() }
        guard s.count == 6 || s.count == 8, let v = UInt64(s, radix: 16) else { return nil }
        let r, g, b, a: Double
        if s.count == 8 {
            r = Double((v >> 24) & 0xFF) / 255; g = Double((v >> 16) & 0xFF) / 255
            b = Double((v >> 8) & 0xFF) / 255; a = Double(v & 0xFF) / 255
        } else {
            r = Double((v >> 16) & 0xFF) / 255; g = Double((v >> 8) & 0xFF) / 255
            b = Double(v & 0xFF) / 255; a = 1
        }
        self = Color(red: r, green: g, blue: b, opacity: a)
    }
}
