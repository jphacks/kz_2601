import SwiftUI
import MetamonDSL

/// `image`: フォームの photo field(data URL / https)を表示。height(small|medium|large)。
struct ImageLeaf: View {
    let c: Component
    let ctx: RenderContext

    private var height: CGFloat {
        switch c.height { case "small": 100; case "large": 260; default: 160 }
    }

    var body: some View {
        let value = c.field.flatMap { ctx.form?.values[$0] } ?? .null
        PhotoView(value: value, height: height)
    }
}
