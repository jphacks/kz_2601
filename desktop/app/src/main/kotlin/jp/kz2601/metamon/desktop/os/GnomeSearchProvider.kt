package jp.kz2601.metamon.desktop.os

import jp.kz2601.metamon.desktop.CommandRanking
import jp.kz2601.metamon.desktop.PaletteCommand

/**
 * GNOME Shell の検索(D-Bus の `org.gnome.Shell.SearchProvider2`)に、作ったアプリの名前だけを返す(#2993)。
 * iOS の Spotlight に当たる。記録の中身は返さない。既定は off([enabled] が false の間は何も返さない)。
 *
 * D-Bus への登録(名前を取る・メソッドを公開する)はここに含めない。公開する側は下の 4 つのメソッドを呼ぶだけにして、
 * 規則はここでテストできるようにしてある。一致の規則はコマンドパレットと同じ [CommandRanking.score]。
 */
class GnomeSearchProvider(
    /** (id, アプリ名)。名前順でなくてよい。 */
    private val apps: () -> List<Pair<String, String>>,
    /** そのアプリを開く(画面のスレッドへ移すのは呼ぶ側)。 */
    private val open: (appId: String) -> Unit,
    /** 設定の「GNOME の検索に出す」。既定 false。 */
    private val enabled: () -> Boolean = { false },
) {
    /** `GetInitialResultSet(as terms) -> as`。結果の id はアプリの id。 */
    fun getInitialResultSet(terms: List<String>): List<String> = search(apps(), terms)

    /** `GetSubsearchResultSet(as previous_results, as terms) -> as`。前の結果の中だけで絞る。 */
    fun getSubsearchResultSet(previous: List<String>, terms: List<String>): List<String> {
        val prev = previous.toSet()
        return search(apps().filter { it.first in prev }, terms)
    }

    /** `GetResultMetas(as identifiers) -> aa{sv}`。名前だけ(説明・記録の中身は入れない)。知らない id は飛ばす。 */
    fun getResultMetas(ids: List<String>): List<ResultMeta> {
        if (!enabled()) return emptyList()
        val byId = apps().toMap()
        return ids.mapNotNull { id -> byId[id]?.let { ResultMeta(id, it) } }
    }

    /** `ActivateResult(s identifier, as terms, u timestamp)`。選ばれたアプリを開く。 */
    fun activateResult(id: String) {
        if (!enabled()) return
        if (apps().any { it.first == id }) open(id)
    }

    /** `LaunchSearch(as terms, u timestamp)`。「すべての結果を見る」は一覧が無いので、呼ぶ側がアプリを前に出すだけ。 */
    fun launchSearch(onLaunch: () -> Unit) {
        if (enabled()) onLaunch()
    }

    private fun search(candidates: List<Pair<String, String>>, terms: List<String>): List<String> {
        if (!enabled()) return emptyList()
        // GNOME は空白で区切った語を渡す。コマンドパレットは空白を無視して 1 つの問いにするので、同じ形に揃える
        val query = terms.joinToString("").trim()
        if (query.isEmpty()) return emptyList()
        val commands = candidates.map { (id, name) -> PaletteCommand(id, name, PaletteCommand.Kind.APP, run = {}) }
        return CommandRanking.rank(commands, query).map { it.id }
    }

    data class ResultMeta(val id: String, val name: String)

    companion object {
        const val BUS_NAME = "jp.kz2601.tailorit.SearchProvider"
        const val OBJECT_PATH = "/jp/kz2601/tailorit/SearchProvider"
        const val INTERFACE = "org.gnome.Shell.SearchProvider2"
    }
}
