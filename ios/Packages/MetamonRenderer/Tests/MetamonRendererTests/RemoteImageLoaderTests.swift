import XCTest
@testable import MetamonRenderer

/// 画像は 1 枚 10 MB まで(Android の RemoteImagesTest と同じ決まり)・キャッシュは後片付けで消す(#1265)。
final class RemoteImageLoaderTests: XCTestCase {
    /// 本文を小分けで返す URLProtocol スタブ。Content-Length は宣言する時だけ付ける。
    final class ChunkStub: URLProtocol {
        struct Canned {
            var status = 200
            var chunks: [Data]
            var declaredLength: Int?
        }
        nonisolated(unsafe) static var canned = Canned(chunks: [])
        nonisolated(unsafe) static var delivered = 0
        static let lock = NSLock()

        override class func canInit(with request: URLRequest) -> Bool { true }
        override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

        override func startLoading() {
            let c = Self.lock.withLock { Self.canned }
            var headers = ["Content-Type": "image/png"]
            if let n = c.declaredLength { headers["Content-Length"] = String(n) }
            let response = HTTPURLResponse(url: request.url!, statusCode: c.status, httpVersion: "HTTP/1.1", headerFields: headers)!
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            for chunk in c.chunks {
                Self.lock.withLock { Self.delivered += chunk.count }
                client?.urlProtocol(self, didLoad: chunk)
            }
            client?.urlProtocolDidFinishLoading(self)
        }

        override func stopLoading() {}
    }

    private let url = URL(string: "https://images.example.com/a.png")!

    private func session(_ canned: ChunkStub.Canned) -> URLSession {
        ChunkStub.lock.withLock {
            ChunkStub.canned = canned
            ChunkStub.delivered = 0
        }
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [ChunkStub.self]
        return URLSession(configuration: config)
    }

    func testLimitIsTenMegabytes() {
        XCTAssertEqual(RemoteImageLoader.maxBytes, 10 * 1024 * 1024)
        XCTAssertFalse(RemoteImageLoader.exceeds(Int64(RemoteImageLoader.maxBytes)))
        XCTAssertTrue(RemoteImageLoader.exceeds(Int64(RemoteImageLoader.maxBytes) + 1))
        XCTAssertFalse(RemoteImageLoader.exceeds(-1), "大きさ不明は受けながら数える")
    }

    func testBodyWithinLimitIsReturned() async {
        let body = Data(repeating: 7, count: 1_000)
        let s = session(.init(chunks: [body.prefix(400), body.suffix(600)], declaredLength: 1_000))
        let data = await RemoteImageLoader.data(from: url, session: s, limit: 1_000)
        XCTAssertEqual(data, body)
    }

    func testDeclaredLengthOverLimitIsRejectedBeforeBody() async {
        let s = session(.init(chunks: [Data(repeating: 1, count: 10)], declaredLength: 1_001))
        let data = await RemoteImageLoader.data(from: url, session: s, limit: 1_000)
        XCTAssertNil(data)
    }

    func testUndeclaredBodyOverLimitIsStoppedWhileStreaming() async {
        let chunk = Data(repeating: 1, count: 600)
        let s = session(.init(chunks: [chunk, chunk, chunk], declaredLength: nil))
        let data = await RemoteImageLoader.data(from: url, session: s, limit: 1_000)
        XCTAssertNil(data, "宣言が無くても上限を超えたら画像にしない")
    }

    func testNonSuccessStatusIsRejected() async {
        let s = session(.init(status: 404, chunks: [Data(repeating: 1, count: 10)], declaredLength: 10))
        let data = await RemoteImageLoader.data(from: url, session: s, limit: 1_000)
        XCTAssertNil(data)
    }

    func testHttpIsNotRequested() async {
        let s = session(.init(chunks: [Data(repeating: 1, count: 10)], declaredLength: 10))
        let data = await RemoteImageLoader.data(from: URL(string: "http://images.example.com/a.png")!, session: s, limit: 1_000)
        XCTAssertNil(data)
        XCTAssertEqual(ChunkStub.lock.withLock { ChunkStub.delivered }, 0)
    }

    func testRemoveAllCachedEmptiesTheImageCache() {
        let request = URLRequest(url: url)
        let response = HTTPURLResponse(url: url, statusCode: 200, httpVersion: "HTTP/1.1", headerFields: nil)!
        RemoteImageLoader.cache.storeCachedResponse(CachedURLResponse(response: response, data: Data([1, 2, 3])), for: request)
        Thread.sleep(forTimeInterval: 0.5) // 保存は裏で書かれる。後片付けは画像を見た後なので書き終わった状態で試す
        XCTAssertNotNil(RemoteImageLoader.cache.cachedResponse(for: request))

        RemoteImageLoader.removeAllCached()
        Thread.sleep(forTimeInterval: 0.5) // 消すのも裏で進む

        XCTAssertNil(RemoteImageLoader.cache.cachedResponse(for: request))
    }

    func testImageSessionUsesTheImageCache() {
        XCTAssertTrue(RemoteImageLoader.session.configuration.urlCache === RemoteImageLoader.cache)
        XCTAssertEqual(RemoteImageLoader.cache.diskCapacity, 64 * 1024 * 1024)
    }
}
