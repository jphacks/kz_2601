import SwiftUI
import MetamonDSL

/// `spacer`: size(small|medium|large|flexible)。
struct SpacerLeaf: View {
    let c: Component

    var body: some View {
        switch c.size {
        case "small": Spacer().frame(height: 8)
        case "medium": Spacer().frame(height: 16)
        case "large": Spacer().frame(height: 32)
        default: Spacer(minLength: 0)
        }
    }
}
