import UIKit
import MetamonDSL

/// aiButton `inputPhoto`: フォームの写真 1 枚を縮めて `POST /v1/text` の `image` に付ける(オンライン AI だけ)。
/// JPEG・長い辺 1024px 以下・base64 400 KB 以下(backend `text_image.go` と同じ上限)。
enum AIPhotoInput {
    enum PhotoError: LocalizedError, Equatable {
        case noPhoto
        case unreadable
        /// 写真の置き場(Firebase Storage)以外の https URL
        case untrustedSource
        case tooLarge

        var errorDescription: String? {
            switch self {
            case .noPhoto: "先に写真を撮るか選んでください"
            case .unreadable: "写真を読み込めませんでした。撮り直してからお試しください"
            case .untrustedSource: "この写真は読み込めない場所にあります。撮り直すか選び直してください"
            case .tooLarge: "写真が大きすぎます(10 MB まで)。撮り直すか選び直してください"
            }
        }
    }

    /// https の写真を読む先。iOS が上げる Firebase Storage(`users/{uid}/apps/{appId}/photos/…`)だけ。
    static let allowedHosts: Set<String> = ["firebasestorage.googleapis.com", "storage.googleapis.com"]
    static let maxDownloadBytes = 10 << 20

    static func isAllowed(_ url: URL) -> Bool {
        guard url.scheme?.lowercased() == "https", let host = url.host?.lowercased(),
              url.user == nil, url.password == nil, url.port == nil || url.port == 443 else { return false }
        return allowedHosts.contains(host)
    }

    /// 上限を超えたら読むのをやめる(全部落としてから大きさを見ない)。
    static func readCapped<S: AsyncSequence>(_ bytes: S, limit: Int = maxDownloadBytes) async throws -> Data where S.Element == UInt8 {
        var data = Data()
        for try await b in bytes {
            guard data.count < limit else { throw PhotoError.tooLarge }
            data.append(b)
        }
        return data
    }

    /// 許可した置き場の外へのリダイレクトは追わない。
    private final class RedirectGuard: NSObject, URLSessionTaskDelegate {
        func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                        newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
            completionHandler(request.url.map(AIPhotoInput.isAllowed) == true ? request : nil)
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

    /// photo field の値(data URL / https)から画像を取り出す。https は写真の置き場(`allowedHosts`)だけ・10 MB まで読む。
    static func image(from value: JSONValue) async throws -> UIImage {
        if let img = PhotoCodec.image(from: value) { return img }
        guard let s = value.stringValue, s.lowercased().hasPrefix("https://") else { throw PhotoError.unreadable }
        guard let url = URL(string: s), isAllowed(url) else { throw PhotoError.untrustedSource }
        let data: Data
        do {
            let (bytes, response) = try await URLSession.shared.bytes(from: url, delegate: RedirectGuard())
            guard let http = response as? HTTPURLResponse, http.statusCode == 200,
                  http.url.map(isAllowed) ?? false else { throw PhotoError.unreadable }
            if http.expectedContentLength > Int64(maxDownloadBytes) { throw PhotoError.tooLarge }
            data = try await readCapped(bytes)
        } catch let e as PhotoError {
            throw e
        } catch {
            throw PhotoError.unreadable
        }
        guard let img = UIImage(data: data) else { throw PhotoError.unreadable }
        return img
    }

    /// `c.inputPhoto` があれば、フォームの写真を縮めて依頼に付ける。無ければ何もしない。
    static func attach(_ c: Component, values: [String: JSONValue], to request: inout AITextRequest) async throws {
        guard let id = c.inputPhoto else { return }
        guard let value = values[id], value != .null, !(value.stringValue ?? "").isEmpty else { throw PhotoError.noPhoto }
        let img = try await image(from: value)
        guard let b64 = jpegBase64(from: img) else { throw PhotoError.unreadable }
        request.image = AITextImage(jpegBase64: b64)
    }
}
