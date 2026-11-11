package jp.kz2601.metamon.services.update

import jp.kz2601.metamon.services.http.ApiJson
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull

/**
 * appconfig `GET /v1/client-config`(#302・#333・認証不要。iOS の `ClientConfig`)。省略されたフィールド = 強制しない。
 *
 * 今の backend が返すのは iOS 向けの版(`minIOSBuild` など)だけ。Android は iOS の値を使わず、
 * `minAndroidBuild` / `latestAndroidBuild` / `androidStoreUrl` が来た時だけ判定する(無ければ止めない・#906)。
 */
data class ClientConfig(
    val minAndroidBuild: Int? = null,
    val latestAndroidBuild: Int? = null,
    val androidStoreUrl: String? = null,
    /** アプリ内のお知らせ(#333)。壊れていても null にするだけで、版の判定は続ける */
    val notice: ServiceNotice? = null,
) {
    companion object {
        /** 本文が JSON のオブジェクトでなければ throw。中の値が変な時はそのフィールドだけ捨てる。 */
        fun decode(body: String): ClientConfig {
            val obj = ApiJson.parseToJsonElement(body) as? JsonObject ?: throw IllegalArgumentException("client-config is not an object")
            return ClientConfig(
                minAndroidBuild = obj.positiveInt("minAndroidBuild"),
                latestAndroidBuild = obj.positiveInt("latestAndroidBuild"),
                androidStoreUrl = obj.string("androidStoreUrl"),
                notice = obj["notice"]?.let(ServiceNotice::decode),
            )
        }
    }
}

/** 版の判定(#302)。**fail open**: 設定が取れない・自分の build が読めない時は何もしない。 */
sealed interface UpdateRequirement {
    data object None : UpdateRequirement

    /** 閉じられるお知らせ(latest 未満) */
    data class Recommended(val latest: Int) : UpdateRequirement

    /** 閉じられない「アップデートしてください」(min 未満) */
    data class Required(val minimum: Int) : UpdateRequirement

    companion object {
        fun evaluate(config: ClientConfig?, build: String): UpdateRequirement {
            val current = BuildNumber.parse(build) ?: return None
            config ?: return None
            config.minAndroidBuild?.takeIf { it > 0 }?.let { if (current < BuildNumber.of(it)) return Required(it) }
            config.latestAndroidBuild?.takeIf { it > 0 }?.let { if (current < BuildNumber.of(it)) return Recommended(it) }
            return None
        }
    }
}

/** 版の番号の比較(iOS の `BuildNumber`)。`12` / `1.2.3` の数字の並びを左から比べる(足りない桁は 0)。 */
class BuildNumber private constructor(private val parts: List<Int>) : Comparable<BuildNumber> {
    override fun compareTo(other: BuildNumber): Int {
        for (i in 0 until maxOf(parts.size, other.parts.size)) {
            val x = parts.getOrElse(i) { 0 }
            val y = other.parts.getOrElse(i) { 0 }
            if (x != y) return x.compareTo(y)
        }
        return 0
    }

    override fun equals(other: Any?): Boolean = other is BuildNumber && compareTo(other) == 0

    override fun hashCode(): Int = parts.dropLastWhile { it == 0 }.hashCode()

    override fun toString(): String = parts.joinToString(".")

    companion object {
        fun of(value: Int): BuildNumber = BuildNumber(listOf(value))

        /** 数字と `.` だけ(空の区切り・負の数・数字以外は null)。 */
        fun parse(raw: String): BuildNumber? {
            val pieces = raw.trim().split(".")
            val parts = pieces.map { p ->
                if (p.isEmpty() || !p.all { it in '0'..'9' }) return null
                p.toIntOrNull() ?: return null
            }
            return BuildNumber(parts)
        }
    }
}

internal fun JsonObject.string(key: String): String? = (this[key] as? JsonPrimitive)?.takeIf { it.isString }?.contentOrNull

internal fun JsonObject.positiveInt(key: String): Int? = (this[key] as? JsonPrimitive)?.takeIf { !it.isString }?.intOrNull?.takeIf { it > 0 }

internal fun JsonElement.asObject(): JsonObject? = this as? JsonObject
