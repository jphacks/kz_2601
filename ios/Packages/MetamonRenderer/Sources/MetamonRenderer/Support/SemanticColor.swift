import SwiftUI
import MetamonDSL

extension Color {
    /// schema `SemanticColor`(primary|secondary|accent|red|green|orange|blue)→ Color。未知は primary。
    static func semantic(_ name: String?) -> Color {
        switch name {
        case "secondary": .secondary
        case "accent": .accentColor
        case "red": .red
        case "green": .green
        case "orange": .orange
        case "blue": .blue
        default: .primary
        }
    }
}

extension Component {
    /// icon / avatar の `size`(small|medium|large)→ pt。
    var symbolPointSize: CGFloat {
        switch size { case "small": 16; case "large": 44; default: 28 }
    }
}
