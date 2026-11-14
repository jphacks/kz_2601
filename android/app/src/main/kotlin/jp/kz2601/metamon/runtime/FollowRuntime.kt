package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.usesFollows
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.services.follows.FollowStore
import jp.kz2601.metamon.services.follows.FollowsClient
import jp.kz2601.metamon.services.follows.FollowsReason
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.FailureKind
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * AppRuntime のフォロー(iOS の AppRuntime+Follows・#270 / #1028)。一覧はアプリ × サインイン中の人ごとに持つ。
 * follow / unfollow は楽観的に先に反映し、失敗したら戻して理由をトーストに出す。
 */
class FollowRuntime(
    private val client: FollowsClient?,
    private val scope: CoroutineScope,
    private val env: Env,
) {
    /** AppRuntime から借りるもの。 */
    interface Env {
        val appId: String
        val dsl: AppDSL

        fun currentUid(): String?

        fun records(entity: String): List<Record>

        fun toast(message: UiText)
    }

    private val stores = mutableMapOf<String, FollowStore>()
    private val watchers = mutableListOf<Job>()
    private val _version = MutableStateFlow(0)

    /** 一覧・送信中が変わるたびに進む(FollowsHost.followsVersion)。 */
    val version: StateFlow<Int> = _version.asStateFlow()

    /** このアプリのフォロー一覧。live で、フォローを使う DSL で、サインインしている時だけ。 */
    private val store: FollowStore?
        get() {
            val client = client ?: return null
            val me = env.currentUid()?.takeIf { it.isNotEmpty() } ?: return null
            if (!usesFollows()) return null
            val appId = env.appId
            return stores.getOrPut("$appId|$me") {
                FollowStore(appId, client).also { s ->
                    watchers += scope.launch { s.snapshot.collect { _version.update { it + 1 } } }
                }
            }
        }

    /** フォローしている人。まだ読めていなければ読みに行き、読めなければ空のまま(次に描いた時に読み直す)。 */
    fun followedAuthors(): Set<String>? {
        val store = store ?: return null
        if (store.uids == null) {
            scope.launch {
                try {
                    store.load()
                } catch (e: CancellationException) {
                    throw e
                } catch (_: Exception) {
                    // 次に開いた時に読み直す(iOS と同じく黙る)
                }
            }
        }
        return store.uids
    }

    fun followState(entity: String, recordId: String): Boolean? {
        val author = authorOf(entity, recordId) ?: return null
        val uids = followedAuthors() ?: return null
        return author in uids
    }

    /** 今開いている投稿(行から開いた画面・フォームのレコード)を書いた人をフォローする / 外す。 */
    suspend fun run(action: Action, ctx: ActionContext) {
        val store = store
        val entity = action.entity
        if (store == null || entity == null) return env.toast(FollowCopy.UNAVAILABLE)
        val author = ctx.currentRecordId?.let { authorOf(entity, it) } ?: return env.toast(FollowCopy.OPEN_POST_FIRST)
        val on = action.type == ActionType.FOLLOW
        try {
            store.set(author, on)
            env.toast(if (on) FollowCopy.FOLLOWED else FollowCopy.UNFOLLOWED)
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            env.toast(errorText(e))
        }
    }

    /** サインアウト・アカウント削除・別の人のサインイン。前の人の一覧を捨てる。 */
    fun forget() {
        watchers.forEach { it.cancel() }
        watchers.clear()
        stores.clear()
        _version.update { it + 1 }
    }

    /** DSL ごとに 1 回だけ全部品を見る(描画のたびに呼ばれる)。 */
    private var usesCache: Pair<AppDSL, Boolean>? = null

    private fun usesFollows(): Boolean {
        val dsl = env.dsl
        usesCache?.let { (d, v) -> if (d === dsl) return v }
        return dsl.usesFollows.also { usesCache = dsl to it }
    }

    /** 他人の投稿の作者(自分の投稿・作者の無い投稿は null)。 */
    private fun authorOf(entity: String, recordId: String): String? {
        val author = env.records(entity).firstOrNull { it.id == recordId }?.author?.takeIf { it.isNotEmpty() } ?: return null
        return author.takeIf { it != env.currentUid() }
    }

    companion object {
        /** 失敗の一言(iOS の `followErrorText`)。 */
        fun errorText(error: Throwable): UiText {
            when ((error as? ApiError)?.reason) {
                FollowsReason.FULL -> return FollowCopy.FULL
                FollowsReason.NOT_IN_APP -> return FollowCopy.NOT_IN_APP
            }
            if (FailureKind.classify(error) == FailureKind.Offline) return FollowCopy.OFFLINE
            return FollowCopy.FAILED
        }
    }
}

/** フォローのトースト(iOS の L10n キーと同じ)。文は strings_follow.xml。 */
object FollowCopy {
    val UNAVAILABLE = UiText.Res(R.string.follow_unavailable)
    val OPEN_POST_FIRST = UiText.Res(R.string.follow_open_post_first)
    val FOLLOWED = UiText.Res(R.string.follow_followed)
    val UNFOLLOWED = UiText.Res(R.string.follow_unfollowed)
    val FULL = UiText.Res(R.string.follow_full)
    val NOT_IN_APP = UiText.Res(R.string.follow_not_in_app)
    val OFFLINE = UiText.Res(R.string.follow_offline)
    val FAILED = UiText.Res(R.string.follow_failed)
}
