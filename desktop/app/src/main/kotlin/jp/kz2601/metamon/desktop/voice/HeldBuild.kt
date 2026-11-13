package jp.kz2601.metamon.desktop.voice

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.io.File

/** 止めた時の会話の 1 行(誰が言ったか・文)。 */
data class HeldChatLine(val fromUser: Boolean, val text: String)

/**
 * 作らずに止めた依頼(#3007・iOS の `AppRuntime+HeldBuild`・Android の `HeldBuild`)。
 * 1 日の枠が切れた(429)・有料が要る(402)時に、聞き返しに答えた会話と依頼文を消さずに残す。
 * 画面は「続きから作る」のカードを出し、押したら [text] と同じ依頼で作る(デスクトップでは買う道は出さない)。
 */
data class HeldBuild(
    val text: String,
    val reason: Reason,
    val chat: List<HeldChatLine> = emptyList(),
    /** 1 日に作れる数(分かる時だけ。カードの文に使う) */
    val dailyFreeLimit: Int? = null,
) {
    enum class Reason { DAILY_LIMIT, PRO_REQUIRED }

    companion object {
        /** HTTP の状態から止める理由に(429 = 枠切れ・402 = 有料)。それ以外は止めない */
        fun reasonFor(httpStatus: Int): Reason? = when (httpStatus) {
            429 -> Reason.DAILY_LIMIT
            402 -> Reason.PRO_REQUIRED
            else -> null
        }
    }
}

/**
 * 止めた依頼の置き場。1 つだけ持つ(新しく止めたら置き換え)。[file] が null ならメモリだけ。
 * - 止めた時に [hold]: 端末に残る(再起動しても出せる)
 * - 作れた時に [onBuilt]・話し直した時に [dismiss]・サインアウトで [onSignOut]: 消える
 * 壊れたファイルは捨てて空から始める(依頼が消えるだけ)。
 */
class HeldBuildStore(private val file: File?) {
    private var memory: HeldBuild? = null
    private var loaded = false

    @Synchronized
    fun current(): HeldBuild? {
        if (!loaded) {
            loaded = true
            memory = readFile()
        }
        return memory
    }

    /** 止めた依頼を残す。依頼文が空なら何も残さない。 */
    @Synchronized
    fun hold(held: HeldBuild) {
        if (held.text.isBlank()) return
        loaded = true
        memory = held
        val f = file ?: return
        runCatching {
            f.parentFile?.mkdirs()
            val tmp = File(f.parentFile, "${f.name}.tmp")
            tmp.writeText(encode(held))
            if (!tmp.renameTo(f)) {
                f.delete()
                tmp.renameTo(f)
            }
        }
    }

    /** 作れた(同じ依頼で続きから作れた)ので消す。 */
    @Synchronized
    fun onBuilt() = clear()

    /** 話し直した・カードを閉じたので消す。 */
    @Synchronized
    fun dismiss() = clear()

    /** サインアウト・アカウント削除で消す(別の人に見せない)。 */
    @Synchronized
    fun onSignOut() = clear()

    private fun clear() {
        loaded = true
        memory = null
        file?.let { runCatching { it.delete() } }
    }

    private fun readFile(): HeldBuild? {
        val f = file ?: return null
        val text = runCatching { if (f.exists()) f.readText() else null }.getOrNull() ?: return null
        return decode(text) ?: run {
            runCatching { f.delete() }
            null
        }
    }

    internal companion object {
        fun encode(held: HeldBuild): String = JsonObject(
            buildMap {
                put("text", JsonPrimitive(held.text))
                put("reason", JsonPrimitive(held.reason.name))
                held.dailyFreeLimit?.let { put("dailyFreeLimit", JsonPrimitive(it)) }
                put(
                    "chat",
                    JsonArray(held.chat.map { JsonObject(mapOf("fromUser" to JsonPrimitive(it.fromUser), "text" to JsonPrimitive(it.text))) }),
                )
            },
        ).toString()

        fun decode(text: String): HeldBuild? = runCatching {
            val o = Json.parseToJsonElement(text).jsonObject
            val body = o["text"]?.jsonPrimitive?.contentOrNull?.takeIf { it.isNotBlank() } ?: return null
            val reason = HeldBuild.Reason.entries.firstOrNull { it.name == o["reason"]?.jsonPrimitive?.contentOrNull } ?: return null
            val chat = o["chat"]?.jsonArray.orEmpty().mapNotNull {
                val line = it.jsonObject
                val said = line["text"]?.jsonPrimitive?.contentOrNull ?: return@mapNotNull null
                HeldChatLine(line["fromUser"]?.jsonPrimitive?.booleanOrNull ?: return@mapNotNull null, said)
            }
            HeldBuild(body, reason, chat, o["dailyFreeLimit"]?.jsonPrimitive?.intOrNull)
        }.getOrNull()
    }
}
