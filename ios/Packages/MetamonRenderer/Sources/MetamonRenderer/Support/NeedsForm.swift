import SwiftUI
import MetamonDSL

/// フォーム外(screen.entity 無し)に入力が置かれた時のプレースホルダ。
struct NeedsForm: View {
    let c: Component
    var body: some View { UnsupportedComponent(label: "\(c.rawType) は screen.entity が必要です") }
}

/// 入力の上にキャプションを付ける共通ラッパ。
struct LabeledInput<Content: View>: View {
    var label: String?
    @ViewBuilder var content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            if let label, !label.isEmpty {
                Text(label).font(.caption).foregroundStyle(.secondary)
            }
            content
        }
    }
}
