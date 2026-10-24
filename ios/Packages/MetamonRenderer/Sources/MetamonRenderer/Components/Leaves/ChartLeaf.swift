import SwiftUI
import Charts
import MetamonDSL

/// `chart`: groupBy で束ねた集計を bar|line|pie で描く(Swift Charts)。
struct ChartLeaf: View {
    @Environment(\.dslRuntime) private var rt
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
