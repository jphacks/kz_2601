package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Presentation
import kotlinx.coroutines.delay

/** 自動で移る 1 手。[presentation] は `sheet` / `fullScreen`(無ければ push)。 */
data class AutoNavStep(val screen: String, val presentation: String?) {
    override fun toString(): String = if (presentation == null) screen else "$screen:$presentation"
}

/**
 * ヘッドレスで画面を順に移る(iOS の `AppRuntime+AutoNav`・#113)。画面の写しの自動化に使う。
 *
 * 環境変数:
 * - `METAMON_AUTO_NAV="b,c,a,c:sheet,other"` 今見えている画面から順に navigate を実行する。`:sheet` / `:fullScreen` で presentation を指定
 * - `METAMON_AUTO_NAV_INTERVAL` 間隔(秒・既定 2)。開始は起動の [START_DELAY_SECONDS] 秒後
 *
 * 開発用: 指定が無ければ何もしない。接続先が [allowed] でない時も効かせない。
 */
data class AutoNavPlan(val steps: List<AutoNavStep>, val intervalSeconds: Double) {
    companion object {
        const val SPEC_ENV = "METAMON_AUTO_NAV"
        const val INTERVAL_ENV = "METAMON_AUTO_NAV_INTERVAL"
        const val DEFAULT_INTERVAL_SECONDS = 2.0
        const val START_DELAY_SECONDS = 3.0

        /**
         * 書き方を読む。`,` で手を分け、`:` の後ろが presentation。空の手・画面の名前が空の手は飛ばす。
         * 手が 1 つも無ければ null。間隔は読めない・0 未満なら既定。
         */
        fun parse(spec: String?, interval: String? = null): AutoNavPlan? {
            val steps = spec.orEmpty().split(',').mapNotNull { raw ->
                val parts = raw.split(':')
                val screen = parts.first().trim()
                if (screen.isEmpty()) return@mapNotNull null
                AutoNavStep(screen, parts.getOrNull(1)?.trim()?.takeIf { it.isNotEmpty() })
            }
            if (steps.isEmpty()) return null
            val seconds = interval?.trim()?.toDoubleOrNull()?.takeIf { it.isFinite() && it >= 0 } ?: DEFAULT_INTERVAL_SECONDS
            return AutoNavPlan(steps, seconds)
        }

        /**
         * stg・開発用の接続先の時だけ。接続先を足した時は、ここで効かせるかを決める(`when` に else を置かない)。
         */
        fun allowed(api: DesktopApiConfig): Boolean = when (api.mode) {
            DesktopApiConfig.Mode.MOCK, DesktopApiConfig.Mode.STG -> true
        }

        fun fromEnvironment(
            env: Map<String, String> = System.getenv(),
            api: DesktopApiConfig = DesktopApiConfig.fromEnvironment(env),
        ): AutoNavPlan? = if (allowed(api)) parse(env[SPEC_ENV], env[INTERVAL_ENV]) else null
    }
}

/** [AutoNavStep] を、押した時と同じ navigate の操作にする(知らない presentation は push)。 */
internal fun AutoNavStep.toAction(): Action = Action(
    type = ActionType.NAVIGATE,
    screen = screen,
    presentation = presentation?.let { w -> Presentation.entries.firstOrNull { it.wire == w && it != Presentation.UNKNOWN } },
)

/** 始める前に [AutoNavPlan.START_DELAY_SECONDS] 秒待ち、[plan] の手を [AutoNavPlan.intervalSeconds] 秒おきに [session] へ流す。 */
suspend fun runAutoNav(
    plan: AutoNavPlan,
    session: DesktopSession,
    startDelayMillis: Long = (AutoNavPlan.START_DELAY_SECONDS * 1000).toLong(),
    log: (String) -> Unit = { println("[Metamon] $it") },
) {
    delay(startDelayMillis)
    for ((i, step) in plan.steps.withIndex()) {
        val route = session.top
        session.dispatch(step.toAction(), ActionContext(route, session.form(route)))
        val nav = session.nav
        // スクショ側が同期できるよう 1 行に状態を出す(iOS と同じ形)
        log("auto nav ${i + 1}/${plan.steps.size}: → $step top=${session.top.screen} sheet=${nav.sheet?.screen ?: "-"} fullScreen=${nav.fullScreen?.screen ?: "-"}")
        delay((plan.intervalSeconds * 1000).toLong())
    }
}
