package jp.kz2601.metamon.desktop.usage

import jp.kz2601.metamon.services.AppUsage
import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.UsageMeter
import jp.kz2601.metamon.services.consent.KeyValueStore
import jp.kz2601.metamon.services.http.PlanLimit
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import java.time.Instant
import java.time.ZoneId

/** 上限に近い時の 1 行のお知らせ(iOS の `UsageNotice`・#317)。描画はしない。 */
data class UsageNotice(
    val resource: PlanLimit.Resource,
    /** 記録・写真はアプリごと */
    val appId: String? = null,
    val meter: UsageMeter,
    /** Pro にすると上がる(作成者だけ) */
    val upgrade: Boolean,
) {
    /** 1 日 1 回の記録に使う(リソース + アプリ) */
    val id: String get() = appId?.let { "${resource.wire}:$it" } ?: resource.wire
}

/** 出す / 出さない(iOS の `UsageNoticeRules`)。 */
object UsageNoticeRules {
    /** 80% 以上で知らせる */
    const val THRESHOLD = UsageMeter.NEAR_THRESHOLD

    /** 上限なし・上限 0・読めない時は false */
    fun isNear(meter: UsageMeter?): Boolean = meter?.fraction?.let { it >= THRESHOLD } ?: false

    /** アプリの数(アプリ一覧に出す) */
    fun appsNotice(me: MyUsage?): UsageNotice? {
        val apps = me?.apps?.takeIf { isNear(it) } ?: return null
        return UsageNotice(PlanLimit.Resource.APPS, meter = apps, upgrade = !me.isPro)
    }

    /** 記録・写真(そのアプリの中に出す)。参加者(他の人のアプリ)には出さない */
    fun appNotices(usage: AppUsage?): List<UsageNotice> {
        if (usage == null || !usage.isMine) return emptyList()
        return buildList {
            usage.records?.takeIf { isNear(it) }?.let { add(UsageNotice(PlanLimit.Resource.RECORDS_PER_APP, usage.appId, it, usage.upgrade)) }
            usage.photos?.takeIf { isNear(it) }?.let { add(UsageNotice(PlanLimit.Resource.PHOTOS_PER_APP, usage.appId, it, usage.upgrade)) }
        }
    }
}

/**
 * お知らせは 1 つのアカウント・リソースにつき 1 日 1 回(iOS の `UsageNoticeLog`)。
 * 出した時点で数える(閉じても同じ日はもう出さない)。日付は JST(上限の戻る時刻と同じ)。uid ごとに別に数える。
 * 鍵は iOS と同じ。中身は `"<uid>|<id>" → "yyyy-M-d"` の JSON の文字列。
 */
class UsageNoticeLog(private val store: KeyValueStore) {
    fun shouldShow(notice: UsageNotice, uid: String?, now: Instant = Instant.now()): Boolean =
        shown()[entry(notice, uid)] != day(now)

    fun markShown(notice: UsageNotice, uid: String?, now: Instant = Instant.now()) {
        val today = day(now)
        val map = shown().filterValues { it == today }.toMutableMap() // 前の日の分は捨てる(増え続けない)
        map[entry(notice, uid)] = today
        store.putString(KEY, JsonObject(map.mapValues { JsonPrimitive(it.value) }).toString())
    }

    fun clear() = store.remove(KEY)

    private fun shown(): Map<String, String> {
        val raw = store.getString(KEY) ?: return emptyMap()
        val obj = runCatching { Json.parseToJsonElement(raw) as? JsonObject }.getOrNull() ?: return emptyMap()
        return obj.mapNotNull { (k, v) -> (v as? JsonPrimitive)?.contentOrNull?.let { k to it } }.toMap()
    }

    companion object {
        const val KEY = "metamon.usageNotice.shownDay"
        private val JST: ZoneId = ZoneId.of("Asia/Tokyo")

        internal fun day(at: Instant): String = at.atZone(JST).toLocalDate().let { "${it.year}-${it.monthValue}-${it.dayOfMonth}" }

        internal fun entry(notice: UsageNotice, uid: String?): String = "${uid ?: "-"}|${notice.id}"
    }
}
