package jp.kz2601.metamon.desktop.store

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.io.File

/**
 * アプリの最後に見た画面の写し(#3203・iOS の `AppThumbnailStore`)。`<AppDataDir>/thumbnails/<appId>.png` にアプリごとに 1 枚。
 * - 消したアプリの物は [prune] で消す。[maxCount] を超えたら古い物(最後に置いた時刻が前の物)から消す
 * - サインアウト・アカウント削除で [removeAll](DesktopStore.wipe)
 * 画面を撮るのは別(ここは置き場と片付けだけ)。
 *
 * @param dir null ならメモリだけ(テスト用)
 * @param clock 置いた時刻(古い順を決める)。ファイルの更新時刻に書く
 */
class ThumbnailStore(
    val dir: File?,
    val maxCount: Int = DEFAULT_MAX_COUNT,
    private val clock: () -> Long = System::currentTimeMillis,
) {
    private class Entry(val bytes: ByteArray, val at: Long)

    /** dir が null の時の置き場 */
    private val memory = mutableMapOf<String, Entry>()

    private val _revision = MutableStateFlow(0)

    /** 置く・消すたびに進む。行はこれを読んで描き直す */
    val revision: StateFlow<Int> = _revision.asStateFlow()

    init {
        require(maxCount > 0) { "maxCount must be positive" }
    }

    /** [appId] の PNG を置く(前の物は置き換える)。数を超えたら古い物を消す。 */
    @Synchronized
    fun put(appId: String, png: ByteArray) {
        if (appId.isBlank()) return
        val now = clock()
        if (dir == null) {
            memory[appId] = Entry(png, now)
        } else {
            val target = file(appId)
            runCatching {
                writeAtomically(target, png)
                target.setLastModified(now)
            }.onFailure { System.err.println("[TailorIt] サムネイルの保存に失敗: $it") }
        }
        trimToMax()
        bump()
    }

    /** [appId] の PNG。無い・読めない時は null。 */
    @Synchronized
    fun get(appId: String): ByteArray? {
        if (appId.isBlank()) return null
        if (dir == null) return memory[appId]?.bytes
        return runCatching { file(appId).takeIf { it.isFile }?.readBytes() }.getOrNull()
    }

    fun contains(appId: String): Boolean = get(appId) != null

    @Synchronized
    fun remove(appId: String) {
        val removed = if (dir == null) memory.remove(appId) != null else file(appId).delete()
        if (removed) bump()
    }

    /**
     * 片付け: [liveAppIds](今あるアプリ)に無い物・[maxCount] を超えた古い物を消す。消した数。
     * 書きかけの一時ファイル(`*.tmp`)も消す。
     */
    @Synchronized
    fun prune(liveAppIds: Set<String>): Int {
        val liveNames = liveAppIds.map(::fileName).toSet()
        val gone = if (dir == null) {
            memory.keys.filter { it !in liveAppIds }.onEach { memory.remove(it) }.size
        } else {
            sweepTempFiles(dir)
            pngFiles().filter { it.name !in liveNames }.count { it.delete() }
        }
        val removed = gone + trimToMax()
        if (removed > 0) bump()
        return removed
    }

    /** サインアウト・アカウント削除: 全部消す。 */
    @Synchronized
    fun removeAll() {
        memory.clear()
        dir?.deleteRecursively()
        bump()
    }

    /** 置いてあるアプリの数(テスト・診断用)。 */
    @Synchronized
    fun count(): Int = if (dir == null) memory.size else pngFiles().size

    /** [appId] の置き場。ファイル名に使えない字は `_` にする(iOS と同じ)。 */
    fun file(appId: String): File = File(requireNotNull(dir), fileName(appId))

    private fun trimToMax(): Int {
        if (dir == null) {
            val extra = memory.size - maxCount
            if (extra <= 0) return 0
            memory.entries.sortedBy { it.value.at }.take(extra).map { it.key }.forEach { memory.remove(it) }
            return extra
        }
        val files = pngFiles()
        val extra = files.size - maxCount
        if (extra <= 0) return 0
        return files.sortedWith(compareBy<File> { it.lastModified() }.thenBy { it.name }).take(extra).count { it.delete() }
    }

    private fun pngFiles(): List<File> =
        dir?.listFiles().orEmpty().filter { it.isFile && it.name.endsWith(EXTENSION) }

    private fun bump() {
        _revision.value += 1
    }

    companion object {
        const val DIR_NAME = "thumbnails"
        const val EXTENSION = ".png"

        /** 一覧に出るアプリの数より十分多く、写しで保存先を食わない程度(1 枚およそ 20〜60 KB) */
        const val DEFAULT_MAX_COUNT = 50

        fun fileName(appId: String): String =
            appId.map { if (it.isLetterOrDigit() || it == '-' || it == '_') it else '_' }.joinToString("") + EXTENSION
    }
}
