package jp.kz2601.metamon.services.local

import kotlinx.serialization.Serializable
import java.io.File

/**
 * この端末のデータの持ち主の uid(#841)。起動をまたいで覚え、別の uid で入られたら前の人のデータを消す判断に使う
 * (外からサインアウトされた後、アプリが落ちている間に別の人が入った時も分かる)。
 *
 * 置き場は `filesDir/metamon-account/owner.json`。
 *
 * @param directory 置き場。null ならメモリだけ(テスト用)
 */
class AccountOwnerStore(private val directory: File?) {
    @Serializable
    private data class Saved(val uid: String)

    private val lock = Any()
    private var loaded = false
    private var memory: String? = null

    /** 覚えている持ち主。まだ誰も入っていない・後片付けの後は null。 */
    var uid: String?
        get() = synchronized(lock) { read() }
        set(value) = synchronized(lock) { write(value) }

    private fun read(): String? {
        if (loaded) return memory
        val file = file()
        memory = runCatching {
            if (file != null && file.exists()) RecordStore.json.decodeFromString(Saved.serializer(), file.readText()).uid else null
        }.getOrNull()?.takeIf { it.isNotEmpty() }
        loaded = true
        return memory
    }

    private fun write(value: String?) {
        memory = value
        loaded = true
        val file = file() ?: return
        runCatching {
            if (value == null) {
                file.delete()
            } else {
                AtomicFile.write(file, RecordStore.json.encodeToString(Saved.serializer(), Saved(value)))
            }
        }.onFailure { println("[Metamon] AccountOwnerStore write failed: $it") }
    }

    private fun file(): File? = directory?.resolve("owner.json")
}
