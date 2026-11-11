import SwiftUI

/// DSL の画像を読む先は公開のホストの https だけ(Android の ImageHosts と同じ規則)。
/// IP の直書き・localhost・*.local・*.internal・*.home.arpa・ドットの無い名前は読まない(写真なしの枠になる)。
public enum ImageHostPolicy {
    static let maxURLLength = 2048
    private static let localSuffixes = ["localhost", "local", "internal", "home.arpa"]

    /// 読み込んでよい URL(https・公開のホスト・ID / パスワード無し)。それ以外は nil。
    public static func loadableURL(_ value: String?) -> URL? {
        guard let s = value?.trimmingCharacters(in: .whitespaces), !s.isEmpty, s.count <= maxURLLength,
              let url = URL(string: s) else { return nil }
        return isLoadable(url) ? url : nil
    }

    static func isLoadable(_ url: URL) -> Bool {
        guard url.scheme?.lowercased() == "https", url.user == nil, url.password == nil,
              let host = URLComponents(url: url, resolvingAgainstBaseURL: false)?.host else { return false }
        return isPublicName(host)
    }

    /// 名前の段階で読んでよいホストか。
    static func isPublicName(_ host: String) -> Bool {
        var h = host.lowercased()
        while h.hasSuffix(".") { h.removeLast() }
        if h.isEmpty || h.contains(":") || h.hasPrefix("[") { return false }
        let labels = h.split(separator: ".", omittingEmptySubsequences: false)
        if labels.count < 2 || labels.contains(where: \.isEmpty) { return false }
        // 最後が数字・0x で始まる名前は IPv4 の書き方(10 進 1 つ・8 / 16 進も含む)
        let last = labels[labels.count - 1]
        if last.allSatisfy(\.isASCIIDigitChar) || last.hasPrefix("0x") { return false }
        return !localSuffixes.contains { h == $0 || h.hasSuffix("." + $0) }
    }
}

private extension Character {
    var isASCIIDigitChar: Bool { ("0"..."9").contains(self) }
}

/// https の画像を読む。リダイレクトの先も [ImageHostPolicy] で確かめる。キャッシュは AsyncImage と同じ URLSession.shared。
enum RemoteImageLoader {
    private final class RedirectGuard: NSObject, URLSessionTaskDelegate {
        func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                        newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
            completionHandler(request.url.map(ImageHostPolicy.isLoadable) == true ? request : nil)
        }
    }

    static func image(from url: URL) async -> PlatformImage? {
        guard ImageHostPolicy.isLoadable(url),
              let (data, response) = try? await URLSession.shared.data(from: url, delegate: RedirectGuard()),
              let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode),
              http.url.map(ImageHostPolicy.isLoadable) ?? false else { return nil }
        return PlatformImage(data: data)
    }
}

/// AsyncImage の代わり(読む先を確かめる)。読み込み中・読めなかった時は placeholder。
struct RemoteImage<Placeholder: View>: View {
    let url: URL
    @ViewBuilder var placeholder: () -> Placeholder
    @State private var loaded: (url: URL, image: PlatformImage)?

    var body: some View {
        Group {
            if let loaded, loaded.url == url {
                Image(platformImage: loaded.image).resizable().scaledToFill()
            } else {
                placeholder()
            }
        }
        .task(id: url) {
            if let image = await RemoteImageLoader.image(from: url) { loaded = (url, image) }
        }
    }
}
