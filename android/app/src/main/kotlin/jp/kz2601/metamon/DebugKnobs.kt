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
    /** `METAMON_MOCK_NEW_ITEMS=<n>`: 新着(#268)に他の人の記録 n 件 + 自分の 1 件を出す(iOS `DebugMockNewItems`)。新着の画面ができたらつなぐ */
    val mockNewItems: Int? = null,
    /** `METAMON_MOCK_USAGE=apps:9/10,records:4200/5000,photos:10/500[,pro][,participant]`: 使っている量をサーバー無しで出す(iOS `DebugMockUsage`) */
    val mockUsage: String? = null,
    /** `METAMON_MOCK_MEMBERS=owner|member`: 共有アプリの参加者(#903)を端末内で出す。自分の立場(iOS `DebugMockMembers`) */
    val mockMembers: String? = null,
    /** `METAMON_MOCK_MEMBERS_FAIL=1`: 参加者を外す・抜ける・管理者にするを失敗させる(楽観的 UI の巻き戻しを見る) */
    val mockMembersFail: Boolean = false,
    /** `METAMON_AI_CONSENT=granted|reset`: AI への送信の同意を済ませた / 初回(未同意・説明未表示)の状態で起動する(iOS `AIConsentDebug`) */
    val aiConsent: AiConsentOverride? = null,
    /** `METAMON_SEED_DUMMY=<n>`: モックの時、最初に描く DSL の各 entity に欄の型からダミーの記録を n 件(1〜10)作る(iOS `DummyRecords`) */
    val seedDummy: Int? = null,
    /** `METAMON_SEED_STEP_HOURS=<h>`: ダミーの記録の作成日時の間隔(既定 1 時間。記録の保存期間 #482 の確認は 24) */
    val seedStepHours: Double = 1.0,
    /** `METAMON_SEED_AUTHORS=mixed`: ダミーの記録の投稿者を「自分 / 別の人」交互にする(`Field.hidden` #262 の確認用) */
    val seedAuthorsMixed: Boolean = false,
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
    /** `METAMON_AUTO_FEEDBACK=<秒>`: その秒数後に、端末を振った時と同じくフィードバックの画面を開く(ヘッドレス E2E・#1314) */
    val autoFeedbackSeconds: Double? = null,
    /** `METAMON_IAP_ENFORCE=1`: debug でも新規作成の無料枠を効かせる(#1261・上限の画面の確認用) */
    val iapEnforce: Boolean = false,
    /** `METAMON_IAP_FREE_LIMIT=<n>`: 1 日の無料作成の回数を上書き(確認用) */
    val iapFreeLimit: Int? = null,
    /**
     * `METAMON_ACCOUNT_LINK_PROMPTED=1|0`: 引き継ぎの案内(#1316)を出した扱いにする / 出していない扱いに戻す
     * (iOS の起動引数 `-metamon.accountLinkPrompted YES|NO`)。前のテストで作ったアプリの案内が画面を塞がないように
     */
    val accountLinkPrompted: Boolean? = null,
) {
    data class AutoNavStep(val screen: String, val presentation: String? = null) {
        override fun toString(): String = presentation?.let { "$screen:$it" } ?: screen
    }

    enum class AiConsentOverride { GRANTED, RESET }

    companion object {
        val NONE = DebugKnobs()
    }
}
