package jp.kz2601.metamon.account

import jp.kz2601.metamon.services.local.AppListCache
import jp.kz2601.metamon.services.local.DocumentCache
import jp.kz2601.metamon.services.local.RecordSyncEngine

/**
 * この端末に残る前の人のデータを全部消す(#841・iOS の `AppRuntime.wipeLocalData`)。サーバーには何も送らない。
 * 表示の言語(端末の設定)は残す(iOS の `LocalWipe.preservedKeys` と同じ)。
 *
 * 消すもの(iOS との対応):
 * - 記録のローカル DB・送信待ち・取り消し待ちの削除([RecordSyncEngine.wipe] = iOS `recordSync.wipe()`)
 * - 保存した DSL([DocumentCache.removeAll] = iOS `DocumentCache.shared.removeAll()`)
 * - アプリの一覧([AppListCache.clear] = iOS `DocumentCache` の一覧・`apps = []`)
 * - 権限の記録(聞いた結果。[forgetDeviceState] = iOS `LocalWipe.clearDefaults`)
 * - 画面の状態・appId(新しい appId に。[resetRuntime] = iOS `recordUndo = nil`・`resetToInitial()`・`DeviceAppID.set`)
 * - 音声の画面の状態は [AccountGuard.generation] で作り直す(iOS `chatHistory = []`)
 */
class LocalDataWipe(
    private val recordSync: RecordSyncEngine,
    private val documentCache: DocumentCache,
    private val appList: AppListCache,
    private val resetRuntime: () -> Unit,
    private val forgetDeviceState: () -> Unit,
) {
    suspend operator fun invoke() {
        recordSync.wipe() // 最初に止める(前の人の送信待ちを次の人に送らない)
        documentCache.removeAll()
        appList.clear()
        forgetDeviceState()
        resetRuntime() // 一覧・DSL を消した後に新しい appId へ
    }
}
