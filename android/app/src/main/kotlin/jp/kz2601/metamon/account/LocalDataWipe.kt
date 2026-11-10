package jp.kz2601.metamon.account

import jp.kz2601.metamon.services.feeds.FeedCache
import jp.kz2601.metamon.services.local.AppListCache
import jp.kz2601.metamon.services.local.DocumentCache
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.social.AuthorProfile
import jp.kz2601.metamon.voice.ChatHistoryStore

/**
 * この端末に残る前の人のデータを全部消す(#841・iOS の `AppRuntime.wipeLocalData`)。サーバーには何も送らない。
 * 表示の言語(端末の設定)は残す(iOS の `LocalWipe.preservedKeys` と同じ)。
 *
 * 消すもの(iOS との対応):
 * - 記録のローカル DB・送信待ち・取り消し待ちの削除([RecordSyncEngine.wipe] = iOS `recordSync.wipe()`)
 * - 保存した DSL([DocumentCache.removeAll] = iOS `DocumentCache.shared.removeAll()`)
 * - アプリの一覧([AppListCache.clear] = iOS `DocumentCache` の一覧・`apps = []`)
 * - 権限の記録(聞いた結果。[forgetDeviceState] = iOS `LocalWipe.clearDefaults`)
 * - 音声の画面の読み上げの ON / OFF・起動回数・マイク FAB の位置・聞き返しの設定(#892・[forgetDeviceState] の中 = iOS `LocalWipe.clearDefaults`)
 * - 会話の履歴(#893・[ChatHistoryStore.removeAll] = iOS `ChatHistoryStore.removeAll()`)
 * - 投稿に表示する名前(#875・[AuthorProfile.clear])
 * - feed の覚えた行(#876・[FeedCache.removeAll] = iOS `FeedCache.removeAll()`)
 * - 端末内の動画(#863)・録音(#1084)のファイル([removeLocalMedia] = iOS `LocalVideoStore` / `LocalAudioStore` の removeAll)
 * - AI への同意・同意の前に止めた 1 件・aiButton の貯めた数・18 歳以上の確認(#877)([forgetConsent] = iOS `LocalWipe.clearDefaults` + `aiConsent.reload()`)
 * - チュートリアルと「こう話すと直せます」を見た印(#900・[forgetTutorial] = iOS `LocalWipe.clearDefaults` + `tutorial.reload()`)
 * - 他の人の行動で届く通知の同意の控え・登録の控え(installationId は残す)・閉じたお知らせ([forgetDeviceState] の中 = iOS `LocalWipe.clearDefaults`)
 * - 生成アプリの通知の予約・出ている通知([forgetDeviceState] の中 = iOS `notifications.removeAll()`)
 * - https の画像のキャッシュ(メモリ・ディスク。[clearImageCache] = iOS は URLCache)
 * - 画面の状態・appId(新しい appId に。[resetRuntime] = iOS `recordUndo = nil`・`resetToInitial()`・`DeviceAppID.set`)
 * - ブロック中の人・仲間探しの一覧(#877・[resetRuntime] の中の `SocialRuntime.forget`。iOS は次の人の uid で読み直すまで残るが、
 *   Android は前の人のブロックで次の人の投稿を隠さないよう先に捨てる)
 * - 音声の画面の状態は [AccountGuard.generation] で作り直す(iOS `chatHistory = []`)
 */
class LocalDataWipe(
    private val recordSync: RecordSyncEngine,
    private val documentCache: DocumentCache,
    private val appList: AppListCache,
    private val resetRuntime: () -> Unit,
    private val forgetDeviceState: () -> Unit,
    private val chatHistory: ChatHistoryStore? = null,
    private val authorProfile: AuthorProfile? = null,
    private val removeLocalMedia: () -> Unit = {},
    private val clearImageCache: () -> Unit = {},
    private val feedCache: FeedCache? = null,
    private val forgetConsent: () -> Unit = {},
    private val forgetTutorial: () -> Unit = {},
) {
    suspend operator fun invoke() {
        recordSync.wipe() // 最初に止める(前の人の送信待ちを次の人に送らない)
        documentCache.removeAll()
        appList.clear()
        forgetDeviceState()
        forgetConsent()
        forgetTutorial()
        authorProfile?.clear()
        chatHistory?.removeAll()
        removeLocalMedia()
        clearImageCache()
        feedCache?.removeAll()
        resetRuntime() // 一覧・DSL を消した後に新しい appId へ
    }
}
