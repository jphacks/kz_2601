package jp.kz2601.metamon

/**
 * DEBUG のつまみ(#844・iOS の環境変数 `METAMON_*` と同じ名前)。値を読むのは debug のビルドだけ
 * (`src/debug/.../DebugLaunch.kt`)。release は `src/release` の版が常に [NONE] を返すので、つまみの読み方がバイナリに入らない。
 * 渡し方は android/README.md の「モックとつまみ」。
 */
data class DebugKnobs(
    /** `METAMON_MOCK=1`: 接続先を無視して全部モック(認証・appconfig・ai・文字起こし) */
    val mock: Boolean = false,
    /** `METAMON_MOCK_AI_DELAY=<秒>`: モックの ai の応答待ち(既定 1.2 秒) */
    val mockAiDelaySeconds: Double? = null,
    /** `METAMON_MOCK_SHARING=1`: 変身一覧に 2 件入れ、共有を端末内で完結させる */
    val mockSharing: Boolean = false,
    /** `METAMON_INITIAL_EXAMPLE=<名前>`: 最初に描く DSL を同梱の docs/dsl/examples から選ぶ */
    val initialExample: String? = null,
    /** `METAMON_INITIAL_DSL_PATH=<端末のパス>`: 最初に描く DSL をファイルから読む(見た目の監査用・#131) */
    val initialDslPath: String? = null,
    /** `METAMON_INITIAL_SCREEN=<screen>`: initialScreen を上書き */
    val initialScreen: String? = null,
    /** `METAMON_AUTO_TRANSCRIPT=<文>`: 起動直後に 1 回だけ発話を投げる(ヘッドレス E2E) */
    val autoTranscript: String? = null,
    /** `METAMON_AUTO_NAV=a,b,c:sheet`: 画面を順に巡る(E2E のスクショ用) */
    val autoNav: List<AutoNavStep> = emptyList(),
    /** `METAMON_AUTO_NAV_INTERVAL=<秒>`(既定 2) */
    val autoNavIntervalSeconds: Double = 2.0,
    /** `METAMON_E2E_OFFLINE_SECONDS=<n>`: 起動から n 秒間、記録の API を圏外にする */
    val offlineSeconds: Double? = null,
    /**
     * `METAMON_ACCOUNT_LINK_PROMPTED=1|0`: 引き継ぎの案内(#1316)を出した扱いにする / 出していない扱いに戻す
     * (iOS の起動引数 `-metamon.accountLinkPrompted YES|NO`)。前のテストで作ったアプリの案内が画面を塞がないように
     */
    val accountLinkPrompted: Boolean? = null,
) {
    data class AutoNavStep(val screen: String, val presentation: String? = null) {
        override fun toString(): String = presentation?.let { "$screen:$it" } ?: screen
    }

    companion object {
        val NONE = DebugKnobs()
    }
}
