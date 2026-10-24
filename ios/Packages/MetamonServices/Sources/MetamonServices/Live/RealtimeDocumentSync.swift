import Foundation
import MetamonDSL

/// リアルタイム版のホットリロード(G13 Day2・#82)。
/// appconfig の `GET /v1/apps/{id}/events`(SSE)を購読し、届くのは「何が変わったか」だけ。本体は従来 API で取り直す(正は API)。
/// - `dsl {version}` → `GET /dsl`(If-None-Match)で取り直して `.document`
/// - `records {entity}` → `.records(entity:)`(呼び出し側が `pull` する)
/// - `bye` → サーバーの予定終了(Cloud Run timeout 対策で 50 秒ごと)。即再接続し、ポーリングには落とさない
/// - それ以外の切断 → `.realtime(false)` + 指数バックオフで再接続。その間は `fallbackInterval` でポーリング
/// ポーリングはリスナー稼働中も `listeningInterval`(60 秒)で保険として回す。
public struct RealtimeDocumentSync: DocumentSync {
    public let client: HTTPAppConfigClient
    public var fallbackInterval: Duration
    public var listeningInterval: Duration
    public var maxBackoff: Duration

    public init(client: HTTPAppConfigClient, fallbackInterval: Duration = .seconds(5),
                listeningInterval: Duration = .seconds(60), maxBackoff: Duration = .seconds(30)) {
        self.client = client
        self.fallbackInterval = fallbackInterval
        self.listeningInterval = listeningInterval
        self.maxBackoff = maxBackoff
    }

    public func updates(appId: String, after version: Int) -> AsyncStream<AppDocument> {
        let events = events(appId: appId, after: version)
        return AsyncStream { continuation in
            let task = Task {
                for await case .document(let doc) in events { continuation.yield(doc) }
                continuation.finish()
            }
            continuation.onTermination = { _ in task.cancel() }
        }
    }

    public func events(appId: String, after version: Int) -> AsyncStream<SyncUpdate> {
        AsyncStream { continuation in
            let state = State(version: version, continuation: continuation)
            let listener = Task { await self.listen(appId: appId, state: state) }
            let poller = Task { await self.poll(appId: appId, state: state) }
            continuation.onTermination = { _ in listener.cancel(); poller.cancel() }
        }
    }

    // MARK: - 共有状態

    actor State {
        private(set) var version: Int
        private(set) var listening = false
        private let continuation: AsyncStream<SyncUpdate>.Continuation

        init(version: Int, continuation: AsyncStream<SyncUpdate>.Continuation) {
            self.version = version
            self.continuation = continuation
        }

        func setListening(_ value: Bool) {
            guard listening != value else { return }
            listening = value
            continuation.yield(.realtime(value))
        }

        func offer(_ doc: AppDocument) {
            guard doc.version > version else { return }
            version = doc.version
            continuation.yield(.document(doc))
        }

        func recordsChanged(_ entity: String) { continuation.yield(.records(entity: entity)) }
    }

    private func refetch(appId: String, state: State) async {
        do {
            if let doc = try await client.fetchDocumentIfChanged(appId: appId, currentVersion: await state.version) {
                await state.offer(doc)
            }
        } catch {
            print("[Metamon] realtime: refetch failed: \(error.localizedDescription)")
        }
    }

    // MARK: - ポーリング(フォールバック)

    private func poll(appId: String, state: State) async {
        while !Task.isCancelled {
            let interval = await state.listening ? listeningInterval : fallbackInterval
            try? await Task.sleep(for: interval)
            guard !Task.isCancelled else { return }
            await refetch(appId: appId, state: state)
        }
    }

    // MARK: - SSE

    private func listen(appId: String, state: State) async {
        var backoff: Duration = .seconds(1)
        var forceRefresh = false
        while !Task.isCancelled {
            var planned = false
            do {
                planned = try await stream(appId: appId, state: state, forceRefresh: forceRefresh) {
                    backoff = .seconds(1)
                }
                forceRefresh = false
            } catch StreamError.unauthorized {
                forceRefresh = true
            } catch {
                if !Task.isCancelled { print("[Metamon] realtime: stream ended: \(error.localizedDescription)") }
            }
            guard !Task.isCancelled else { return }
            if planned { continue }
            await state.setListening(false)
            try? await Task.sleep(for: backoff)
            backoff = min(backoff * 2, maxBackoff)
        }
    }

    enum StreamError: Error { case unauthorized, status(Int) }

    /// 1 本の SSE を読み切る。`bye` で終わったら true(予定終了)。
    private func stream(appId: String, state: State, forceRefresh: Bool, onOpen: () -> Void) async throws -> Bool {
        guard let url = URL(string: "v1/apps/\(appId)/events", relativeTo: client.baseURL) else { throw APIError.invalidURL }
        // 心拍は 15 秒ごとなので、それより長く無音なら切れたとみなす
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalAndRemoteCacheData, timeoutInterval: 40)
        req.setValue("text/event-stream", forHTTPHeaderField: "Accept")
        if let bearer = try await client.token.idToken(forceRefresh: forceRefresh) {
            req.setValue("Bearer \(bearer)", forHTTPHeaderField: "Authorization")
        }
        let (bytes, resp) = try await HTTP.session.bytes(for: req)
        let status = (resp as? HTTPURLResponse)?.statusCode ?? -1
        if status == 401 { throw StreamError.unauthorized }
        guard status == 200 else { throw StreamError.status(status) }

        var event = ""
        // `lines` は空行を返さないので、event: の次の data: 行で 1 件として扱う(サーバーは常に 1 行 data)
        for try await line in bytes.lines {
            if line.hasPrefix("event:") {
                event = line.dropFirst(6).trimmingCharacters(in: .whitespaces)
                continue
            }
            guard line.hasPrefix("data:") else { continue }
            let payload = Payload.decode(line.dropFirst(5))
            switch event {
            case "hello":
                onOpen()
                await state.setListening(true)
                if let v = payload.version, v > (await state.version) { await refetch(appId: appId, state: state) }
            case "dsl":
                if let v = payload.version, v > (await state.version) { await refetch(appId: appId, state: state) }
            case "records":
                if let entity = payload.entity { await state.recordsChanged(entity) }
            case "bye":
                return true
            default:
                break
            }
            event = ""
        }
        return false
    }

    struct Payload: Decodable {
        var version: Int?
        var entity: String?

        static func decode(_ raw: Substring) -> Payload {
            let data = Data(raw.trimmingCharacters(in: .whitespaces).utf8)
            return (try? JSONDecoder().decode(Payload.self, from: data)) ?? Payload()
        }
    }
}
