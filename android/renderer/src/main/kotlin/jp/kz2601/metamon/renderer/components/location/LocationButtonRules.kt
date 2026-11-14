package jp.kz2601.metamon.renderer.components.location

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.JSONValue
import java.util.Locale

/** 現在地を取れなかった理由(文言の出し分け)。 */
enum class LocationProblem { DENIED, OFF, FAILED }

/** `locationButton` の規則(iOS の `LocationButtonLeaf` / `OneShotLocation`)。端末に触らないので JVM で試せる。 */
object LocationButtonRules {
    /** 前に取れた位置を使ってよい古さ(1 回取りが時間切れの時の代わり)。 */
    const val MAX_LAST_KNOWN_AGE_MS = 2 * 60 * 1000L

    /** 1 つの提供元を待つ長さ。 */
    const val PROVIDER_TIMEOUT_MS = 10_000L

    const val FUSED = "fused"
    const val NETWORK = "network"
    const val GPS = "gps"

    /** 保存する形: "lat,lng"(小数 6 桁・端末の言語に依らず小数点は `.`)。 */
    fun coordinate(lat: Double, lng: Double): String = String.format(Locale.US, "%.6f,%.6f", lat, lng)

    /** 入っている値の表示(文字列で空でない時だけ)。 */
    fun preview(value: JSONValue?): String? = value?.stringValue?.takeIf { it.isNotEmpty() }

    /**
     * 聞く提供元の順。iOS は 100m 程度の精度(`kCLLocationAccuracyHundredMeters`)なので、速い順に fused → network → gps。
     * gps は細かい位置の許可がある時だけ(おおよその許可だけなら fused / network で取る)。
     */
    fun providers(enabled: Collection<String>, fineGranted: Boolean): List<String> =
        listOfNotNull(FUSED, NETWORK, GPS.takeIf { fineGranted }).filter { it in enabled }

    /** 住所も入れる field(`addressField`)。空・位置の field と同じなら入れない。 */
    fun addressTarget(c: Component): String? = c.addressField?.takeIf { it.isNotEmpty() && it != c.field }

    /** 前に取れた位置が使えるか。 */
    fun freshEnough(ageMs: Long): Boolean = ageMs in 0..MAX_LAST_KNOWN_AGE_MS
}
