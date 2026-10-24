import SwiftUI

/// 未知ノード。DEBUG では薄く表示、RELEASE では何も描かない(前方互換の約束)。
public struct UnsupportedComponent: View {
    let label: String

    public init(label: String) { self.label = label }

    public var body: some View {
        #if DEBUG
        Text(label)
            .font(.caption2)
            .foregroundStyle(.secondary)
            .padding(6)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(.yellow.opacity(0.15), in: RoundedRectangle(cornerRadius: 6))
        #else
        EmptyView()
        #endif
    }
}
