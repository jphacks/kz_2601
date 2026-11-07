import UIKit
import MetamonDSL

/// aiButton `inputPhoto`: フォームの写真 1 枚を縮めて `POST /v1/text` の `image` に付ける(オンライン AI だけ)。
/// JPEG・長い辺 1024px 以下・base64 400 KB 以下(backend `text_image.go` と同じ上限)。
enum AIPhotoInput {
    enum PhotoError: LocalizedError, Equatable {
        case noPhoto
        case unreadable

        var errorDescription: String? {
            switch self {
            case .noPhoto: "先に写真を撮るか選んでください"
            case .unreadable: "写真を読み込めませんでした。撮り直してからお試しください"
            }
        }
    }

    /// 品質を下げ、それでも大きければ辺を縮めて、上限に収まる JPEG の base64 を返す。
    static func jpegBase64(from image: UIImage, maxSide: CGFloat = CGFloat(AITextLimits.maxImageSide),
                           maxBase64: Int = AITextLimits.maxImageBase64) -> String? {
        var side = maxSide
        while side >= 256 {
            for quality in [0.7, 0.55, 0.4, 0.3] as [CGFloat] {
                guard let url = PhotoCodec.dataURL(from: image, maxSide: side, quality: quality),
                      let comma = url.firstIndex(of: ",") else { return nil }
                let b64 = String(url[url.index(after: comma)...])
                if b64.count <= maxBase64 { return b64 }
            }
            side = (side * 0.75).rounded(.down)
        }
        return nil
    }

    /// photo field の値(data URL / https)から画像を取り出す。https は 10 MB まで読む。
    static func image(from value: JSONValue) async -> UIImage? {
        if let img = PhotoCodec.image(from: value) { return img }
        guard let s = value.stringValue, s.hasPrefix("https://"), let url = URL(string: s),
              let (data, response) = try? await URLSession.shared.data(from: url),
              (response as? HTTPURLResponse)?.statusCode == 200, data.count <= 10 << 20 else { return nil }
        return UIImage(data: data)
    }

    /// `c.inputPhoto` があれば、フォームの写真を縮めて依頼に付ける。無ければ何もしない。
    static func attach(_ c: Component, values: [String: JSONValue], to request: inout AITextRequest) async throws {
        guard let id = c.inputPhoto else { return }
        guard let value = values[id], value != .null, !(value.stringValue ?? "").isEmpty else { throw PhotoError.noPhoto }
        guard let img = await image(from: value), let b64 = jpegBase64(from: img) else { throw PhotoError.unreadable }
        request.image = AITextImage(jpegBase64: b64)
    }
}
