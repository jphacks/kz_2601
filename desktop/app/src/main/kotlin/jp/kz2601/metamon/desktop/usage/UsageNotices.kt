package jp.kz2601.metamon.desktop.usage

import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import jp.kz2601.metamon.desktop.store.DesktopStore
import jp.kz2601.metamon.services.AppUsage
import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.PlanUsageSource
import jp.kz2601.metamon.services.UsageMeter
import jp.kz2601.metamon.services.consent.KeyValueStore
import kotlinx.coroutines.CancellationException
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.time.Instant
import java.time.ZoneId

/** お知らせの対象(iOS の `PlanLimit.Resource` のうちお知らせに出す物)。[wire] は 1 日 1 回の記録の鍵。 */
enum class UsageResource(val wire: String) {
    APPS("apps"),
    RECORDS_PER_APP("recordsPerApp"),
    PHOTOS_PER_APP("photosPerApp"),
}

/** 上限に近い時の 1 行のお知らせ(iOS の `UsageNotice`)。Pro の案内はデスクトップでは出さない(課金が未定)。 */
data class UsageNotice(val resource: UsageResource, val meter: UsageMeter, val appId: String? = null) {
    /** 1 日 1 回の記録に使う(リソース + アプリ) */
    val id: String get() = appId?.let { "${resource.wire}:$it" } ?: resource.wire

    val isAtLimit: Boolean get() = meter.limit?.let { meter.used >= it } ?: false
}

/** 出す / 出さないの規則(iOS の `UsageNoticeRules`)。80% 以上で出す。 */
object UsageNoticeRules {
    fun isNear(meter: UsageMeter?): Boolean = meter?.isNear == true

    /** 作れるアプリの数(アプリの一覧の上に出す) */
    fun appsNotice(me: MyUsage?): UsageNotice? {
        val apps = me?.apps?.takeIf(::isNear) ?: return null
        return UsageNotice(UsageResource.APPS, apps)
    }

    /** 記録・写真(そのアプリのウィンドウの上に出す)。参加者(他の人のアプリ)には出さない */
    fun appNotices(usage: AppUsage?): List<UsageNotice> {
        if (usage == null || !usage.isMine) return emptyList()
        return buildList {
            usage.records?.takeIf(::isNear)?.let { add(UsageNotice(UsageResource.RECORDS_PER_APP, it, usage.appId)) }
            usage.photos?.takeIf(::isNear)?.let { add(UsageNotice(UsageResource.PHOTOS_PER_APP, it, usage.appId)) }
        }
    }
}

/**
 * お知らせは 1 つのアカウント・リソースにつき 1 日 1 回(iOS の `UsageNoticeLog`)。出した時点で数え、閉じても同じ日はもう出さない。
 * 日付は JST(上限の戻る時刻と同じ)。uid を鍵に含める。前の日の分は書く時に捨てる。
 */
class UsageNoticeLog(private val store: KeyValueStore) {
    fun shouldShow(notice: UsageNotice, uid: String?, now: Instant): Boolean = shown()[entry(notice, uid)] != day(now)

    fun markShown(notice: UsageNotice, uid: String?, now: Instant) {
        val today = day(now)
        val map = shown().filterValues { it == today } + (entry(notice, uid) to today)
        store.putString(KEY, JsonObject(map.mapValues { JsonPrimitive(it.value) }).toString())
    }

    private fun shown(): Map<String, String> = runCatching {
        Json.parseToJsonElement(store.getString(KEY) ?: return emptyMap()).jsonObject
            .mapNotNull { (k, v) -> v.jsonPrimitive.contentOrNull?.let { k to it } }.toMap()
    }.getOrDefault(emptyMap())

    companion object {
        const val KEY = "tailorit.usageNotice.shownDay"
        private val JST: ZoneId = ZoneId.of("Asia/Tokyo")

        fun day(now: Instant): String = now.atZone(JST).toLocalDate().toString()

        fun entry(notice: UsageNotice, uid: String?): String = "${uid ?: "-"}|${notice.id}"
    }
}

/** 1 行の文(iOS の `UsageCopy.notice`)。「作れるアプリの数が上限に近づいています(8 / 10)」 */
object UsageNoticeText {
    fun text(notice: UsageNotice): String {
        val what = Strings[
            when (notice.resource) {
                UsageResource.APPS -> "desktop_usage_resource_apps"
                UsageResource.RECORDS_PER_APP -> "desktop_usage_resource_records"
                UsageResource.PHOTOS_PER_APP -> "desktop_usage_resource_photos"
            },
        ]
        val amount = "${notice.meter.used} / ${notice.meter.limit}"
        return Strings.format(if (notice.isAtLimit) "desktop_usage_notice_at_limit" else "desktop_usage_notice_near", what, amount)
    }
}

/**
 * 使っている量を読み、今出すお知らせを 1 つ選ぶ。選んだ物は出した印を付ける(同じ日はもう出さない)。
 * 読めない・古いサーバー(null)・通信の失敗は出さない。[source] が null(モック)なら何も出さない。
 */
class UsageNotices(
    private val source: PlanUsageSource?,
    private val log: UsageNoticeLog,
    private val uid: () -> String? = { null },
    private val now: () -> Instant = Instant::now,
) {
    /** アプリのウィンドウの上に出す 1 件(記録・写真)。 */
    suspend fun nextForApp(appId: String): UsageNotice? =
        pick(UsageNoticeRules.appNotices(read { it.appUsage(appId) }))

    /** アプリの一覧の上に出す「作れるアプリの数」。 */
    suspend fun nextForApps(): UsageNotice? =
        pick(listOfNotNull(UsageNoticeRules.appsNotice(read { it.myUsage() })))

    fun pick(candidates: List<UsageNotice>): UsageNotice? {
        val at = now()
        val who = uid()
        return candidates.firstOrNull { log.shouldShow(it, who, at) }?.also { log.markShown(it, who, at) }
    }

    companion object {
        /** 本物の API の時だけ作る(モックの間は null = 出さない)。サインインが入るまで uid は無い。 */
        fun create(store: DesktopStore, config: DesktopApiConfig = DesktopApiConfig.fromEnvironment()): UsageNotices? {
            val source = DesktopApi.appConfig(config, store.documents) as? PlanUsageSource ?: return null
            return UsageNotices(source, UsageNoticeLog(store.settings))
        }
    }

    private suspend fun <T> read(block: suspend (PlanUsageSource) -> T?): T? {
        val s = source ?: return null
        return try {
            block(s)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        }
    }
}
