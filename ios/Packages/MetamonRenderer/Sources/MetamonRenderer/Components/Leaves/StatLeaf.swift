import SwiftUI
import MetamonDSL

/// `stat`: entity の集計値(count|sum|avg|min|max)を1枚のカードに。数値は大きく・等幅数字、ラベルは secondary。
/// 見た目(#89): 小さなブランド色アイコン + ラベル、カードの左上に極薄のブランドグラデーション。
/// VoiceOver はカード 1 要素として「今月の支出, 1,400円」と読む。Dynamic Type(アクセシビリティサイズ)ではラベルを 2 行まで許す。
struct StatLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.dynamicTypeSize) private var typeSize
    let c: Component

    var body: some View {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let rows = Aggregator.apply(c.filter, entity: entity, to: rt.records(for: entityId))
        let value = Aggregator.aggregate(c.aggregate, field: c.field, over: rows)
        let fieldDef = c.field.flatMap { entity?.field($0) }
        let unit = fieldDef?.unit
        // format 省略時: count → 件数、currency 型の field → ¥ と桁区切り
        let format = c.format ?? (c.aggregate == "count" ? "plain" : (fieldDef?.type == .currency ? "currency" : nil))
        // 取得失敗(#85)は「¥0」ではなく「—」(データ無しと区別)。ローカルに何かあれば通常どおり集計する
        let unavailable = rows.isEmpty && rt.emptyState(for: entityId) == .unavailable
        // 初回ロード中は 0 を見せず placeholder(起動直後に「¥0 → ¥480」と跳ねるのを防ぐ)
        let isLoading = rt.loading.contains(entityId) && rt.records(for: entityId).isEmpty
        let formatted = isLoading ? "0000" : unavailable ? EmptyPlaceholder.unavailableValue : Aggregator.format(value, as: format, unit: c.aggregate == "count" ? "件" : unit)
        // アクセシビリティサイズ: 1 行高のセルでも数値が見えるよう、ラベルは縮小 1 行・余白を詰める(足りなければセル内スクロール)
        let compact = typeSize.isAccessibilitySize
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.xs) {
            HStack(spacing: 6) {
                Image(systemName: c.icon ?? FieldIcon.stat(aggregate: c.aggregate, field: fieldDef))
                    .font(.caption.weight(.bold))
                    .foregroundStyle(RenderTheme.Colors.brandA)
                    .frame(width: 18, height: 18)
                    .background(RenderTheme.Colors.brandSoft, in: RoundedRectangle(cornerRadius: 6, style: .continuous))
                    .accessibilityHidden(true)
                Text(c.label ?? "")
                    .font(RenderTheme.Fonts.statLabel)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .minimumScaleFactor(compact ? 0.6 : 0.8)
            }
            // 大きめの基準サイズから、セルの高さに収まるまで縮める(「もっと大きく」で area を広げると数字も大きくなる)
            // minimumScaleFactor は幅にしか効かないので、高さは ViewThatFits で基準 56pt から段階的に下げる
            // (#89: アイコン行を足した分、2 行セルで数値の下が切れていた)
            ViewThatFits(in: .vertical) {
                valueText(formatted, size: 56)
                valueText(formatted, size: 44)
                valueText(formatted, size: 34)
                valueText(formatted, size: 28)
                valueText(formatted, size: 22)
            }
            .contentTransition(.numericText())
            .redacted(reason: isLoading ? .placeholder : [])
        }
        // セル全体をカードにする(area を縦に広げた時にカードの下だけ空くのを防ぐ)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .renderCard(padding: compact ? RenderTheme.Spacing.sm : RenderTheme.Spacing.md, wash: true)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(c.label ?? "")
        .accessibilityValue(unavailable ? "取得できませんでした" : formatted)
    }

    /// 数値 1 段分(ViewThatFits の候補)。幅は minimumScaleFactor で縮める
    private func valueText(_ text: String, size: CGFloat) -> some View {
        Text(text)
            .font(.system(size: size, weight: .bold, design: .rounded).monospacedDigit())
            .lineLimit(1)
            .minimumScaleFactor(0.45)
    }
}
