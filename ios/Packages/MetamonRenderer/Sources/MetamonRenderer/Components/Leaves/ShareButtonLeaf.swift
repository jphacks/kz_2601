import SwiftUI
import MetamonDSL

/// `shareButton`: `text` の `{{fieldId}}` をフォーム値で置換し、`field` が photo なら画像も共有する。
/// `shareTo` preset: any = OS 共有シート(ShareLink)/ line = LINE の共有 URL を `openURL`(未インストールなら Web 版)/
/// slack・discord = 公式の汎用共有スキームが無いので共有シートを開き、文面をチャンネル向けのプレーンテキストに整える(Webhook 投稿は #37 中版)。
struct ShareButtonLeaf: View {
    @Environment(\.openURL) private var openURL
    let c: Component
    let ctx: RenderContext

    private var target: ShareTarget { ShareTarget(c.shareTo) }
    private var label: String { c.label ?? target.defaultLabel }
    private var icon: String { c.icon ?? target.defaultIcon }

    /// `{{note}}` → ドラフトの値(フォーム外ではそのまま)。
    private var rawText: String {
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

    private var text: String { target.format(rawText) }

    private var photo: UIImage? {
        guard let field = c.field, let form = ctx.form, form.entity.field(field)?.type == .photo else { return nil }
        return PhotoCodec.image(from: form.values[field] ?? .null)
    }

    var body: some View {
        Group {
            if target == .line, let url = ShareTarget.lineShareURL(text: text) {
                // LINE は写真を URL で渡せないので文面のみ。
                Button { openURL(url) } label: { buttonLabel }
            } else if let img = photo {
                ShareLink(item: Image(uiImage: img), message: Text(text), preview: SharePreview(text, image: Image(uiImage: img))) {
                    buttonLabel
                }
            } else {
                ShareLink(item: text) { buttonLabel }
            }
        }
        .buttonStyle(.bordered)
        .controlSize(.large)
    }

    private var buttonLabel: some View {
        Label(label, systemImage: icon).frame(maxWidth: .infinity)
    }
}
