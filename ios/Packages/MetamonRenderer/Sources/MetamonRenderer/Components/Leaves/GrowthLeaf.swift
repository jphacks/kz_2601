import SwiftUI
import MetamonDSL

/// `growth`: 数値が増えるほど絵が育つカード(tree=木に実がなる / flower=花が咲く / plant=芽が木に育つ)。
/// 数値は `metric: steps`(今日の歩数・`live` で開いている間も増える)・`entity` の集計・`value`(Expr)のどれか 1 つ。
/// `step` ごとに実・花・葉が 1 つ増え(増えた瞬間にポンと出る)、`target` で育ち切る。絵は図形だけで描く(画像を持たない)。
/// 歩数は `live` を省略しても、画面に出ていて前面にいる間はいつも歩数計を購読する(育つのを見るための部品なので)。
/// 前面に戻るたびに今日の歩数を読み直す。表示はその間は減らさない(歩数計の問い合わせの遅れで実が一瞬消えないように)。
struct GrowthLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    let c: Component
    @State private var steps: StepsPhase

    enum StepsPhase: Equatable {
        case loading
        case value(Double)
        case failed(String)
        /// 許可されていない(設定を開く案内を出す)
        case denied
    }

    init(c: Component, steps: StepsPhase = .loading) {
        self.c = c
        _steps = State(initialValue: steps)
    }

    private var kind: GrowthModel.Kind { .init(c.kind) }
    private var isSteps: Bool { c.metric == "steps" }

    var body: some View {
        let target = Self.target(c, rt: rt)
        let raw: Double? = isSteps ? stepsValue : Self.recordValue(c, rt: rt)
        let model = GrowthModel(value: raw ?? 0, target: target ?? 1, step: c.step)
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            header(model)
            GrowthScene(kind: kind, model: model, reduceMotion: reduceMotion)
                .frame(maxWidth: .infinity, minHeight: 96, maxHeight: .infinity)
            footer(model, raw: raw, target: target)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .renderCard(padding: RenderTheme.Spacing.md, wash: true)
        .sensoryFeedback(.increase, trigger: model.items)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(c.label ?? "")
        .accessibilityValue(accessibilityValue(model, raw: raw, target: target))
        .task(id: scenePhase == .active) {
            guard isSteps, scenePhase == .active else { return }
            await loadSteps()
        }
    }

    // MARK: 見出し・数値

    private func header(_ model: GrowthModel) -> some View {
        HStack(spacing: 6) {
            Image(systemName: Self.icon(kind))
                .font(.caption.weight(.bold))
                .foregroundStyle(RenderTheme.Colors.positive)
                .frame(width: 18, height: 18)
                .background(RenderTheme.Colors.positive.opacity(0.14), in: RoundedRectangle(cornerRadius: 6, style: .continuous))
                .accessibilityHidden(true)
            Text(c.label ?? "")
                .font(RenderTheme.Fonts.statLabel)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .minimumScaleFactor(0.8)
            Spacer(minLength: RenderTheme.Spacing.sm)
            Text(model.isComplete ? Self.completeText(kind) : "\(Self.itemName(kind)) \(model.items) / \(model.capacity)")
                .font(.caption.weight(.bold).monospacedDigit())
                .foregroundStyle(model.isComplete ? Color.white : RenderTheme.Colors.positive)
                .padding(.horizontal, 8)
                .padding(.vertical, 3)
                .background(model.isComplete ? AnyShapeStyle(RenderTheme.Colors.positive) : AnyShapeStyle(RenderTheme.Colors.positive.opacity(0.14)), in: Capsule())
                .contentTransition(.numericText())
                .animation(.snappy, value: model.items)
        }
    }

    @ViewBuilder
    private func footer(_ model: GrowthModel, raw: Double?, target: Double?) -> some View {
        if case .failed(let message) = steps, isSteps {
            Label(message, systemImage: "figure.walk")
                .font(.footnote)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        } else if steps == .denied, isSteps {
            deniedGuide
        } else {
            let unit = unitText
            HStack(alignment: .firstTextBaseline, spacing: 4) {
                ScaledValueText(raw.map(Self.numberText) ?? EmptyPlaceholder.unavailableValue, size: 30)
                    .contentTransition(.numericText())
                    .animation(.snappy, value: raw)
                    .redacted(reason: isSteps && steps == .loading ? .placeholder : [])
                Text(target.map { "\(unit) / \(Self.numberText($0)) \(unit)" } ?? unit)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(.secondary)
                    .monospacedDigit()
            }
            .lineLimit(1)
            .minimumScaleFactor(0.6)
            if let next = model.nextAt, raw != nil {
                Text(L("あと \(Self.numberText(max(next - model.value, 0))) \(unit)で次の\(Self.itemName(kind))"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .monospacedDigit()
            }
        }
    }

    @Environment(\.openURL) private var openURL

    /// 歩数の許可が無い時: どこで許可するかと、設定を開くボタン
    private var deniedGuide: some View {
        VStack(alignment: .leading, spacing: 6) {
            Label(Self.deniedMessage, systemImage: "figure.walk")
                .font(.footnote)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            #if os(iOS)
            Button(L("設定を開く")) {
                if let url = URL(string: UIApplication.openSettingsURLString) { openURL(url) }
            }
            .font(.footnote.weight(.semibold))
            .buttonStyle(.bordered)
            .tint(RenderTheme.Colors.positive)
            #endif
        }
    }

    static var deniedMessage: String {
        L("歩数を読む許可がありません。設定の「プライバシーとセキュリティ > モーションとフィットネス」で Tailor It をオンにしてください")
    }

    private var unitText: String {
        if let u = c.unit, !u.isEmpty { return u }
        if isSteps { return L("歩") }
        return c.field.flatMap { rt.dsl.entity(c.entity ?? "")?.field($0)?.unit } ?? ""
    }

    private func accessibilityValue(_ model: GrowthModel, raw: Double?, target: Double?) -> String {
        let v = raw.map(Self.numberText) ?? L("取得できませんでした")
        let t = target.map(Self.numberText) ?? "—"
        return L("\(v) \(unitText)、目標 \(t) \(unitText)、\(Self.itemName(kind)) \(model.items) / \(model.capacity)")
    }

    // MARK: 値(テストと共有)

    /// 育ち切る値。数値の `target`、または `targetExpr`({"setting"})の今の値。0 以下・空は nil。
    @MainActor
    static func target(_ c: Component, rt: any DSLRuntime) -> Double? {
        guard let t = c.target ?? c.targetExpr.flatMap({ rt.evaluator().evaluate($0)?.number }), t.isFinite, t > 0 else { return nil }
        return t
    }

    /// 歩数以外の数値: `value`(Expr)か entity の集計(count|sum・既定 sum)。取れない時は nil。
    @MainActor
    static func recordValue(_ c: Component, rt: any DSLRuntime) -> Double? {
        if let expr = c.valueExpr, c.entity == nil {
            return rt.evaluator().evaluate(expr)?.number
        }
        guard let entityId = c.entity else { return nil }
        let rows = rt.filteredRecords(c)
        if rows.isEmpty && rt.emptyState(for: entityId) == .unavailable { return nil }
        return Aggregator.aggregate(c.aggregate ?? "sum", field: c.field, over: rows)
    }

    /// 「8,000」「1.5」。整数は桁区切り、小数は 1 桁。
    static func numberText(_ v: Double) -> String {
        guard v.isFinite else { return "—" }
        if abs(v - v.rounded()) < 0.05 { return NumericInput.clampedInt(v).formatted(.number.grouping(.automatic)) }
        return v.formatted(.number.precision(.fractionLength(1)))
    }

    static func icon(_ kind: GrowthModel.Kind) -> String {
        switch kind {
        case .tree: "tree.fill"
        case .flower: "camera.macro"
        case .plant: "leaf.fill"
        }
    }

    static func itemName(_ kind: GrowthModel.Kind) -> String {
        switch kind {
        case .tree: L("実")
        case .flower: L("花")
        case .plant: L("葉")
        }
    }

    static func completeText(_ kind: GrowthModel.Kind) -> String {
        switch kind {
        case .tree: L("収穫!")
        case .flower: L("満開!")
        case .plant: L("大きな木!")
        }
    }

    // MARK: 歩数

    private var stepsValue: Double? {
        if case .value(let v) = steps { return v }
        return steps == .loading ? 0 : nil
    }

    /// 開いている間の歩数は減らさない(取り直しの値が歩数計の増分より遅れても戻さない)。日付が変わったら新しい値。
    static func merged(current: Double?, next: Double, sameDay: Bool) -> Double {
        guard sameDay, let current else { return next }
        return max(current, next)
    }

    private func loadSteps() async {
        #if APP_STORE_REVIEW
        steps = .failed(L("この版では歩数を読めません"))
        #else
        let source = HealthData.source
        do {
            let base = try await source.steps(days: 1).last?.value ?? 0
            steps = .value(base)
            // healthStat の live(#3694)と同じ口: 画面に出ている間だけ歩数計の増分を足し続ける(growth は live の有無に関わらず)
            let monitor = HealthLiveStepsMonitor(baseline: base, source: HealthData.liveSource) {
                try await source.steps(days: 1).last?.value ?? 0
            }
            var day = Calendar.current.startOfDay(for: Date())
            monitor.onChange = { value in
                let today = Calendar.current.startOfDay(for: Date())
                steps = .value(Self.merged(current: stepsValue, next: value, sameDay: today == day))
                day = today
            }
            monitor.start()
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(60))
            }
            monitor.stop()
        } catch HealthDataError.denied {
            steps = .denied
        } catch let e as HealthDataError {
            steps = .failed(e.errorDescription ?? "")
        } catch {
            steps = .failed(L("歩数を取得できませんでした"))
        }
        #endif
    }
}

// MARK: - 絵

/// 正方形の中に地面・木(または花・芽)・実を描く。座標は 0〜1(`GrowthModel.slots`)。
struct GrowthScene: View {
    let kind: GrowthModel.Kind
    let model: GrowthModel
    var reduceMotion = false
    @State private var popToken = 0
    @State private var popIndex: Int?

    private static let trunk = Color(red: 0.55, green: 0.38, blue: 0.24)
    private static let fruit = Color(red: 0.92, green: 0.26, blue: 0.24)
    private static let gold = Color(red: 0.98, green: 0.78, blue: 0.22)

    var body: some View {
        GeometryReader { g in
            let side = min(g.size.width, g.size.height)
            ZStack {
                ground(side)
                switch kind {
                case .tree: tree(side)
                case .flower: flowers(side)
                case .plant: plant(side)
                }
                if let i = popIndex, i < slots.count {
                    PopRing(token: popToken, color: kind == .tree ? Self.fruit : RenderTheme.Colors.positive)
                        .frame(width: side * 0.16, height: side * 0.16)
                        .position(point(i, side))
                        .allowsHitTesting(false)
                }
                if model.isComplete { sparkles(side) }
            }
            .frame(width: side, height: side)
            .position(x: g.size.width / 2, y: g.size.height / 2)
        }
        .animation(reduceMotion ? nil : .spring(response: 0.45, dampingFraction: 0.5), value: model.items)
        .animation(reduceMotion ? nil : .easeOut(duration: 0.6), value: model.bodyScale)
        .onChange(of: model.items) { old, new in
            guard new > old, !reduceMotion else { return }
            popIndex = new - 1
            popToken += 1
        }
        .accessibilityHidden(true)
    }

    private var slots: [CGPoint] { GrowthModel.slots(kind, count: model.capacity) }

    /// 実・花・葉 i の画面上の位置(木は体の大きさで縮むので同じ変換をかける)
    private func point(_ i: Int, _ side: CGFloat) -> CGPoint {
        let p = slots[i]
        switch kind {
        case .tree:
            let s = model.bodyScale
            return CGPoint(x: side * (0.5 + (p.x - 0.5) * s), y: side * (0.92 - (0.92 - p.y) * s))
        case .flower:
            return CGPoint(x: side * p.x, y: side * p.y)
        case .plant:
            return CGPoint(x: side * 0.5 + p.x * side * 0.09, y: side * (0.92 - p.y * stemLength))
        }
    }

    private func ground(_ side: CGFloat) -> some View {
        Ellipse()
            .fill(RenderTheme.Colors.positive.opacity(0.22))
            .frame(width: side * 0.92, height: side * 0.12)
            .position(x: side / 2, y: side * 0.93)
    }

    // MARK: tree

    private func tree(_ side: CGFloat) -> some View {
        ZStack {
            ZStack {
                TrunkShape().fill(Self.trunk)
                ForEach(Array(Self.canopy.enumerated()), id: \.offset) { _, c in
                    Circle()
                        .fill(LinearGradient(colors: [RenderTheme.Colors.positive.opacity(0.95), RenderTheme.Colors.positive.opacity(0.75)], startPoint: .top, endPoint: .bottom))
                        .frame(width: side * c.r * 2, height: side * c.r * 2)
                        .position(x: side * c.x, y: side * c.y)
                }
            }
            .frame(width: side, height: side)
            .scaleEffect(model.bodyScale, anchor: UnitPoint(x: 0.5, y: 0.92))
            ForEach(0..<model.capacity, id: \.self) { i in
                if i < model.items {
                    Fruit(color: model.isComplete ? Self.gold : Self.fruit)
                        .frame(width: side * 0.085, height: side * 0.085)
                        .position(point(i, side))
                        .transition(.scale(scale: 0.05).combined(with: .opacity))
                } else {
                    Circle()
                        .strokeBorder(Color.white.opacity(0.55), style: StrokeStyle(lineWidth: 1.2, dash: [2, 2]))
                        .frame(width: side * 0.06, height: side * 0.06)
                        .position(point(i, side))
                }
            }
        }
    }

    private static let canopy: [(x: CGFloat, y: CGFloat, r: CGFloat)] = [
        (0.30, 0.44, 0.15), (0.70, 0.44, 0.15), (0.36, 0.27, 0.16), (0.64, 0.27, 0.16), (0.5, 0.36, 0.22), (0.5, 0.17, 0.13),
    ]

    // MARK: flower

    private func flowers(_ side: CGFloat) -> some View {
        ZStack {
            ForEach(0..<model.capacity, id: \.self) { i in
                let p = point(i, side)
                if i < model.items {
                    Path { path in
                        path.move(to: CGPoint(x: p.x, y: side * 0.93))
                        path.addQuadCurve(to: p, control: CGPoint(x: p.x + side * 0.03, y: (p.y + side * 0.93) / 2))
                    }
                    .stroke(RenderTheme.Colors.positive, style: StrokeStyle(lineWidth: max(side * 0.012, 1.5), lineCap: .round))
                    .transition(.opacity)
                    Bloom(color: Self.petal(i), center: model.isComplete ? Self.gold : Color(red: 0.99, green: 0.83, blue: 0.3))
                        .frame(width: side * 0.13, height: side * 0.13)
                        .position(p)
                        .transition(.scale(scale: 0.05).combined(with: .opacity))
                } else {
                    Capsule()
                        .fill(RenderTheme.Colors.positive.opacity(0.35))
                        .frame(width: side * 0.014, height: side * 0.05)
                        .position(x: p.x, y: side * 0.9)
                }
            }
        }
    }

    private static func petal(_ i: Int) -> Color {
        let palette: [Color] = [
            Color(red: 0.96, green: 0.45, blue: 0.62), Color(red: 0.98, green: 0.6, blue: 0.3),
            Color(red: 0.62, green: 0.48, blue: 0.95), RenderTheme.Colors.brandA, Color(red: 0.95, green: 0.35, blue: 0.4),
        ]
        return palette[i % palette.count]
    }

    // MARK: plant

    private var stemLength: CGFloat { 0.72 * model.bodyScale * (model.isComplete ? 1 : 0.92) }

    private func plant(_ side: CGFloat) -> some View {
        ZStack {
            Path { path in
                path.move(to: CGPoint(x: side * 0.5, y: side * 0.93))
                path.addQuadCurve(to: CGPoint(x: side * 0.5, y: side * (0.92 - stemLength)), control: CGPoint(x: side * 0.46, y: side * (0.92 - stemLength / 2)))
            }
            .stroke(model.isComplete ? Self.trunk : RenderTheme.Colors.positive, style: StrokeStyle(lineWidth: side * (model.isComplete ? 0.05 : 0.025), lineCap: .round))
            if model.isComplete {
                ForEach(Array(Self.canopy.enumerated()), id: \.offset) { _, c in
                    Circle()
                        .fill(RenderTheme.Colors.positive.opacity(0.85))
                        .frame(width: side * c.r * 1.7, height: side * c.r * 1.7)
                        .position(x: side * c.x, y: side * (c.y * 0.85 + 0.02))
                }
                .transition(.scale(scale: 0.2, anchor: .bottom).combined(with: .opacity))
            }
            ForEach(0..<model.items, id: \.self) { i in
                LeafShape()
                    .fill(RenderTheme.Colors.positive)
                    .frame(width: side * 0.16, height: side * 0.07)
                    .rotationEffect(.degrees(slots[i].x < 0 ? 200 : -20))
                    .position(point(i, side))
                    .transition(.scale(scale: 0.05).combined(with: .opacity))
            }
            if model.items == 0 {
                LeafShape().fill(RenderTheme.Colors.positive)
                    .frame(width: side * 0.08, height: side * 0.04)
                    .rotationEffect(.degrees(-30))
                    .position(x: side * 0.54, y: side * (0.92 - stemLength))
            }
        }
    }

    // MARK: 育ち切った時

    private func sparkles(_ side: CGFloat) -> some View {
        ForEach(0..<5, id: \.self) { i in
            Image(systemName: "sparkle")
                .font(.system(size: side * (i.isMultiple(of: 2) ? 0.07 : 0.05)))
                .foregroundStyle(Self.gold)
                .position(x: side * [0.12, 0.88, 0.2, 0.82, 0.5][i], y: side * [0.18, 0.22, 0.6, 0.62, 0.05][i])
                .transition(.scale.combined(with: .opacity))
        }
    }
}

/// 増えた瞬間に広がって消える輪(ポン)。最初は見えない。
private struct PopRing: View {
    let token: Int
    let color: Color

    struct Frame { var scale: CGFloat = 0.4; var opacity: Double = 0 }

    var body: some View {
        Circle()
            .stroke(color, lineWidth: 2.5)
            .keyframeAnimator(initialValue: Frame(), trigger: token) { content, f in
                content.scaleEffect(f.scale).opacity(f.opacity)
            } keyframes: { _ in
                KeyframeTrack(\.scale) {
                    LinearKeyframe(0.4, duration: 0.01)
                    CubicKeyframe(1.8, duration: 0.5)
                }
                KeyframeTrack(\.opacity) {
                    LinearKeyframe(0.9, duration: 0.01)
                    LinearKeyframe(0, duration: 0.5)
                }
            }
    }
}

/// 実: 赤い丸 + 光 + 小さな葉
private struct Fruit: View {
    let color: Color
    var body: some View {
        GeometryReader { g in
            let d = min(g.size.width, g.size.height)
            ZStack {
                Circle().fill(RadialGradient(colors: [color.opacity(0.85), color], center: UnitPoint(x: 0.35, y: 0.35), startRadius: 0, endRadius: d * 0.6))
                Circle().fill(Color.white.opacity(0.55)).frame(width: d * 0.24, height: d * 0.24).offset(x: -d * 0.17, y: -d * 0.17)
                LeafShape().fill(Color(red: 0.25, green: 0.6, blue: 0.3)).frame(width: d * 0.45, height: d * 0.22).rotationEffect(.degrees(-35)).offset(x: d * 0.2, y: -d * 0.5)
            }
            .frame(width: d, height: d)
            .shadow(color: .black.opacity(0.15), radius: 1.5, y: 1)
        }
    }
}

/// 花: 5 枚の花びら + 真ん中
private struct Bloom: View {
    let color: Color
    let center: Color
    var body: some View {
        GeometryReader { g in
            let d = min(g.size.width, g.size.height)
            ZStack {
                ForEach(0..<5, id: \.self) { i in
                    Ellipse().fill(color)
                        .frame(width: d * 0.36, height: d * 0.5)
                        .offset(y: -d * 0.24)
                        .rotationEffect(.degrees(Double(i) * 72))
                }
                Circle().fill(center).frame(width: d * 0.32, height: d * 0.32)
            }
            .frame(width: d, height: d)
        }
    }
}

/// 葉: 両端のとがった紡錘形(横長)
struct LeafShape: Shape {
    func path(in r: CGRect) -> Path {
        var p = Path()
        p.move(to: CGPoint(x: r.minX, y: r.midY))
        p.addQuadCurve(to: CGPoint(x: r.maxX, y: r.midY), control: CGPoint(x: r.midX, y: r.minY - r.height * 0.4))
        p.addQuadCurve(to: CGPoint(x: r.minX, y: r.midY), control: CGPoint(x: r.midX, y: r.maxY + r.height * 0.4))
        return p
    }
}

/// 幹: 下が太く上が細い台形 + 2 本の枝(0〜1 の正方形)
struct TrunkShape: Shape {
    func path(in r: CGRect) -> Path {
        func pt(_ x: CGFloat, _ y: CGFloat) -> CGPoint { CGPoint(x: r.minX + r.width * x, y: r.minY + r.height * y) }
        var p = Path()
        p.move(to: pt(0.44, 0.93))
        p.addQuadCurve(to: pt(0.475, 0.4), control: pt(0.47, 0.65))
        p.addLine(to: pt(0.525, 0.4))
        p.addQuadCurve(to: pt(0.56, 0.93), control: pt(0.53, 0.65))
        p.closeSubpath()
        p.move(to: pt(0.48, 0.6))
        p.addLine(to: pt(0.34, 0.47))
        p.addLine(to: pt(0.36, 0.45))
        p.addLine(to: pt(0.49, 0.56))
        p.closeSubpath()
        p.move(to: pt(0.52, 0.56))
        p.addLine(to: pt(0.64, 0.45))
        p.addLine(to: pt(0.66, 0.47))
        p.addLine(to: pt(0.52, 0.6))
        p.closeSubpath()
        return p
    }
}
