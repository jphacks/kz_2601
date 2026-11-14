package jp.kz2601.metamon.desktop.update

import jp.kz2601.metamon.desktop.net.ClientPlatform
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.intOrNull

/** 版の判定の結果(Android の `UpdateRequirement` と同じ 3 つ)。 */
sealed interface UpdateDecision {
    /** そのまま */
    data object None : UpdateDecision

    /** 閉じられる案内(最新より古い) */
    data class Recommended(val latest: Int) : UpdateDecision

    /** 先へ進めない画面(最低より古い) */
    data class Required(val minimum: Int) : UpdateDecision
}

/**
 * `GET /v1/client-config` の `min{Windows,Linux}Build` / `latest{Windows,Linux}Build` から判定する(#2157 の 1)。
 * 規則は Android の `UpdateRequirement.evaluate` と同じ。**fail open**: 値が無い・形が変・知らない OS は [UpdateDecision.None]。
 */
object UpdatePolicy {
    private val json = Json { ignoreUnknownKeys = true }

    fun decide(config: JsonObject?, os: ClientPlatform, currentBuild: String): UpdateDecision {
        config ?: return UpdateDecision.None
        val key = when (os) {
            ClientPlatform.WINDOWS -> "Windows"
            ClientPlatform.LINUX -> "Linux"
            ClientPlatform.MACOS -> return UpdateDecision.None
        }
        val current = parseBuild(currentBuild) ?: return UpdateDecision.None
        config.positiveInt("min${key}Build")?.let { if (compare(current, listOf(it)) < 0) return UpdateDecision.Required(it) }
        config.positiveInt("latest${key}Build")?.let { if (compare(current, listOf(it)) < 0) return UpdateDecision.Recommended(it) }
        return UpdateDecision.None
    }

    /** 応答の本文から。JSON のオブジェクトでなければ [UpdateDecision.None]。 */
    fun decide(body: String?, os: ClientPlatform, currentBuild: String): UpdateDecision {
        val obj = body?.let { runCatching { json.parseToJsonElement(it) }.getOrNull() } as? JsonObject
        return decide(obj, os, currentBuild)
    }

    /** `12` / `1.2.3`。数字と `.` 以外・空の区切りは null(Android の `BuildNumber.parse` と同じ)。 */
    internal fun parseBuild(raw: String): List<Int>? {
        val parts = raw.trim().split(".")
        return parts.map { p ->
            if (p.isEmpty() || !p.all { it in '0'..'9' }) return null
            p.toIntOrNull() ?: return null
        }
    }

    /** 左から比べ、足りない桁は 0。 */
    internal fun compare(a: List<Int>, b: List<Int>): Int {
        for (i in 0 until maxOf(a.size, b.size)) {
            val x = a.getOrElse(i) { 0 }
            val y = b.getOrElse(i) { 0 }
            if (x != y) return x.compareTo(y)
        }
        return 0
    }

    private fun JsonObject.positiveInt(key: String): Int? =
        (this[key] as? JsonPrimitive)?.takeIf { !it.isString }?.intOrNull?.takeIf { it > 0 }
}
