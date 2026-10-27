#if DEBUG
import Foundation
import MetamonServices

/// UI テスト用の DEBUG フック `METAMON_MOCK_NEW_ITEMS=<n>`(`METAMON_MOCK=1` と併用・#268)。
/// 他の人のレコード n 件 + 自分のレコード 1 件(数えない)を返し、前回開いた時刻を 1 時間前にする。
enum DebugMockNewItems {
    static var count: Int? {
        ProcessInfo.processInfo.environment["METAMON_MOCK_NEW_ITEMS"].flatMap(Int.init)
    }

    static var source: (any SharedRecordStampSource)? { count.map(Source.init) }

    static func prepare(store: LastOpenedStore, appId: String) {
        guard count != nil else { return }
        store.mark(appId: appId, at: Date.now.addingTimeInterval(-3600))
    }

    struct Source: SharedRecordStampSource {
        let n: Int
        init(_ n: Int) { self.n = n }

        func sharedStamps(appId: String, entity: String, notBy uid: String) async throws -> [RecordStamp] {
            let now = ISO8601DateFormatter().string(from: .now)
            let others = (0..<n).map { RecordStamp(id: "\(entity)-other-\($0)", author: "uitest-other", createdAt: now) }
            return others + [RecordStamp(id: "\(entity)-mine", author: uid, createdAt: now)]
        }
    }
}
#endif
