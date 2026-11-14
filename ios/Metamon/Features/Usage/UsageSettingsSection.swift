import SwiftUI
import MetamonServices

/// 設定の「使っている量」(#317)。作ったアプリの数と、今のアプリの記録・写真(作成者の時だけ)。
/// サーバーが古い・読めない時は出さない。Pro は上限が上がった値がそのまま出る。
struct UsageSettingsSection: View {
    @Environment(AppRuntime.self) private var rt

    private var me: MyUsage? { PlanUsageStore.shared.me }

    private struct Row: Identifiable {
        var resource: PlanLimit.Resource
        var meter: UsageMeter
        var id: String { resource.rawValue }
    }

    private var rows: [Row] {
        var out: [Row] = []
        if let apps = me?.apps { out.append(Row(resource: .apps, meter: apps)) }
        if let app = rt.currentAppUsage {
            if let r = app.records { out.append(Row(resource: .recordsPerApp, meter: r)) }
            if let p = app.photos { out.append(Row(resource: .photosPerApp, meter: p)) }
        }
        return out
    }

    private var isPro: Bool {
        if RemainingTodayPlan.isPro { return true }
        return me?.isPro == true || rt.currentAppUsage?.plan == "pro"
    }

    /// 読み込みは設定シートの `.task`(行が無い間はこの View が無いので、ここでは読まない)
    var body: some View {
        Group {
            if !rows.isEmpty {
                Section {
                    ForEach(rows) { row in
                        let resource = row.resource, meter = row.meter
                        LabeledContent(UsageCopy.rowTitle(resource)) {
                            Text(UsageCopy.amount(meter))
                                .monospacedDigit()
                                .foregroundStyle(UsageNoticeRules.isNear(meter) ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(.secondary))
                        }
                        .accessibilityElement(children: .ignore)
                        .accessibilityLabel(UsageCopy.rowTitle(resource))
                        .accessibilityValue(UsageCopy.accessibilityAmount(meter))
                        .accessibilityIdentifier("settings.usage.\(resource.rawValue)")
                    }
                } header: {
                    Text(UsageCopy.sectionTitle)
                } footer: {
                    Text(UsageCopy.footer(isPro: isPro))
                }
            }
        }
    }
}
