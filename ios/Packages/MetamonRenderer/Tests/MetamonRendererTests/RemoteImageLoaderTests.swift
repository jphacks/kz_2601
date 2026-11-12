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
        // 回ごとに別の URL にして、前の回の書き込み・削除の残りに左右されないようにする
        let key = URL(string: "https://images.example.com/\(UUID().uuidString).png")!
        let request = URLRequest(url: key)
        let response = HTTPURLResponse(url: key, statusCode: 200, httpVersion: "HTTP/1.1", headerFields: nil)!
        RemoteImageLoader.cache.storeCachedResponse(CachedURLResponse(response: response, data: Data([1, 2, 3])), for: request)
        // 保存も削除も裏で進み、終わりを知らせる口が無い。決め打ちで待たず、期限つきで状態を見に行く
        XCTAssertTrue(waitUntil { RemoteImageLoader.cache.cachedResponse(for: request) != nil }, "保存が期限内に見えない")

        // 初めて使うキャッシュでは、見えた後もディスクへの書き込みが裏に残り、消した後に書き戻ることがある(CI で再現)。
        // 本番の後片付けは画像を見終えた後なので、書き終わるまで消し直して「最後は空になる」ことを確かめる
        XCTAssertTrue(waitUntil {
            RemoteImageLoader.removeAllCached()
            return RemoteImageLoader.cache.cachedResponse(for: request) == nil
        }, "削除が期限内に終わらない")
    }

    /// 条件が満たされるまで短い間隔で見に行く。期限までに満たされなければ false。
    private func waitUntil(timeout: TimeInterval = 10, _ condition: () -> Bool) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        while !condition() {
            if Date() >= deadline { return false }
            Thread.sleep(forTimeInterval: 0.01)
        }
        return true
    }

    func testImageSessionUsesTheImageCache() {
        XCTAssertTrue(RemoteImageLoader.session.configuration.urlCache === RemoteImageLoader.cache)
        XCTAssertEqual(RemoteImageLoader.cache.diskCapacity, 64 * 1024 * 1024)
    }
}
