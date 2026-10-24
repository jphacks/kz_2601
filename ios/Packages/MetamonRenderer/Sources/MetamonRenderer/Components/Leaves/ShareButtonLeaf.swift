import SwiftUI
import MetamonDSL

/// `shareButton`: ShareLink。`text` の `{{fieldId}}` をフォーム値で置換し、`field` が photo なら画像も共有する。
struct ShareButtonLeaf: View {
    let c: Component
    let ctx: RenderContext

    private var label: String { c.label ?? "共有" }
    private var icon: String { c.icon ?? "square.and.arrow.up" }

    /// `{{note}}` → ドラフトの値(フォーム外ではそのまま)。
    private var text: String {
        var s = c.text ?? ""
        if let form = ctx.form {
            for f in form.entity.fields {
                let v = FieldCoding.display(form.values[f.id] ?? .null, field: f)
                s = s.replacingOccurrences(of: "{{\(f.id)}}", with: v)
            }
        }
        if let field = c.field, let form = ctx.form, let f = form.entity.field(field), f.type != .photo {
            let v = FieldCoding.display(form.values[field] ?? .null, field: f)
            if !v.isEmpty { s += (s.isEmpty ? "" : "\n") + v }
        }
        return s
    }

    private var photo: UIImage? {
        guard let field = c.field, let form = ctx.form, form.entity.field(field)?.type == .photo else { return nil }
        return PhotoCodec.image(from: form.values[field] ?? .null)
    }

    var body: some View {
        Group {
            if let img = photo {
                ShareLink(item: Image(uiImage: img), message: Text(text), preview: SharePreview(text, image: Image(uiImage: img))) {
                    Label(label, systemImage: icon).frame(maxWidth: .infinity)
                }
            } else {
                ShareLink(item: text) {
                    Label(label, systemImage: icon).frame(maxWidth: .infinity)
                }
            }
        }
        .buttonStyle(.bordered)
        .controlSize(.large)
    }
}
