import SwiftUI
import MetamonDSL

/// `growth`: 数値が増えるほど絵が育つカード(tree=木に実がなる / flower=花が咲く / plant=芽が木に育つ)。
/// 数値は `metric: steps`(今日の歩数・`live` で開いている間も増える)・`entity` の集計・`value`(Expr)のどれか 1 つ。
/// `step` ごとに実・花・葉が 1 つ増え(増えた瞬間にポンと出る)、`target` で育ち切る。絵は図形だけで描く(画像を持たない)。
/// 歩数は `live` を省略しても、画面に出ていて前面にいる間はいつも歩数計を購読する(育つのを見るための部品なので)。
/// 前面に戻るたびに今日の歩数を読み直す。表示はその間は減らさない(歩数計の問い合わせの遅れで実が一瞬消えないように)。
/// `since: open` は、カードが出た時の今日の歩数を 0 として、そこから歩いた分で育てる(展示向け・開き直すか「最初から」で 0 に戻る)。
struct GrowthLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    let c: Component
    @State private var steps: StepsPhase
    /// `steps` の値が何日の歩数か(日付が変わったら減ってよい)
    @State private var stepsDay: Date?
    /// `since: open` の 0 にする今日の歩数(最初の読みで決まる・「最初から」で今の値に)
    @State private var baseline: Double?

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
    private var isSinceOpen: Bool { isSteps && c.since == "open" }

    var body: some View {
        let target = Self.target(c, rt: rt)
        let raw: Double? = isSteps ? Self.sinceValue(total: stepsValue, baseline: isSinceOpen ? baseline : nil) : Self.recordValue(c, rt: rt)
        let model = Self.model(raw: raw, target: target, step: c.step)
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            header(model, target: target)
            GrowthScene(kind: kind, model: model, reduceMotion: reduceMotion)
                .frame(maxWidth: .infinity, minHeight: 96, maxHeight: .infinity)
            footer(model, raw: raw, target: target)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .renderCard(padding: RenderTheme.Spacing.md, wash: true)
        // 1 つ増えた時だけ(開いた直後に 0 → 3 と跳ねる時・日付が変わって減る時は鳴らさない)
        .sensoryFeedback(.increase, trigger: model.items) { old, new in new == old + 1 }
        // 30 個なって摘んだ時
        .sensoryFeedback(.success, trigger: model.harvests) { old, new in new == old + 1 }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(c.label ?? "")
        .accessibilityValue(accessibilityValue(model, raw: raw, target: target))
        .accessibilityActions {
            if isSinceOpen { Button(L("最初から")) { restart() } }
        }
        .task(id: scenePhase == .active) {
            guard isSteps, scenePhase == .active else { return }
            await loadSteps()
        }
    }

    // MARK: 見出し・数値

    private func header(_ model: GrowthModel, target: Double?) -> some View {
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
            if model.harvests > 0 {
                Text(Self.harvestText(kind, model.harvests))
                    .font(.caption.weight(.semibold).monospacedDigit())
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .contentTransition(.numericText())
                    .animation(.snappy, value: model.harvests)
            }
            Text(target == nil ? "\(Self.itemName(kind)) —" : model.isComplete ? Self.completeText(kind) : "\(Self.itemName(kind)) \(model.items) / \(model.capacity)")
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
            if target == nil {
                Text(L("目標を決めると育ちます"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            } else {
                HStack(spacing: RenderTheme.Spacing.sm) {
                    if let next = model.nextAt, raw != nil {
                        Text(L("あと \(Self.numberText(max(next - model.value, 0))) \(unit)で次の\(Self.itemName(kind))"))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .monospacedDigit()
                    }
                    if isSinceOpen, case .value = steps {
                        Spacer(minLength: 0)
                        // 展示で次の人に渡す時など、その場で 0 から
                        Button(L("最初から"), systemImage: "arrow.counterclockwise") { restart() }
                            .font(.caption.weight(.semibold))
                            .buttonStyle(.bordered)
                            .controlSize(.mini)
                            .tint(RenderTheme.Colors.positive)
                    }
                }
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

    /// `since: open` の表示する歩数: 今日の歩数 − 0 にした歩数(減らない・負にしない)。baseline が無ければ今日の歩数のまま。
    static func sinceValue(total: Double?, baseline: Double?) -> Double? {
        guard let total, let baseline else { return total }
        return max(total - baseline, 0)
    }

    /// `since: open` の 0 にする歩数。最初の読みでその値。日付が変わったら前日に歩いた分を持ち越す(0 時をまたいでも減らない)。
    static func nextBaseline(current: Double?, previousTotal: Double?, nextTotal: Double, sameDay: Bool) -> Double {
        guard let current else { return nextTotal }
        if sameDay { return current }
        return current - (previousTotal ?? current)
    }

    /// 目標が無い(設定が空・0)時は育てない(target 1 で「収穫!」にしない)。
    static func model(raw: Double?, target: Double?, step: Double?) -> GrowthModel {
        guard let target else { return GrowthModel(value: 0, target: 1, step: 1) }
        return GrowthModel(value: raw ?? 0, target: target, step: step)
    }

    /// 育ち切る値。数値の `target`、または `targetExpr`({"setting"})の今の値。0 以下・空は nil。
    @MainActor
    static func target(_ c: Component, rt: any DSLRuntime) -> Double? {
        // target 省略は step × 10(10 個で育ち切る)
        if c.target == nil, c.targetExpr == nil, let s = c.step, s.isFinite, s > 0 { return s * 10 }
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

    /// 30 個なって摘んだ回数(target ÷ step が 30 を超える時)
    static func harvestText(_ kind: GrowthModel.Kind, _ n: Int) -> String {
        switch kind {
        case .tree: L("収穫 \(n) 回")
        case .flower: L("満開 \(n) 回")
        case .plant: L("育てた木 \(n) 本")
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

    /// 新しい歩数を出す。同じ日の間は減らさない(前面に戻った時の読み直しも)。
    private func show(steps value: Double) {
        let today = Calendar.current.startOfDay(for: Date())
        let sameDay = stepsDay == today
        let previous = stepsValue
        let total = Self.merged(current: previous, next: value, sameDay: sameDay)
        if isSinceOpen {
            baseline = Self.nextBaseline(current: baseline, previousTotal: previous, nextTotal: total, sameDay: sameDay)
        }
        steps = .value(total)
        stepsDay = today
    }

    /// `since: open` をその場で 0 から数え直す
    private func restart() {
        guard isSinceOpen, case .value(let total) = steps else { return }
        baseline = total
    }

    private func loadSteps() async {
        #if APP_STORE_REVIEW
        steps = .failed(L("この版では歩数を読めません"))
        #else
        let source = HealthData.source
        do {
            let base = try await source.steps(days: 1).last?.value ?? 0
            // 許可のダイアログ等で背面に回って取り消された読みは捨てる(新しい task が読み直す・遅れて来た古い結果で上書きしない)
            guard !Task.isCancelled else { return }
            show(steps: base)
            // healthStat の live(#3694)と同じ口: 画面に出ている間だけ歩数計の増分を足し続ける(growth は live の有無に関わらず)
            let monitor = HealthLiveStepsMonitor(baseline: base, source: HealthData.liveSource) {
                try await source.steps(days: 1).last?.value ?? 0
            }
            monitor.onChange = { show(steps: $0) }
            monitor.start()
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(60))
            }
            monitor.stop()
        } catch HealthDataError.denied {
            guard !Task.isCancelled else { return }
            steps = .denied
        } catch let e as HealthDataError {
            guard !Task.isCancelled else { return }
            steps = .failed(e.errorDescription ?? "")
        } catch {
            guard !Task.isCancelled else { return }
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
    private static let trunkLight = Color(red: 0.66, green: 0.47, blue: 0.31)
    /// 葉は不透明(重なりや幹が透けない)。下側の影の色・上側の明るい色
    private static let leafShade = Color(red: 0.13, green: 0.45, blue: 0.24)
    private static let leafTop = Color(red: 0.38, green: 0.72, blue: 0.36)
    private static let leafBottom = Color(red: 0.22, green: 0.58, blue: 0.29)
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
            // 幹と枝は葉の後ろ(葉の下から幹と枝の付け根だけが見える)
            ZStack {
                TrunkShape().fill(LinearGradient(colors: [Self.trunkLight, Self.trunk], startPoint: .leading, endPoint: .trailing))
                Self.foliage(Self.canopy)
            }
            .frame(width: side, height: side)
            .scaleEffect(model.bodyScale, anchor: UnitPoint(x: 0.5, y: 0.92))
            ForEach(0..<model.capacity, id: \.self) { i in
                if i < model.items {
                    Fruit(color: model.isComplete ? Self.gold : Self.fruit)
                        .frame(width: side * 0.095, height: side * 0.095)
                        .position(point(i, side))
                        .transition(.scale(scale: 0.05).combined(with: .opacity))
                } else {
                    Circle()
                        .strokeBorder(Color.white.opacity(0.6), style: StrokeStyle(lineWidth: 1.2, dash: [2, 2]))
                        .frame(width: side * 0.06, height: side * 0.06)
                        .position(point(i, side))
                }
            }
        }
    }

    private static let canopy: [CanopyShape.Blob] = [
        .init(x: 0.30, y: 0.44, r: 0.15), .init(x: 0.70, y: 0.44, r: 0.15), .init(x: 0.36, y: 0.27, r: 0.16),
        .init(x: 0.64, y: 0.27, r: 0.16), .init(x: 0.5, y: 0.36, r: 0.22), .init(x: 0.5, y: 0.17, r: 0.13),
    ]

    /// 樹冠: 円の和集合を 1 つの不透明なシルエットにする。各円の下側に影(少し上にずらした明るい円を重ねる)・輪郭に影。
    static func foliage(_ blobs: [CanopyShape.Blob]) -> some View {
        let outline = CanopyShape(blobs: blobs)
        return ZStack {
            outline.fill(leafShade)
            CanopyShape(blobs: blobs, lift: 0.14, shrink: 0.9)
                .fill(LinearGradient(colors: [leafTop, leafBottom], startPoint: .top, endPoint: .bottom))
                .clipShape(outline)
            outline.stroke(leafShade.opacity(0.9), lineWidth: 1)
        }
        .compositingGroup()
        .shadow(color: .black.opacity(0.18), radius: 3, y: 2)
    }

    // MARK: flower

    private func flowers(_ side: CGFloat) -> some View {
        // 奥(上)の花から順に描く(手前の茎が奥の花の上を通る)
        let order = (0..<model.capacity).sorted { slots[$0].y < slots[$1].y }
        return ZStack {
            ForEach(order, id: \.self) { i in
                let p = point(i, side)
                if i < model.items {
                    Path { path in
                        path.move(to: CGPoint(x: p.x, y: side * 0.93))
                        path.addQuadCurve(to: p, control: CGPoint(x: p.x + side * 0.03, y: (p.y + side * 0.93) / 2))
                    }
                    .stroke(Self.leafBottom, style: StrokeStyle(lineWidth: max(side * 0.012, 1.5), lineCap: .round))
                    .transition(.opacity)
                    LeafShape()
                        .fill(Self.leafBottom)
                        .frame(width: side * 0.07, height: side * 0.03)
                        .rotationEffect(.degrees(i.isMultiple(of: 2) ? -30 : 210))
                        .position(x: p.x + side * (i.isMultiple(of: 2) ? 0.045 : -0.02), y: side * 0.93 - (side * 0.93 - p.y) * 0.4)
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
            .stroke(model.isComplete ? Self.trunk : Self.leafBottom, style: StrokeStyle(lineWidth: side * (model.isComplete ? 0.05 : 0.025), lineCap: .round))
            ForEach(0..<model.items, id: \.self) { i in
                LeafShape()
                    .fill(LinearGradient(colors: [Self.leafTop, Self.leafBottom], startPoint: .top, endPoint: .bottom))
                    .frame(width: side * 0.16, height: side * 0.07)
                    .rotationEffect(.degrees(slots[i].x < 0 ? 200 : -20))
                    .position(point(i, side))
                    .transition(.scale(scale: 0.05).combined(with: .opacity))
            }
            if model.items == 0 {
                LeafShape().fill(Self.leafBottom)
                    .frame(width: side * 0.08, height: side * 0.04)
                    .rotationEffect(.degrees(-30))
                    .position(x: side * 0.54, y: side * (0.92 - stemLength))
            }
            // 育ち切ったら樹冠が茎と上の葉を覆う(透けない)
            if model.isComplete {
                Self.foliage(Self.canopy.map { .init(x: $0.x, y: $0.y * 0.85 + 0.02, r: $0.r * 0.85) })
                    .frame(width: side, height: side)
                    .transition(.scale(scale: 0.2, anchor: .bottom).combined(with: .opacity))
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
                // 不透明な地 + 明るい側(白を重ねても透けない)+ 細い縁
                Circle().fill(color)
                Circle().fill(RadialGradient(colors: [Color.white.opacity(0.3), Color.white.opacity(0)], center: UnitPoint(x: 0.35, y: 0.3), startRadius: 0, endRadius: d * 0.55))
                Circle().strokeBorder(Color.black.opacity(0.14), lineWidth: max(d * 0.04, 0.75))
                Circle().fill(Color.white.opacity(0.75)).frame(width: d * 0.2, height: d * 0.2).offset(x: -d * 0.18, y: -d * 0.18)
                LeafShape().fill(Color(red: 0.25, green: 0.6, blue: 0.3)).frame(width: d * 0.45, height: d * 0.22).rotationEffect(.degrees(-35)).offset(x: d * 0.2, y: -d * 0.5)
            }
            .frame(width: d, height: d)
            .shadow(color: .black.opacity(0.22), radius: 1.5, y: 1)
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

/// 円の和集合(0〜1 の正方形)。重なりを 1 つの輪郭にするので塗っても縁を引いても重なりが見えない。
/// `lift` は円を半径の何倍上にずらすか、`shrink` は半径の倍率(樹冠の陰影用)。
struct CanopyShape: Shape {
    struct Blob: Equatable, Sendable {
        var x: CGFloat
        var y: CGFloat
        var r: CGFloat
    }

    var blobs: [Blob]
    var lift: CGFloat = 0
    var shrink: CGFloat = 1

    func path(in rect: CGRect) -> Path {
        blobs.reduce(Path()) { acc, b in
            let r = b.r * shrink * rect.width
            let c = CGPoint(x: rect.minX + b.x * rect.width, y: rect.minY + (b.y - b.r * lift) * rect.height)
            let circle = Path(ellipseIn: CGRect(x: c.x - r, y: c.y - r, width: r * 2, height: r * 2))
            return acc.isEmpty ? circle : acc.union(circle)
        }
    }
}
