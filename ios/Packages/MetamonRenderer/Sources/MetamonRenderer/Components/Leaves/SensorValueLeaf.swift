// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL

/// `sensorValue`(#419): iPhone のセンサーの今の値。画面に出ている間だけ `SensorData.source` を購読する(最大 10 回/秒)。
/// style number(大きな数字)/ gauge(min〜max)/ level(水平器の泡・tilt)。
/// entity + field があれば「今の値を記録」、seconds で「記録を開始/停止」、threshold で警告 + 振動(任意で保存・action)。
struct SensorValueLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.scenePhase) private var scenePhase
    let c: Component
    let ctx: RenderContext

    @State private var availability: SensorAvailability?
    @State private var value: Double?
    @State private var motion: MotionSample?
    @State private var pipeline: SensorPipeline
    @State private var fired = 0
    @State private var savedCount = 0
    @State private var note: String?

    init(c: Component, ctx: RenderContext, initial: SensorFrame? = nil, availability: SensorAvailability? = nil) {
        self.c = c
        self.ctx = ctx
        _pipeline = State(initialValue: SensorPipeline(threshold: c.threshold, canSave: c.entity != nil && c.field != nil))
        _value = State(initialValue: initial?.value)
        _motion = State(initialValue: initial?.motion)
        _availability = State(initialValue: availability)
    }

    private var kind: SensorKind? { c.sensor.flatMap(SensorKind.init(rawValue:)) }
    private var canSave: Bool { c.entity != nil && c.field != nil }

    var body: some View {
        if let kind {
            card(kind)
        } else {
            UnsupportedComponent(label: "sensorValue")
        }
    }

    private func card(_ kind: SensorKind) -> some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            header(kind)
            content(kind)
            if pipeline.isAlerting {
                Label(c.threshold?.message ?? L("しきい値を超えました"), systemImage: "exclamationmark.triangle.fill")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(RenderTheme.Colors.negative)
                    .accessibilityIdentifier("sensorValue.alert")
            }
            if canSave, availability == .available {
                controls(kind)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .renderCard(padding: RenderTheme.Spacing.md, wash: true)
        .sensoryFeedback(.warning, trigger: fired)
        .task(id: scenePhase == .active) {
            guard scenePhase == .active else { return }
            await run(kind)
        }
        .onDisappear { pipeline.stopLog() } // 画面を離れたら一定間隔の記録も止める
    }

    private func header(_ kind: SensorKind) -> some View {
        HStack(spacing: 6) {
            Image(systemName: Self.icon(kind))
                .font(.caption.weight(.bold))
                .foregroundStyle(RenderTheme.Colors.brandA)
                .frame(width: 18, height: 18)
                .background(RenderTheme.Colors.brandSoft, in: RoundedRectangle(cornerRadius: 6, style: .continuous))
                .accessibilityHidden(true)
            Text(c.label ?? "")
                .font(RenderTheme.Fonts.statLabel)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .minimumScaleFactor(0.8)
        }
    }

    @ViewBuilder
    private func content(_ kind: SensorKind) -> some View {
        switch availability {
        case .unavailable:
            Label(L("このセンサーはこの端末では使えません"), systemImage: "sensor")
                .font(.footnote).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("sensorValue.unavailable")
        case .denied:
            Label(L("モーションとフィットネスへのアクセスが許可されていません"), systemImage: "hand.raised")
                .font(.footnote).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        case .available, nil:
            if let value {
                switch c.style {
                case "gauge": gauge(kind, value)
                case "level" where kind == .tilt: level(value)
                default: number(kind, value, sizes: [44, 34, 22])
                }
            } else {
                number(kind, 0, sizes: [34]).redacted(reason: .placeholder)
            }
        }
    }

    private func number(_ kind: SensorKind, _ v: Double, sizes: [CGFloat]) -> some View {
        ViewThatFits(in: [.horizontal, .vertical]) {
            ForEach(sizes, id: \.self) { size in
                HStack(alignment: .firstTextBaseline, spacing: 4) {
                    Text(kind.format(v))
                        .font(.system(size: size, weight: .bold, design: .rounded).monospacedDigit())
                        .contentTransition(.numericText())
                    Text(kind.unit)
                        .font(.system(size: max(size * 0.4, 13), weight: .semibold, design: .rounded))
                        .foregroundStyle(.secondary)
                }
                .lineLimit(1)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("sensorValue.value")
    }

    private func gauge(_ kind: SensorKind, _ v: Double) -> some View {
        let range = Self.range(kind, min: c.min, max: c.max)
        return VStack(alignment: .leading, spacing: 6) {
            number(kind, v, sizes: [34, 22])
            Gauge(value: Swift.min(Swift.max(v, range.lowerBound), range.upperBound), in: range) {
                EmptyView()
            } currentValueLabel: {
                EmptyView()
            } minimumValueLabel: {
                Text(kind.format(range.lowerBound)).font(.caption2)
            } maximumValueLabel: {
                Text(kind.format(range.upperBound)).font(.caption2)
            }
            .gaugeStyle(.linearCapacity)
            .tint(pipeline.isAlerting ? RenderTheme.Colors.negative : RenderTheme.Colors.brandA)
        }
    }

    /// 水平器: 円の中の泡(高い方へ動く)と傾きの角度。1° 未満は水平として緑。
    private func level(_ tilt: Double) -> some View {
        let g = motion?.gravity ?? Vector3(0, 0, -1)
        let b = SensorMath.bubble(gravity: g)
        let flat = tilt < 1
        return HStack(spacing: RenderTheme.Spacing.md) {
            GeometryReader { geo in
                let d = Swift.min(geo.size.width, geo.size.height)
                let r = d / 2 - d * 0.11
                ZStack {
                    Circle().strokeBorder(RenderTheme.Colors.hairline, lineWidth: 2)
                    Circle().strokeBorder(RenderTheme.Colors.hairline, lineWidth: 1).frame(width: d * 0.3, height: d * 0.3)
                    Circle()
                        .fill(flat ? RenderTheme.Colors.positive : (pipeline.isAlerting ? RenderTheme.Colors.negative : RenderTheme.Colors.brandA))
                        .frame(width: d * 0.22, height: d * 0.22)
                        .offset(x: b.x * r, y: b.y * r)
                        .animation(.linear(duration: 0.1), value: b.x)
                        .animation(.linear(duration: 0.1), value: b.y)
                }
                .frame(width: d, height: d)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
            .frame(minWidth: 60, minHeight: 60)
            .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 2) {
                number(.tilt, tilt, sizes: [40, 30, 22])
                Text(flat ? L("水平です") : L("傾いています"))
                    .font(.caption).foregroundStyle(.secondary)
            }
        }
    }

    @ViewBuilder
    private func controls(_ kind: SensorKind) -> some View {
        HStack(spacing: RenderTheme.Spacing.sm) {
            Button(L("今の値を記録"), systemImage: "square.and.arrow.down") {
                if let value { Task { await save(kind, value) } }
            }
            .buttonStyle(RenderButtonStyle(kind: .secondary))
            .disabled(value == nil)
            .accessibilityIdentifier("sensorValue.save")
            if let seconds = c.seconds, seconds > 0 {
                if pipeline.isLogging {
                    Button(L("記録を停止"), systemImage: "stop.fill") { pipeline.stopLog() }
                        .buttonStyle(RenderButtonStyle(kind: .destructive))
                        .accessibilityIdentifier("sensorValue.stop")
                } else {
                    Button(L("記録を開始"), systemImage: "record.circle") {
                        note = nil
                        pipeline.startLog(seconds: seconds, limit: c.limit)
                    }
                    .buttonStyle(RenderButtonStyle(kind: .primary))
                    .accessibilityIdentifier("sensorValue.start")
                }
            }
        }
        if let status = statusText {
            Text(status).font(.caption).foregroundStyle(.secondary).monospacedDigit()
        }
    }

    private var statusText: String? {
        if let log = pipeline.log {
            return L("記録中 \(log.saved)/\(log.limit) 件・\(Int(log.interval)) 秒ごと(画面を開いている間だけ)")
        }
        if let note { return note }
        return savedCount > 0 ? L("\(savedCount) 件を記録しました") : nil
    }

    // MARK: run

    private func run(_ kind: SensorKind) async {
        let source = SensorData.source
        let state = source.availability(kind)
        availability = state
        guard state == .available else { return }
        for await f in source.frames(kind) {
            guard let events = pipeline.ingest(f.value, at: Date()) else { continue }
            value = f.value
            motion = f.motion
            for e in events { await apply(e, kind) }
        }
    }

    private func apply(_ e: SensorPipeline.Event, _ kind: SensorKind) async {
        switch e {
        case .fired:
            fired += 1
            if let action = c.threshold?.action {
                let rt = rt, ctx = ctx
                Task { await rt.run(action, ctx.actionContext()) }
            }
        case .save(let v):
            await save(kind, v)
        case .logFinished(let n):
            note = L("\(n) 件を記録して止めました(上限)")
        }
    }

    private func save(_ kind: SensorKind, _ v: Double) async {
        guard let data = Self.record(kind: kind, value: v, component: c, entity: c.entity.flatMap(rt.dsl.entity), at: Date()),
              let entity = c.entity else { return }
        if await rt.createRecord(entity: entity, data: data) {
            savedCount += 1
        } else {
            note = L("保存できませんでした")
        }
    }

    /// 保存する 1 件: field = 表示と同じ桁の値、dateField = 時刻、他は defaultValue のある field だけ。
    static func record(kind: SensorKind, value: Double, component c: Component, entity: Entity?, at date: Date) -> [String: JSONValue]? {
        guard let field = c.field, let entity, let f = entity.field(field), f.computed == nil else { return nil }
        var data = LocationTrackerLeaf.baseData(entity, skipping: [field, c.dateField].compactMap { $0 })
        let v = kind.rounded(value)
        data[field] = .number(f.type == .integer ? v.rounded() : v)
        if let df = c.dateField { data[df] = .string(FieldCoding.datetimeFormatter.string(from: date)) }
        return data
    }

    static func range(_ kind: SensorKind, min: Double?, max: Double?) -> ClosedRange<Double> {
        let d = kind.defaultRange
        let lo = min ?? d.lowerBound, hi = max ?? d.upperBound
        return hi > lo ? lo...hi : d
    }

    static func icon(_ kind: SensorKind) -> String {
        switch kind {
        case .tilt, .pitch, .roll: "level"
        case .acceleration: "waveform.path.ecg"
        case .rotation: "gyroscope"
        case .pressure: "barometer"
        case .altitude: "mountain.2"
        }
    }
}
#endif
