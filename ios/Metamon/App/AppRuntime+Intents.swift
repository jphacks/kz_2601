import SwiftUI
import MetamonDSL
import MetamonServices

/// Siri / ショートカット(App Intents・#80)から呼ぶ入口。中身は既存の経路(変身一覧の切替・音声シート・ローカル DB)をそのまま使う。
extension AppRuntime {
    /// 「Tailor It で◯◯を開く」。今のアプリならそのまま、別なら変身一覧と同じ切替。
    func openFromIntent(appId: String) async {
        showAppSwitcher = false
        guard appId != document.appId else {
            summaryBanner = SummaryBanner(text: "いま使っているのが「\(dsl.app.name)」です", highlight: nil)
            return
        }
        await switchApp(to: appId)
    }

    /// 「Tailor It で話しかける」。生成済みのアプリなら音声シート、初期画面なら音声画面そのものなので何もしない。
    func openVoiceFromIntent() {
        showAppSwitcher = false
        sheet = nil
        fullScreen = nil
        guard !dsl.isVoiceOnly else { return }
        showVoiceSheet = true
    }

    /// 「記録を追加」。画面の保存と同じくローカル DB に即書いて SyncQueue に積む(appId は今のアプリでなくてもよい)。
    /// Intent が返った後に背面プロセスが止められても行とキューはファイルに残り、次回起動で送られる。
    /// 戻り値: 返す前にサーバーへ送り終えたか(最大 `wait` 待つ。オフラインなら即 false)。
    func addRecordFromIntent(appId: String, entity: String, data: [String: JSONValue], wait: Duration = .seconds(4)) async -> Bool {
        recordSync.start() // 背面起動では bootstrap 前なので、ここで到達性とキューの購読を始める(2 回目以降は何もしない)
        _ = await recordSync.save(appId: appId, entity: entity, recordId: nil, data: data)
        await reloadLocal(entity, appId: appId)
        print("[Metamon] intent: record added appId=\(appId) entity=\(entity) data=\(data)")
        guard recordSync.connectivity.isOnline else { return false }
        let deadline = ContinuousClock.now + wait
        while ContinuousClock.now < deadline {
            if await recordSync.queue.count == 0 { return true }
            try? await Task.sleep(for: .milliseconds(200))
        }
        return false
    }
}
