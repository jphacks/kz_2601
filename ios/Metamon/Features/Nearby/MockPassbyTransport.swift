// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import Foundation
import MetamonDSL
import MetamonRenderer

/// `METAMON_MOCK=1`(シミュレータ)用: MultipeerConnectivity / UWB を使わず、1.5 秒後に「テスト花子」とすれ違ったことにする。
/// distance の時は距離と方向を 1 秒ごとに動かす。受け取りの保存・重複除けは本物と同じ経路を通る。
@MainActor
final class MockPassbyTransport: PassbyTransport {
    static let peerId = "mock-hanako"
    static let peerName = "テスト花子"

    var onPayload: ((NearbyPassby.Payload) -> Void)?
    var onPeers: (([PassbyPeer]) -> Void)?
    var onNotice: ((String?) -> Void)?
    var distanceSupport: PassbyDistanceSupport { .supported }

    private var task: Task<Void, Never>?

    func start(_ start: PassbyStart) {
        stop()
        task = Task { @MainActor [weak self] in
            try? await Task.sleep(for: .seconds(1.5))
            guard let self, !Task.isCancelled else { return }
            let mine = start.payload() // 送る側の組み立ても通す
            print("[Metamon] passby mock: would send \(mine.records.count) records")
            if start.shouldExchange(Self.peerId) {
                self.onPayload?(Self.payload(for: start))
            }
            guard start.config.distance else { return }
            var t: Float = 0
            while !Task.isCancelled {
                let meters = 1.6 + 1.2 * sin(t)
                self.onPeers?([PassbyPeer(id: Self.peerId, name: Self.peerName, distance: meters, azimuth: 0.9 * sin(t * 0.7))])
                t += 0.5
                try? await Task.sleep(for: .seconds(1))
            }
        }
    }

    func stop() {
        task?.cancel()
        task = nil
        onPeers?([])
    }

    /// 相手の記録 1 件(許可 field に型に合う見本の値)。
    static func payload(for start: PassbyStart) -> NearbyPassby.Payload {
        var data: [String: JSONValue] = [:]
        for (i, f) in NearbyPassby.allowedFields(start.config, in: start.entity).enumerated() {
            switch f.type {
            case .integer, .number, .currency: data[f.id] = .number(20)
            case .boolean: data[f.id] = .bool(true)
            case .date: data[f.id] = .string(String(ISO8601DateFormatter().string(from: .now).prefix(10)))
            case .datetime: data[f.id] = .string(ISO8601DateFormatter().string(from: .now))
            case .enum: data[f.id] = f.options?.first.map(JSONValue.string)
            case .location: data[f.id] = .string("35.6812,139.7671")
            default: data[f.id] = .string(i == 0 ? peerName : "よろしくお願いします")
            }
        }
        return NearbyPassby.Payload(v: NearbyPassby.version, senderId: peerId, name: peerName, entityId: start.entity.id,
                                    records: [.init(id: "mock-1", data: data)])
    }
}
#endif
