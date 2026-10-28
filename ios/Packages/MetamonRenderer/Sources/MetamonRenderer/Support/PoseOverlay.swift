import SwiftUI
import MetamonDSL

/// 写真に骨格(関節の点と線)を重ねて描く(#418)。写真は縦横比を保って枠に収める(切り取ると点がずれるため)。
struct PosePhotoView: View {
    let image: UIImage
    let pose: BodyPose
    var height: CGFloat

    var body: some View {
        GeometryReader { geo in
            let rect = Self.fittedRect(imageSize: image.size, in: geo.size)
            ZStack(alignment: .topLeading) {
                Image(uiImage: image).resizable().frame(width: rect.width, height: rect.height).offset(x: rect.minX, y: rect.minY)
                Canvas { ctx, _ in
                    var bones = Path()
                    for (a, b) in PoseKeypoint.bones {
                        guard let pa = visible(a), let pb = visible(b) else { continue }
                        bones.move(to: Self.point(pa, in: rect))
                        bones.addLine(to: Self.point(pb, in: rect))
                    }
                    ctx.stroke(bones, with: .color(.white.opacity(0.9)), style: StrokeStyle(lineWidth: 4, lineCap: .round))
                    ctx.stroke(bones, with: .color(.accentColor), style: StrokeStyle(lineWidth: 2, lineCap: .round))
                    for key in PoseKeypoint.allCases {
                        guard let p = visible(key) else { continue }
                        let c = Self.point(p, in: rect)
                        let dot = Path(ellipseIn: CGRect(x: c.x - 4, y: c.y - 4, width: 8, height: 8))
                        ctx.fill(dot, with: .color(.accentColor))
                        ctx.stroke(dot, with: .color(.white), lineWidth: 1.5)
                    }
                }
            }
        }
        .frame(height: height)
        .frame(maxWidth: .infinity)
        .background(Color.black.opacity(0.06))
        .clipShape(RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(L("関節の位置を重ねた写真"))
        .accessibilityAddTraits(.isImage)
    }

    private func visible(_ key: PoseKeypoint) -> PosePoint? {
        pose[key].flatMap { $0.confidence >= PoseAngle.minConfidence ? $0 : nil }
    }

    /// 枠に縦横比を保って収めた画像の矩形。
    static func fittedRect(imageSize: CGSize, in size: CGSize) -> CGRect {
        guard imageSize.width > 0, imageSize.height > 0, size.width > 0, size.height > 0 else { return .zero }
        let scale = min(size.width / imageSize.width, size.height / imageSize.height)
        let w = imageSize.width * scale, h = imageSize.height * scale
        return CGRect(x: (size.width - w) / 2, y: (size.height - h) / 2, width: w, height: h)
    }

    /// Vision の正規化座標(左下原点)→ 画面の座標(左上原点)。
    static func point(_ p: PosePoint, in rect: CGRect) -> CGPoint {
        CGPoint(x: rect.minX + p.x * rect.width, y: rect.minY + (1 - p.y) * rect.height)
    }
}
