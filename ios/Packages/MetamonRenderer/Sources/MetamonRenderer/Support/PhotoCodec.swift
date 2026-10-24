import SwiftUI
import MetamonDSL

/// photo field の値表現("data:image/jpeg;base64,..." / https URL)と UIImage の相互変換。
public enum PhotoCodec {
    public static func dataURL(from data: Data, maxSide: CGFloat = 800, quality: CGFloat = 0.5) -> String? {
        UIImage(data: data).flatMap { dataURL(from: $0, maxSide: maxSide, quality: quality) }
    }

    /// カメラ撮影の UIImage を直接縮小・エンコード(再デコード不要)。EXIF 回転は draw で正立化される。
    public static func dataURL(from image: UIImage, maxSide: CGFloat = 800, quality: CGFloat = 0.5) -> String? {
        // ピクセル基準で縮小。renderer の既定 scale は画面倍率(3x)なので 1 に固定しないと 800pt=2400px になる。
        let pixel = CGSize(width: image.size.width * image.scale, height: image.size.height * image.scale)
        let ratio = min(1, maxSide / max(pixel.width, pixel.height))
        let size = CGSize(width: (pixel.width * ratio).rounded(.down), height: (pixel.height * ratio).rounded(.down))
        let format = UIGraphicsImageRendererFormat.default()
        format.scale = 1
        format.opaque = true
        let resized = UIGraphicsImageRenderer(size: size, format: format).image { _ in image.draw(in: CGRect(origin: .zero, size: size)) }
        guard let jpeg = resized.jpegData(compressionQuality: quality) else { return nil }
        return "data:image/jpeg;base64," + jpeg.base64EncodedString()
    }

    public static func image(from value: JSONValue) -> UIImage? {
        guard let s = value.stringValue, s.hasPrefix("data:"), let comma = s.firstIndex(of: ",") else { return nil }
        return Data(base64Encoded: String(s[s.index(after: comma)...])).flatMap(UIImage.init(data:))
    }
}

/// data URL / https / 空 を描く共通の写真枡。
struct PhotoView: View {
    let value: JSONValue
    var height: CGFloat
    /// VoiceOver の読み上げ(省略時は「写真」。プレースホルダは「写真なし」)
    var accessibilityLabel: String? = nil

    private var hasPhoto: Bool {
        PhotoCodec.image(from: value) != nil || (value.stringValue?.hasPrefix("http") ?? false)
    }

    var body: some View {
        Group {
            if let img = PhotoCodec.image(from: value) {
                Image(uiImage: img).resizable().scaledToFill()
            } else if let s = value.stringValue, s.hasPrefix("http"), let url = URL(string: s) {
                AsyncImage(url: url) { $0.resizable().scaledToFill() } placeholder: { Rectangle().fill(.quaternary) }
            } else {
                Rectangle().fill(.quaternary).overlay(Image(systemName: "photo").foregroundStyle(.secondary))
            }
        }
        .frame(height: height)
        .frame(maxWidth: .infinity)
        .clipShape(RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(accessibilityLabel ?? (hasPhoto ? "写真" : "写真なし"))
        .accessibilityAddTraits(hasPhoto ? .isImage : [])
    }
}
