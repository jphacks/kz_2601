import Foundation
import BackgroundTasks
import MetamonDSL
import MetamonServices

/// バックグラウンドまわり(#38)。
/// - 前面復帰: 未送信キューを流し、DSL と表示中の records を即取り直す(背面中は SSE が止まっているので取りこぼしを埋める)
/// - 裏の定期同期: `BGAppRefreshTask`(OS が都合の良い時に最短 15 分間隔で起こす。保証はない)
extension AppRuntime {
    /// 前面に戻った直後に 1 回。`setForeground(true)` から呼ぶ。
    func catchUpAfterBackground() async {
        await syncAll(reason: "foreground")
    }

    /// `BGAppRefreshTask` の本体。OS の持ち時間(約 30 秒)内で終わる量だけやる。
    func backgroundRefresh() async {
        await syncAll(reason: "bg-refresh")
    }

    /// 未送信 → DSL → 表示中の records の順。どれかが失敗しても残りは続ける(ローカル表示はそのまま)。
    private func syncAll(reason: String) async {
        let pendingBefore = await recordSync.queue.count
        await recordSync.queue.resume()
        // 前面復帰では RecordSyncEngine.setForeground も同時に流し始めるので、そちらの送信が終わるまで待つ(最大 10 秒)
        var waited = 0
        while waited < 100, await recordSync.queue.isDraining {
            try? await Task.sleep(for: .milliseconds(100))
            waited += 1
        }
        let pendingAfter = await recordSync.queue.count
        print("[Metamon] background(\(reason)): queue sent=\(pendingBefore - pendingAfter) pending=\(pendingAfter)")

        let appId = document.appId
        if services.isLive {
            do {
                if let doc = try await services.appConfig.fetchDocumentIfChanged(appId: appId, currentVersion: document.version),
                   document.appId == appId {
                    hotReload(doc) // 操作中なら閉じるまで保留される
                }
            } catch {
                print("[Metamon] background(\(reason)): dsl fetch failed: \(error.localizedDescription)")
            }
        }
        let entities = Array(records.keys)
        for entity in entities where document.appId == appId { await refresh(entity) }
        print("[Metamon] background(\(reason)): done version=\(document.version) entities=\(entities.sorted())")
    }
}

/// `BGTaskScheduler` の予約。識別子は Info.plist の `BGTaskSchedulerPermittedIdentifiers` と一致させる。
enum BackgroundRefresh {
    static let identifier = "jp.kz2601.metamon.refresh"
    /// OS への希望(これより早くは起こさない)。実際の間隔は OS が利用状況から決める。
    static let interval: TimeInterval = 15 * 60

    static func schedule(now: Date = .now) {
        let request = BGAppRefreshTaskRequest(identifier: identifier)
        request.earliestBeginDate = now.addingTimeInterval(interval)
        do {
            try BGTaskScheduler.shared.submit(request)
            print("[Metamon] background: refresh scheduled earliest=\(request.earliestBeginDate!)")
        } catch {
            // シミュレータは常に失敗する(BGTaskSchedulerErrorDomain 1 = unavailable)。lldb の _simulateLaunch で確認する
            print("[Metamon] background: refresh schedule failed: \(error)")
        }
    }
}
