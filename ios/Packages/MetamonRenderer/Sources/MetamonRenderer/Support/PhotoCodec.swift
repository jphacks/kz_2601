import SwiftUI
import MetamonDSL

/// photo field の値表現("data:image/jpeg;base64,..." / https URL)と UIImage の相互変換。
public enum PhotoCodec {
    public static func dataURL(from data: Data, maxSide: CGFloat = 800, quality: CGFloat = 0.5) -> String? {
        guard let image = UIImage(data: data) else { return nil }
        let scale = min(1, maxSide / max(image.size.width, image.size.height))
        let size = CGSize(width: image.size.width * scale, height: image.size.height * scale)
        let resized = UIGraphicsImageRenderer(size: size).image { _ in image.draw(in: CGRect(origin: .zero, size: size)) }
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
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }
}
