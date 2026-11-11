package jp.kz2601.metamon

import android.content.Intent

/**
 * 起動の intent extra からつまみを読む(debug だけ・#844)。名前は iOS の環境変数と同じ。
 * `adb shell am start -n <pkg>/jp.kz2601.metamon.MainActivity --es METAMON_MOCK 1`、
 * instrumentation のテストは引数を同じ名前の extra にして ActivityScenario に渡す。
 */
object DebugLaunch {
    fun knobs(intent: Intent?): DebugKnobs {
        val extras = intent?.extras ?: return DebugKnobs.NONE
        val raw = extras.keySet().filter { it.startsWith("METAMON_") }
            .associateWith { key -> @Suppress("DEPRECATION") extras.get(key)?.toString().orEmpty() }
        return DebugKnobParser.parse(raw)
    }
}

/** 文字列のつまみ → [DebugKnobs]。壊れた値は無視する(既定のまま)。 */
object DebugKnobParser {
    fun parse(env: Map<String, String>): DebugKnobs {
        fun str(key: String) = env[key]?.trim()?.takeIf { it.isNotEmpty() }
        fun flag(key: String) = str(key) == "1"
        fun seconds(key: String) = str(key)?.toDoubleOrNull()?.takeIf { it >= 0 && it.isFinite() }
        return DebugKnobs(
            mock = flag("METAMON_MOCK"),
            mockAiDelaySeconds = seconds("METAMON_MOCK_AI_DELAY"),
            mockSharing = flag("METAMON_MOCK_SHARING"),
            initialExample = str("METAMON_INITIAL_EXAMPLE"),
            initialDslPath = str("METAMON_INITIAL_DSL_PATH"),
            initialScreen = str("METAMON_INITIAL_SCREEN"),
            autoTranscript = str("METAMON_AUTO_TRANSCRIPT"),
            autoNav = parseAutoNav(str("METAMON_AUTO_NAV")),
            autoNavIntervalSeconds = seconds("METAMON_AUTO_NAV_INTERVAL") ?: 2.0,
            offlineSeconds = seconds("METAMON_E2E_OFFLINE_SECONDS")?.takeIf { it > 0 },
            autoFeedbackSeconds = seconds("METAMON_AUTO_FEEDBACK"),
            iapEnforce = flag("METAMON_IAP_ENFORCE"),
            iapFreeLimit = str("METAMON_IAP_FREE_LIMIT")?.toIntOrNull()?.takeIf { it >= 0 },
            accountLinkPrompted = when (str("METAMON_ACCOUNT_LINK_PROMPTED")) {
                "1" -> true
                "0" -> false
                else -> null
            },
        )
    }

    /** `b,c,a,c:sheet` → 画面と出し方(iOS `AppRuntime+AutoNav` と同じ書き方)。 */
    fun parseAutoNav(spec: String?): List<DebugKnobs.AutoNavStep> =
        spec.orEmpty().split(",").map(String::trim).filter(String::isNotEmpty).map {
            val parts = it.split(":")
            DebugKnobs.AutoNavStep(parts[0], parts.getOrNull(1)?.takeIf(String::isNotEmpty))
        }
}
