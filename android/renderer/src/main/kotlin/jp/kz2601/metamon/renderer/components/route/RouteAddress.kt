package jp.kz2601.metamon.renderer.components.route

/** 住所 → 緯度経度を引いた結果(#1350)。 */
sealed interface AddressLookup {
    data class Found(val lat: Double, val lng: Double) : AddressLookup

    /** 引けたが当たりが無い(iOS の「「…」が見つかりませんでした」)。 */
    data object NotFound : AddressLookup

    /** 引けなかった(Geocoder が無い端末・圏外・時間切れ)。何も言わず距離を出さない。 */
    data object Unavailable : AddressLookup

    companion object {
        /** Geocoder の答え(null = 引けなかった・要素の null = 緯度経度の無い当たり)から結果を作る。 */
        fun of(results: List<Pair<Double, Double>?>?): AddressLookup {
            val list = results ?: return Unavailable
            val (lat, lng) = list.firstNotNullOfOrNull { it } ?: return NotFound
            return Found(lat, lng)
        }
    }
}

/**
 * 部品の中で住所の引き結果を覚えておく(描き直しのたびに引かない)。
 * 答えが出た結果(見つかった / 見つからない)だけ覚え、引けなかった時は次に聞かれた時にまた引く。
 */
class RouteAddressCache(private val lookup: suspend (String) -> AddressLookup) {
    private val memo = HashMap<String, AddressLookup>()

    /** もう引いてあれば、その結果。 */
    fun peek(address: String): AddressLookup? = memo[address]

    suspend fun resolve(address: String): AddressLookup {
        memo[address]?.let { return it }
        val r = lookup(address)
        if (r !is AddressLookup.Unavailable) memo[address] = r
        return r
    }
}

/** 住所の行き先(距離を出す時だけ使う。地図アプリには住所の文字列のまま渡す)。 */
fun AddressLookup?.place(address: String): RoutePlace? =
    (this as? AddressLookup.Found)?.let { RoutePlace("address", address, it.lat, it.lng) }
