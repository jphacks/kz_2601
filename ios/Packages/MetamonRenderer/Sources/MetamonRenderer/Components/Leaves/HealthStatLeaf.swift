// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import SwiftUI
import Charts
import MetamonDSL

/// `healthStat`(#167): 端末のヘルスデータのカード。entity を持たず `HealthData.source` から読む。
/// metric steps|weight × period today|week。today は数値(steps は target で達成率バー)、week は 7 日のグラフ。
/// 許可が無い・この端末で使えない時は理由を出す(`HealthDataError`)。前面に戻るたびに読み直す。
struct HealthStatLeaf: View {
    let c: Component
    @Environment(\.scenePhase) private var scenePhase
    @State private var phase: LoadPhase = .loading

    enum LoadPhase: Equatable {
        case loading
        case loaded([HealthDailyValue])
        case failed(String)
    }

    private var isWeight: Bool { c.metric == "weight" }
    private var isWeek: Bool { c.period == "week" }

    var body: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            header
            content
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .renderCard(padding: RenderTheme.Spacing.md, wash: true)
        .accessibilityElement(children: .combine)
        .task(id: scenePhase == .active) {
            guard scenePhase == .active else { return }
            await load()
        }
    }

    private var header: some View {
        HStack(spacing: 6) {
            Image(systemName: isWeight ? "scalemass.fill" : "figure.walk")
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
    private var content: some View {
        switch phase {
        case .loading:
            Text(isWeight ? "00.0 kg" : "0,000")
                .font(.system(size: 34, weight: .bold, design: .rounded))
                .redacted(reason: .placeholder)
        case .failed(let message):
            Label(message, systemImage: "heart.slash")
                .font(.footnote)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        case .loaded(let values):
            if values.isEmpty {
                Label(L("ヘルスケアに体重の記録がありません"), systemImage: "heart.text.square")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            } else if isWeek {
                week(values)
            } else {
                today(values)
            }
        }
    }

    // MARK: today

    @ViewBuilder
    private func today(_ values: [HealthDailyValue]) -> some View {
        let latest = values.last?.value ?? 0
        ViewThatFits(in: [.horizontal, .vertical]) {
            valueText(latest, size: 44)
            valueText(latest, size: 34)
            valueText(latest, size: 22)
        }
        if isWeight, let day = values.last?.day, !Calendar.current.isDateInToday(day) {
            Text(L("\(day.formatted(.dateTime.month().day())) の記録"))
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        if !isWeight, let target = c.target, target > 0 {
            let rate = latest / target
            ProgressView(value: min(rate, 1))
                .tint(rate >= 1 ? RenderTheme.Colors.positive : RenderTheme.Colors.brandA)
            Text(L("目標 \(Self.stepsText(target)) 歩の \(Int((rate * 100).rounded()))%"))
                .font(.caption)
                .foregroundStyle(.secondary)
                .monospacedDigit()
        }
    }

    private func valueText(_ v: Double, size: CGFloat) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 4) {
            Text(isWeight ? String(format: "%.1f", v) : Self.stepsText(v))
                .font(.system(size: size, weight: .bold, design: .rounded).monospacedDigit())
                .contentTransition(.numericText())
            Text(isWeight ? "kg" : L("歩"))
                .font(.system(size: max(size * 0.4, 13), weight: .semibold, design: .rounded))
                .foregroundStyle(.secondary)
        }
        .lineLimit(1)
    }

    // MARK: week

    @ViewBuilder
    private func week(_ values: [HealthDailyValue]) -> some View {
        Chart {
            ForEach(values) { v in
                if isWeight {
                    LineMark(x: .value("day", v.day, unit: .day), y: .value("kg", v.value))
                        .foregroundStyle(RenderTheme.Colors.brandA)
                        .lineStyle(StrokeStyle(lineWidth: 2.5, lineCap: .round))
                    PointMark(x: .value("day", v.day, unit: .day), y: .value("kg", v.value))
                        .foregroundStyle(RenderTheme.Colors.brandA)
                        .symbolSize(36)
                } else {
                    BarMark(x: .value("day", v.day, unit: .day), y: .value("steps", v.value))
                        .foregroundStyle(RenderTheme.brandGradient)
                        .cornerRadius(5)
                }
            }
            if !isWeight, let target = c.target, target > 0 {
                RuleMark(y: .value("target", target))
                    .foregroundStyle(RenderTheme.Colors.positive)
                    .lineStyle(StrokeStyle(lineWidth: 1.5, dash: [4, 3]))
            }
        }
        .chartYScale(domain: isWeight ? .automatic(includesZero: false) : .automatic(includesZero: true))
        .chartXAxis {
            AxisMarks(values: .stride(by: .day)) { _ in
                AxisValueLabel(format: .dateTime.weekday(.narrow)).font(RenderTheme.Fonts.legend)
            }
        }
        .chartYAxis { AxisMarks { AxisGridLine().foregroundStyle(RenderTheme.Colors.hairline); AxisValueLabel().font(RenderTheme.Fonts.legend) } }
        .frame(minHeight: 100, idealHeight: 160, maxHeight: .infinity)
        Text(summary(values))
            .font(.caption)
            .foregroundStyle(.secondary)
            .monospacedDigit()
    }

    private func summary(_ values: [HealthDailyValue]) -> String {
        if isWeight {
            guard let first = values.first?.value, let last = values.last?.value else { return "" }
            return L("最新 \(String(format: "%.1f", last)) kg(7 日で \(String(format: "%+.1f", last - first)) kg)")
        }
        let avg = values.map(\.value).reduce(0, +) / Double(max(values.count, 1))
        return L("1 日平均 \(Self.stepsText(avg)) 歩")
    }

    static func stepsText(_ v: Double) -> String {
        Int(v.rounded()).formatted(.number.grouping(.automatic))
    }

    // MARK: load

    private func load() async {
        let source = HealthData.source
        let days = isWeek ? 7 : (isWeight ? 30 : 1)
        do {
            let values = isWeight ? try await source.weights(days: days) : try await source.steps(days: days)
            phase = .loaded(values)
        } catch let e as HealthDataError {
            phase = .failed(e.errorDescription ?? "")
        } catch {
            phase = .failed(L("ヘルスデータを取得できませんでした"))
        }
    }
}
#endif
