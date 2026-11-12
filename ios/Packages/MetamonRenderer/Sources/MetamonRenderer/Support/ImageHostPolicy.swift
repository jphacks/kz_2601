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

/// https の画像を読む。リダイレクトの先も [ImageHostPolicy] で確かめ、1 枚 [maxBytes] まで(Android の RemoteImages と同じ)。
/// 大きさは宣言(Content-Length)で先に断り、宣言が無くても受けた量が超えた時点で止める。どちらも画像にはしない。
/// キャッシュは画像専用の [cache](メモリ 16 MB・ディスク 64 MB)。サインアウト・アカウント削除で [removeAllCached] する。
public enum RemoteImageLoader {
    static let maxBytes = 10 * 1024 * 1024
    static let memoryCapacity = 16 * 1024 * 1024
    static let diskCapacity = 64 * 1024 * 1024

    static let cache: URLCache = {
        let dir = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask).first?
            .appendingPathComponent("metamon-images", isDirectory: true)
        return URLCache(memoryCapacity: memoryCapacity, diskCapacity: diskCapacity, directory: dir)
    }()

    static let session: URLSession = {
        let config = URLSessionConfiguration.default
        config.urlCache = cache
        return URLSession(configuration: config)
    }()

    /// 画像のキャッシュ(メモリ・ディスク)を消す。端末のデータ削除の一部。
    public static func removeAllCached() {
        cache.removeAllCachedResponses()
    }

    /// 宣言された大きさが上限を超えるか(不明 = -1 は受けながら数える)。
    static func exceeds(_ length: Int64, limit: Int = maxBytes) -> Bool { length > Int64(limit) }

    static func image(from url: URL) async -> PlatformImage? {
        guard let data = await data(from: url) else { return nil }
        return PlatformImage(data: data)
    }

    /// 上限内で読み切れた本文。https・公開のホスト・2xx 以外、上限超えは nil。
    static func data(from url: URL, session: URLSession = session, limit: Int = maxBytes) async -> Data? {
        guard ImageHostPolicy.isLoadable(url) else { return nil }
        let load = CappedLoad(limit: limit)
        let task = session.dataTask(with: url)
        task.delegate = load
        return await withTaskCancellationHandler {
            await withCheckedContinuation { continuation in
                load.start(task, continuation: continuation)
            }
        } onCancel: {
            task.cancel()
        }
    }

    /// 1 回分の読み込み。受けた量を数え、上限を超えたら止める。
    private final class CappedLoad: NSObject, URLSessionDataDelegate, @unchecked Sendable {
        private let limit: Int
        private let lock = NSLock()
        private var buffer = Data()
        private var rejected = false
        private var continuation: CheckedContinuation<Data?, Never>?

        init(limit: Int) { self.limit = limit }

        func start(_ task: URLSessionDataTask, continuation: CheckedContinuation<Data?, Never>) {
            lock.withLock { self.continuation = continuation }
            task.resume()
        }

        private func finish(_ data: Data?) {
            let c = lock.withLock { () -> CheckedContinuation<Data?, Never>? in
                defer { continuation = nil }
                return continuation
            }
            c?.resume(returning: data)
        }

        func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                        newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
            completionHandler(request.url.map(ImageHostPolicy.isLoadable) == true ? request : nil)
        }

        func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive response: URLResponse,
                        completionHandler: @escaping (URLSession.ResponseDisposition) -> Void) {
            guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode),
                  http.url.map(ImageHostPolicy.isLoadable) ?? false,
                  !RemoteImageLoader.exceeds(response.expectedContentLength, limit: limit) else {
                lock.withLock { rejected = true }
                completionHandler(.cancel)
                return
            }
            completionHandler(.allow)
        }

        func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive data: Data) {
            let over = lock.withLock { () -> Bool in
                guard !rejected else { return false }
                buffer.append(data)
                if buffer.count > limit {
                    rejected = true
                    buffer = Data()
                    return true
                }
                return false
            }
            if over { dataTask.cancel() }
        }

        func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
            let result = lock.withLock { () -> Data? in
                error == nil && !rejected ? buffer : nil
            }
            finish(result)
        }
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
