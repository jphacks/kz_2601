import SwiftUI
import MetamonDSL

/// `list` の1行(`ListItem` テンプレート: title / subtitle / trailing / icon / image)。
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
