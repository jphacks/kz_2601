package jp.kz2601.metamon.services.purchase

import jp.kz2601.metamon.services.consent.KeyValueStore
import java.time.LocalDate

/**
 * 新規作成(初期画面からの生成)の無料枠の判定(#1261・iOS の `CreationQuota`)。純粋関数のみ。
 * 改変(作った後の「ここを変えて」)は数えない。Pro かどうかはサーバーの答え(`ProStore`)、回数は端末で数える。
 */
data class CreationQuota(val dailyFreeLimit: Int = DAILY_FREE_CREATIONS) {
    sealed interface Decision {
        /** Pro または制限の無効化(デモ端末) */
        data object Unlimited : Decision

        /** 無料枠内。[remaining] は今回の分を含めた残り回数 */
        data class Allowed(val remaining: Int) : Decision

        /** 今日の無料枠を使い切った */
        data object Blocked : Decision
    }

    fun decide(used: Int, isPro: Boolean, limitDisabled: Boolean): Decision {
        if (isPro || limitDisabled) return Decision.Unlimited
        val remaining = dailyFreeLimit.coerceAtLeast(0) - used.coerceAtLeast(0)
        return if (remaining > 0) Decision.Allowed(remaining) else Decision.Blocked
    }

    companion object {
        /** 1 日の無料作成の回数(iOS の `ProProducts.dailyFreeCreations`) */
        const val DAILY_FREE_CREATIONS = 3
    }
}

/** 今日の新規作成の回数。日付が変わったら 0 から数え直す(端末のローカル日付・iOS の `CreationLedger`)。 */
data class CreationLedger(val day: String = "", val count: Int = 0) {
    fun used(on: LocalDate): Int = if (day == dayKey(on)) count else 0

    fun recording(on: LocalDate): CreationLedger {
        val key = dayKey(on)
        return CreationLedger(key, (if (day == key) count else 0) + 1)
    }

    /** 保存する形(`yyyy-MM-dd|count`) */
    fun encode(): String = "$day|$count"

    companion object {
        /** `yyyy-MM-dd` */
        fun dayKey(date: LocalDate): String = date.toString()

        fun decode(raw: String?): CreationLedger {
            val parts = raw?.split("|") ?: return CreationLedger()
            val count = parts.getOrNull(1)?.toIntOrNull() ?: return CreationLedger()
            return CreationLedger(parts[0], count)
        }
    }
}

/** [CreationLedger] の置き場(iOS の `CreationUsageStore`・鍵の名前も同じ)。 */
class CreationUsageStore(
    private val store: KeyValueStore,
    private val today: () -> LocalDate = LocalDate::now,
    private val key: String = KEY,
) {
    val ledger: CreationLedger get() = CreationLedger.decode(store.getString(key))

    fun used(): Int = ledger.used(today())

    fun record() = store.putString(key, ledger.recording(today()).encode())

    fun reset() = store.remove(key)

    companion object {
        const val KEY = "metamon.pro.creationLedger"
    }
}

/** 有料の機能を外して作る時の指示(iOS の `PaidFeatureCheck.fallbackFreeSuffix` と同じ文・AI に送るだけで人には出さない)。 */
object PaidFeatureCheck {
    const val FALLBACK_FREE_SUFFIX = "(無料版で作る: 他のユーザーとデータを共有する機能などの有料機能は入れない)"
}
