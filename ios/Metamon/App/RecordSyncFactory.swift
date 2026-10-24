import Foundation
import MetamonServices

/// `AppRuntime` が使うレコード同期エンジンの組み立て。通常は appconfig クライアント + `NWPathMonitor`。
/// DEBUG で `METAMON_E2E_OFFLINE_SECONDS=<n>` があれば、起動から n 秒間を機内モード扱いにする
/// (レコード API を通信断で落とし、到達性も offline → n 秒後に online に切り替える)。シミュレータ E2E 用。
@MainActor
enum RecordSyncFactory {
    static func make(client: any AppConfigClient) -> RecordSyncEngine {
        #if DEBUG
        if let raw = ProcessInfo.processInfo.environment["METAMON_E2E_OFFLINE_SECONDS"], let seconds = Double(raw), seconds > 0 {
            let until = Date.now.addingTimeInterval(seconds)
            let gated = OfflineSimulatingAppConfigClient(inner: client, offlineUntil: until)
            let connectivity = ManualConnectivity(online: false)
            print("[Metamon] E2E: records offline for \(Int(seconds))s")
            Task {
                try? await Task.sleep(for: .seconds(seconds))
                print("[Metamon] E2E: back online")
                connectivity.set(online: true)
            }
            return RecordSyncEngine(client: gated, connectivity: connectivity)
        }
        #endif
        return RecordSyncEngine(client: client)
    }
}
