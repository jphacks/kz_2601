package jp.kz2601.metamon.voice

/**
 * 版の履歴の 1 行(appconfig の `GET /versions`)。restore で作られた版は [kind] が `restore` で、
 * 中身の元の版が [restoredFrom] に入る。
 */
data class VersionInfo(
    val version: Int,
    val parentVersion: Int? = null,
    val kind: String? = null,
    val restoredFrom: Int? = null,
)

/**
 * 「元に戻す」で戻る先を版の履歴から選ぶ(iOS の `UndoWalk`)。
 * restore はサーバーで新しい版を作るので「今の版 − 1」だと直前の 2 つを往復する。
 * restore で作られた版は中身の元([VersionInfo.restoredFrom])に読み替え、その親へ戻る(v5 → v4 → v3 …)。
 */
object UndoWalk {
    /** 版の中身の元。restore の版は [VersionInfo.restoredFrom] を一覧にある限りたどる。 */
    fun origin(version: Int, byVersion: Map<Int, VersionInfo>): Int {
        var v = version
        val seen = mutableSetOf<Int>()
        while (seen.add(v)) {
            val info = byVersion[v] ?: break
            if (info.kind != "restore") break
            val from = info.restoredFrom ?: break
            if (from <= 0) break
            v = from
        }
        return v
    }

    /** 戻す先の版。無ければ null。[versions] は `GET /versions` の結果(並び順は問わない)。 */
    fun undoTarget(current: Int, versions: List<VersionInfo>): Int? {
        val byVersion = byVersion(versions)
        val from = origin(current, byVersion)
        val info = byVersion[from]
        val parent = when {
            info?.parentVersion != null && info.parentVersion > 0 -> info.parentVersion
            info != null && from == 1 -> null
            // 親を返さない古い版: 一覧にある 1 つ前の版
            else -> versions.map { it.version }.filter { it < from }.maxOrNull()
        } ?: return null
        val target = origin(parent, byVersion)
        return if (target == from) null else target
    }

    /** 同じ版が二度載っていたら先に出てきた方を採る(iOS の `uniquingKeysWith`)。 */
    fun byVersion(versions: List<VersionInfo>): Map<Int, VersionInfo> {
        val map = LinkedHashMap<Int, VersionInfo>(versions.size)
        for (v in versions) map.putIfAbsent(v.version, v)
        return map
    }
}
