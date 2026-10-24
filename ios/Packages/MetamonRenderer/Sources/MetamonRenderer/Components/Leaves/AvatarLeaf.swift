import SwiftUI
import MetamonDSL

/// `avatar`: 円形の画像。優先順: screen.entity の photo `field`(フォームのドラフト)→ `image` リテラル(data URL / https)→ `name` のイニシャル → person アイコン。
struct AvatarLeaf: View {
    let c: Component
    let ctx: RenderContext

    private var diameter: CGFloat { c.symbolPointSize * 1.8 }

    private var photo: JSONValue {
        if let field = c.field, let v = ctx.form?.values[field], !v.isNull { return v }
        return c.image.map { .string($0) } ?? .null
    }

    private var initials: String {
        let name = (c.name ?? "").trimmingCharacters(in: .whitespaces)
        guard !name.isEmpty else { return "" }
        let parts = name.split(separator: " ")
        if parts.count >= 2, let a = parts[0].first, let b = parts[1].first { return String(a) + String(b) }
        return String(name.prefix(name.first?.isASCII == true ? 2 : 1)).uppercased()
    }

    var body: some View {
        ZStack {
            if let img = PhotoCodec.image(from: photo) {
                Image(uiImage: img).resizable().scaledToFill()
            } else if let s = photo.stringValue, s.hasPrefix("http"), let url = URL(string: s) {
                AsyncImage(url: url) { $0.resizable().scaledToFill() } placeholder: { placeholder }
            } else {
                placeholder
            }
        }
        .frame(width: diameter, height: diameter)
        .clipShape(Circle())
        .overlay(Circle().strokeBorder(.quaternary, lineWidth: 1))
        .frame(maxWidth: .infinity)
        .accessibilityLabel(c.name ?? "avatar")
    }

    private var placeholder: some View {
        ZStack {
            Circle().fill(RenderTheme.Colors.brandSoft)
            if initials.isEmpty {
                Image(systemName: "person.fill").font(.system(size: diameter * 0.45)).foregroundStyle(RenderTheme.Colors.brandA.opacity(0.7))
            } else {
                Text(initials).font(.system(size: diameter * 0.4, weight: .semibold, design: .rounded)).foregroundStyle(RenderTheme.Colors.brandText)
            }
        }
    }
}
