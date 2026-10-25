import SwiftUI
import MetamonDSL

/// `stat`: entity の集計値(count|sum|avg|min|max)、または `value`(Expr)の評価結果を1枚のカードに。数値は大きく・等幅数字、ラベルは secondary。
/// 見た目(#89): 小さなブランド色アイコン + ラベル、カードの左上に極薄のブランドグラデーション。
/// VoiceOver はカード 1 要素として「今月の支出, 1,400円」と読む。Dynamic Type(アクセシビリティサイズ)ではラベルを 2 行まで許す。
struct StatLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.dynamicTypeSize) private var typeSize
    let c: Component

    var body: some View {
        let (formatted, isLoading, unavailable) = isValueMode ? exprValue : aggregateValue
        // アクセシビリティサイズ: 1 行高のセルでも数値が見えるよう、ラベルは縮小 1 行・余白を詰める(足りなければセル内スクロール)
        let compact = typeSize.isAccessibilitySize
        // area が 1 行しか無いと「アイコン行 + 数値」の 2 段が収まらず、カードの下が隣のセルに隠れていた(#157)。
        // その高さに満たない時は 1 段(アイコン・ラベル・数値を横並び)のカードに切り替える。
        // どちらもセルの高さいっぱいに広がるので、測った高さ = セルの高さ(切り替えても揺れない)。
        // 高さの提案が無い所(フォーム・スクロール内)は 2 段の自然な高さになり、従来どおり
        Group {
            if singleLineMode {
                singleLine(formatted, isLoading: isLoading)
            } else {
                regular(formatted, isLoading: isLoading, compact: compact)
            }
        }
        .onGeometryChange(for: Bool.self) { Self.usesSingleLine(height: $0.size.height) } action: { singleLineMode = $0 }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(c.label ?? "")
        .accessibilityValue(unavailable ? L("取得できませんでした") : formatted)
    }

    /// 2 段版が崩れずに収まる最小の高さ(余白 16×2 + アイコン行 18 + 間 4 + 最小の数値 22pt の行高 ≒ 27)。
    static let regularMinHeight: CGFloat = RenderTheme.Spacing.md * 2 + 18 + RenderTheme.Spacing.xs + 27

    /// 器の高さが 2 段版に足りない(1 行ぶんの area 等)なら 1 段版。0 はまだ測れていない時。
    static func usesSingleLine(height: CGFloat) -> Bool { height > 0 && height < regularMinHeight }

    @State private var singleLineMode = false

    /// 計算式モードで icon 未指定なら出さない(#193: 「fx」チップは意味が伝わらない)
    private var iconName: String? {
        c.icon ?? (isValueMode ? nil : FieldIcon.stat(aggregate: c.aggregate, field: fieldDef))
    }

    @ViewBuilder
    private var iconView: some View {
        if let iconName {
            Image(systemName: iconName)
                .font(.caption.weight(.bold))
                .foregroundStyle(RenderTheme.Colors.brandA)
                .frame(width: 18, height: 18)
                .background(RenderTheme.Colors.brandSoft, in: RoundedRectangle(cornerRadius: 6, style: .continuous))
                .accessibilityHidden(true)
        }
    }

    private func valueStyle(_ formatted: String) -> AnyShapeStyle {
        // マイナス(予算の残りが赤字 等)は負の色にする(#131: 「-¥112,230」が普通の黒で読み流されていた)
        formatted.hasPrefix("-") ? AnyShapeStyle(RenderTheme.Colors.negative) : AnyShapeStyle(.primary)
    }

    /// 1 段版(1 行ぶんの area): [アイコン ラベル ……… 数値]。数値は高さ・幅に収まる大きさまで下げる。
    private func singleLine(_ formatted: String, isLoading: Bool) -> some View {
        HStack(spacing: 6) {
            iconView
            Text(c.label ?? "")
                .font(RenderTheme.Fonts.statLabel)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .minimumScaleFactor(0.7)
            Spacer(minLength: RenderTheme.Spacing.sm)
            ViewThatFits(in: [.horizontal, .vertical]) {
                valueText(formatted, size: 28)
                valueText(formatted, size: 22)
                valueText(formatted, size: 17)
                valueText(formatted, size: 17).minimumScaleFactor(0.5)
            }
            .layoutPriority(1)
            .foregroundStyle(valueStyle(formatted))
            .contentTransition(.numericText())
            .redacted(reason: isLoading ? .placeholder : [])
        }
        .padding(.horizontal, RenderTheme.Spacing.xs)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .renderCard(padding: RenderTheme.Spacing.sm, wash: true)
    }

    private func regular(_ formatted: String, isLoading: Bool, compact: Bool) -> some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.xs) {
            HStack(spacing: 6) {
                iconView
                Text(c.label ?? "")
                    .font(RenderTheme.Fonts.statLabel)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .minimumScaleFactor(compact ? 0.6 : 0.8)
            }
            .frame(minHeight: 18)  // アイコン無しでも行高を保つ(regularMinHeight の前提)
            // 大きめの基準サイズから、セルの高さに収まるまで縮める(「もっと大きく」で area を広げると数字も大きくなる)
            // minimumScaleFactor は幅にしか効かないので、高さは ViewThatFits で基準 56pt から段階的に下げる
            // (#89: アイコン行を足した分、2 行セルで数値の下が切れていた)
            // #131: 幅も ViewThatFits で判定する(縦だけ見ていたので「¥1,582,7…」と省略されていた)。
            // 最後の候補だけ minimumScaleFactor で縮めて、どんな桁数でも省略記号を出さない
            ViewThatFits(in: [.horizontal, .vertical]) {
                valueText(formatted, size: 56)
                valueText(formatted, size: 44)
                valueText(formatted, size: 34)
                valueText(formatted, size: 28)
                valueText(formatted, size: 22)
                valueText(formatted, size: 22).minimumScaleFactor(0.5)
            }
            .foregroundStyle(valueStyle(formatted))
            .contentTransition(.numericText())
            .redacted(reason: isLoading ? .placeholder : [])
        }
        // セル全体をカードにする(area を縦に広げた時にカードの下だけ空くのを防ぐ)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .renderCard(padding: compact ? RenderTheme.Spacing.sm : RenderTheme.Spacing.md, wash: true)
    }

    /// 数値 1 段分(ViewThatFits の候補)。
    private func valueText(_ text: String, size: CGFloat) -> some View {
        Text(text)
            .font(.system(size: size, weight: .bold, design: .rounded).monospacedDigit())
            .lineLimit(1)
    }

    /// `value`(Expr)で値を出すモード(aggregate 指定が優先)
    private var isValueMode: Bool { c.aggregate == nil && c.valueExpr != nil }

    /// アイコン選択用の field 定義(value モードでは無し)
    private var fieldDef: Field? {
        c.field.flatMap { rt.dsl.entity(c.entity ?? "")?.field($0) }
    }

    /// aggregate モード(従来): entity の集計。field に computed を指定してもよい(#101)。
    private var aggregateValue: (String, Bool, Bool) {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let rows = rt.filteredRecords(c)
        let value = Aggregator.aggregate(c.aggregate, field: c.field, over: rows)
        let fieldDef = c.field.flatMap { entity?.field($0) }
        let unit = fieldDef?.unit
        // format 省略時: count → 件数、currency 型の field → ¥ と桁区切り
        let format = c.format ?? (c.aggregate == "count" ? "plain" : (fieldDef?.type == .currency ? "currency" : nil))
        // 取得失敗(#85)は「¥0」ではなく「—」(データ無しと区別)。ローカルに何かあれば通常どおり集計する
        let unavailable = rows.isEmpty && rt.emptyState(for: entityId) == .unavailable
        // 初回ロード中は 0 を見せず placeholder(起動直後に「¥0 → ¥480」と跳ねるのを防ぐ)
        let isLoading = rt.loading.contains(entityId) && rt.records(for: entityId).isEmpty
        var formatted = isLoading ? "0000" : unavailable ? EmptyPlaceholder.unavailableValue : Aggregator.format(value, as: format, unit: c.aggregate == "count" ? L("件") : unit)
        // latest / earliest(#133): 1 件の値。0 件・空は「—」
        if !isLoading, !unavailable, Aggregator.isPick(c.aggregate) {
            formatted = Aggregator.pickText(c, entity: entity, records: rows) ?? EmptyPlaceholder.unavailableValue
        }
        // distance / duration(#168): locationTracker の点列を時刻順に繋いだ距離・時間(10 分以上空いたら別の記録)
        if !isLoading, !unavailable, Track.isTrackAggregate(c.aggregate) {
            formatted = Track.format(c.aggregate, Track.aggregate(c.aggregate, records: rows, locationField: c.field, by: c.by))
        }
        return (formatted, isLoading, unavailable)
    }

    /// value モード(#101): 行コンテキスト無しで Expr を評価。評価できなければ「—」。
    private var exprValue: (String, Bool, Bool) {
        guard let expr = c.valueExpr else { return (EmptyPlaceholder.unavailableValue, false, true) }
        let entities = expr.referencedEntities
        let isLoading = entities.contains { rt.loading.contains($0) && rt.records(for: $0).isEmpty }
        if isLoading { return ("0000", true, false) }
        let failed = entities.contains { rt.records(for: $0).isEmpty && rt.emptyState(for: $0) == .unavailable }
        guard !failed, let v = rt.evaluator().evaluate(expr) else { return (EmptyPlaceholder.unavailableValue, false, true) }
        switch v {
        // percent の value は比率(0〜1)で来る(生成 DSL は全件 `sum / 目標` の形・progressBar の value と同じ尺度)。
        // そのまま "%.0f%%" に通すと 1.42 → 「1%」になっていた(#131)
        case .number(let n): return (Aggregator.format(c.format == "percent" ? n * 100 : n, as: c.format, unit: c.unit), false, false)
        default: return (v.text, false, false)
        }
    }
}
