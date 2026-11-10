package jp.kz2601.metamon.services.discover

import jp.kz2601.metamon.services.InstallResult
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.social.ModerationClient
import jp.kz2601.metamon.services.social.ReportReason
import jp.kz2601.metamon.services.social.ReportTarget
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/** 読めなかった・入れられなかった理由(画面はこれで文言を選ぶ)。 */
enum class DiscoverFailure {
    /** サーバーにつながっていない構成(モック) */
    UNAVAILABLE,

    /** 403: 掲載者が公開をやめた */
    UNPUBLISHED,

    /** 404: 掲載が無い */
    NOT_FOUND,

    /** 圏外・時間切れ */
    OFFLINE,
    OTHER,
}

/** 通報・ブロックの結果。 */
enum class DiscoverModerationResult { REPORTED, REPORT_LIMITED, REPORT_FAILED, BLOCKED, BLOCK_FAILED }

sealed interface DiscoverInstallResult {
    /** 入れた(2 回目以降は最初に入れたアプリ)。このアプリに切り替える */
    data class Installed(val app: InstallResult) : DiscoverInstallResult

    data class Failed(val reason: DiscoverFailure) : DiscoverInstallResult
}

data class DiscoverState(
    val query: String = "",
    val sort: DiscoverSort = DiscoverSort.POPULAR,
    val items: List<PublicApp> = emptyList(),
    /** 選んだカテゴリ(タグ)。null = すべて */
    val category: String? = null,
    val loading: Boolean = true,
    /** 一覧を読めなかった(読めていた分は残す) */
    val failure: DiscoverFailure? = null,
    /** インストール中の掲載(二度押しを止める) */
    val installing: String? = null,
) {
    /** 一覧の上に並べるカテゴリ(結果のタグの多い順・同数は先に出た順・最大 [MAX_CATEGORIES])。 */
    val categories: List<String> get() = categoriesOf(items)

    /** カテゴリで絞った一覧。 */
    val visibleItems: List<PublicApp> get() = category?.let { c -> items.filter { it.hasTag(c) } } ?: items

    companion object {
        const val MAX_CATEGORIES = 8

        fun categoriesOf(items: List<PublicApp>): List<String> {
            val counts = LinkedHashMap<String, Int>()
            val labels = HashMap<String, String>()
            items.forEach { item ->
                item.tags.map(::normalizedTag).filter { it.isNotEmpty() }.distinctBy { it.lowercase() }.forEach { tag ->
                    val key = tag.lowercase()
                    labels.putIfAbsent(key, tag)
                    counts[key] = (counts[key] ?: 0) + 1
                }
            }
            return counts.entries.sortedByDescending { it.value }.take(MAX_CATEGORIES).map { labels.getValue(it.key) }
        }
    }
}

/** タグの表記ゆれ(先頭の #・前後の空白)を除く。 */
internal fun normalizedTag(tag: String): String = tag.trim().removePrefix("#").trim()

/** 大文字小文字を区別せずにタグを持つか。 */
internal fun PublicApp.hasTag(tag: String): Boolean = tags.any { normalizedTag(it).equals(normalizedTag(tag), ignoreCase = true) }

/**
 * 「探す」(iOS の `DiscoverSheet`・#902): 検索・並び順・カテゴリ・インストール・通報・掲載者のブロック。
 * 入力中の間引きは画面側(250ms 待ってから [search])。
 * @param client null = 「探す」が使えない構成
 * @param moderation null = 通報・ブロックを出さない
 */
class DiscoverModel(
    private val client: DiscoverClient?,
    private val moderation: ModerationClient? = null,
) {
    private val _state = MutableStateFlow(DiscoverState())
    val state: StateFlow<DiscoverState> = _state.asStateFlow()

    val canModerate: Boolean get() = moderation != null

    fun setQuery(query: String) = _state.update { it.copy(query = query) }

    fun setSort(sort: DiscoverSort) = _state.update { it.copy(sort = sort) }

    /** 同じカテゴリをもう一度選ぶと外す。 */
    fun toggleCategory(tag: String) = _state.update { it.copy(category = if (it.category.equals(tag, ignoreCase = true)) null else tag) }

    /** 今の検索語・並び順で読み直す。 */
    suspend fun search() {
        val client = client ?: return _state.update { it.copy(loading = false, failure = DiscoverFailure.UNAVAILABLE) }
        val asked = _state.value
        _state.update { it.copy(loading = true) }
        try {
            val page = client.search(asked.query, asked.sort)
            _state.update { now ->
                if (now.query != asked.query || now.sort != asked.sort) return@update now // 読んでいる間に条件が変わった
                // 絞っていたカテゴリが結果に無くなったら外す
                val category = now.category?.takeIf { c -> page.items.any { it.hasTag(c) } }
                now.copy(items = page.items, category = category, loading = false, failure = null)
            }
        } catch (e: CancellationException) {
            _state.update { it.copy(loading = false) }
            throw e
        } catch (e: Exception) {
            _state.update { it.copy(loading = false, failure = failureOf(e)) }
        }
    }

    /** インストール(取り込み)。切り替えは呼び出し側。 */
    suspend fun install(item: PublicApp): DiscoverInstallResult {
        val client = client ?: return DiscoverInstallResult.Failed(DiscoverFailure.UNAVAILABLE)
        if (_state.value.installing != null) return DiscoverInstallResult.Failed(DiscoverFailure.OTHER)
        _state.update { it.copy(installing = item.id) }
        return try {
            DiscoverInstallResult.Installed(client.install(item.id))
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            DiscoverInstallResult.Failed(failureOf(e))
        } finally {
            _state.update { it.copy(installing = null) }
        }
    }

    /** 掲載を通報する(自分の掲載は出さない・画面側で隠す)。 */
    suspend fun report(item: PublicApp, reason: ReportReason): DiscoverModerationResult {
        val moderation = moderation ?: return DiscoverModerationResult.REPORT_FAILED
        return try {
            moderation.report(ReportTarget.Listing(item.id), reason)
            DiscoverModerationResult.REPORTED
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            if ((e as? ApiError)?.status == 429) DiscoverModerationResult.REPORT_LIMITED else DiscoverModerationResult.REPORT_FAILED
        }
    }

    /** 掲載者をブロックする(uid はサーバーが引く)。先にその掲載を隠し、成功したら読み直す(他の掲載も消える)。 */
    suspend fun blockPublisher(item: PublicApp): DiscoverModerationResult {
        val moderation = moderation ?: return DiscoverModerationResult.BLOCK_FAILED
        val before = _state.value.items
        _state.update { s -> s.copy(items = s.items.filter { it.id != item.id }) }
        return try {
            moderation.blockListing(item.id)
            search()
            DiscoverModerationResult.BLOCKED
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            _state.update { it.copy(items = before) }
            DiscoverModerationResult.BLOCK_FAILED
        }
    }

    companion object {
        fun failureOf(e: Throwable): DiscoverFailure {
            when ((e as? ApiError)?.status) {
                403 -> return DiscoverFailure.UNPUBLISHED
                404 -> return DiscoverFailure.NOT_FOUND
            }
            return when (FailureKind.classify(e)) {
                FailureKind.Offline, FailureKind.Timeout -> DiscoverFailure.OFFLINE
                else -> DiscoverFailure.OTHER
            }
        }
    }
}
