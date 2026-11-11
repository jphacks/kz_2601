package jp.kz2601.metamon.renderer.components.map

/**
 * 地図のタイルの配信元(#867)。差し替える時はここだけ変える。
 *
 * OpenFreeMap(https://openfreemap.org): API キー・登録が要らず、表示回数の上限なし・商用可(利用規約 https://openfreemap.org/tos/)。
 * 守ること: 帰属表示(OpenFreeMap © OpenMapTiles・データは © OpenStreetMap contributors)を地図の上に出す。
 * 一括の取り込み(オフライン用の大量ダウンロード)はしない。tile.openstreetmap.org はアプリからの大量の利用を禁じているので使わない。
 */
object MapStyle {
    const val LIGHT = "https://tiles.openfreemap.org/styles/liberty"
    const val DARK = "https://tiles.openfreemap.org/styles/dark"

    /** 地図の上に必ず出す帰属表示。 */
    const val ATTRIBUTION = "OpenFreeMap © OpenMapTiles © OpenStreetMap contributors"

    /** 帰属表示を押した時に開く(OSM の著作権とライセンス)。 */
    const val COPYRIGHT_URL = "https://www.openstreetmap.org/copyright"

    /** ピンの名前の字(スタイルが配る字の集まり)。日本語などは MapLibre が端末の字で描く。 */
    val FONT = arrayOf("Noto Sans Regular")

    fun url(dark: Boolean): String = if (dark) DARK else LIGHT
}
