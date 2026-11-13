package jp.kz2601.metamon.renderer.components.health

import android.content.Context
import androidx.health.connect.client.HealthConnectClient
import androidx.health.connect.client.aggregate.AggregationResultGroupedByPeriod
import androidx.health.connect.client.permission.HealthPermission
import androidx.health.connect.client.records.StepsRecord
import androidx.health.connect.client.records.WeightRecord
import androidx.health.connect.client.records.metadata.Metadata
import androidx.health.connect.client.request.AggregateGroupByPeriodRequest
import androidx.health.connect.client.request.ReadRecordsRequest
import androidx.health.connect.client.time.TimeRangeFilter
import androidx.health.connect.client.units.Mass
import java.time.Instant
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.Period
import java.time.ZoneId
import kotlin.coroutines.cancellation.CancellationException

/**
 * 実機の Health Connect(#870・iOS の `LiveHealthData`)。歩数は 1 日ごとの合計、体重は記録のある日の最新値。
 * 権限は Health Connect の画面で聞く(聞くのは [rememberHealthPermissionRequest])。ここでは許可されているかだけ見て、
 * 足りなければ [HealthFailure.DENIED] を投げる(iOS の「アクセスが許可されていません」に当たる)。
 */
class HealthConnectSource(context: Context) : HealthSource {
    private val appContext = context.applicationContext
    private val zone: ZoneId get() = ZoneId.systemDefault()

    private val client: HealthConnectClient by lazy { HealthConnectClient.getOrCreate(appContext) }

    override fun availability(): HealthAvailability = when (HealthConnectClient.getSdkStatus(appContext)) {
        HealthConnectClient.SDK_AVAILABLE -> HealthAvailability.AVAILABLE
        HealthConnectClient.SDK_UNAVAILABLE_PROVIDER_UPDATE_REQUIRED -> HealthAvailability.UPDATE_REQUIRED
        else -> HealthAvailability.UNAVAILABLE
    }

    override fun readPermissions(weight: Boolean): Set<String> =
        setOf(if (weight) HealthPermission.getReadPermission(WeightRecord::class) else HealthPermission.getReadPermission(StepsRecord::class))

    override fun writePermissions(): Set<String> = setOf(HealthPermission.getWritePermission(WeightRecord::class))

    override suspend fun granted(): Set<String> = call { client.permissionController.getGrantedPermissions() }

    override suspend fun steps(days: Int): List<HealthDailyValue> {
        requirePermissions(readPermissions(weight = false))
        val starts = HealthStatRules.dayStarts(days, LocalDate.now(zone))
        val from = starts.first().atStartOfDay()
        val to = LocalDateTime.now(zone)
        val groups = call {
            client.aggregateGroupByPeriod(
                AggregateGroupByPeriodRequest(
                    metrics = setOf(StepsRecord.COUNT_TOTAL),
                    timeRangeFilter = TimeRangeFilter.between(from, to),
                    timeRangeSlicer = Period.ofDays(1),
                ),
            )
        }
        val byDay = groups.associate { g: AggregationResultGroupedByPeriod ->
            g.startTime.toLocalDate() to (g.result[StepsRecord.COUNT_TOTAL]?.toDouble() ?: 0.0)
        }
        return starts.map { HealthDailyValue(it, byDay[it] ?: 0.0) }
    }

    override suspend fun weights(days: Int): List<HealthDailyValue> {
        requirePermissions(readPermissions(weight = true))
        val starts = HealthStatRules.dayStarts(days, LocalDate.now(zone))
        val from = starts.first().atStartOfDay(zone).toInstant()
        val records = call {
            client.readRecords(
                ReadRecordsRequest(
                    recordType = WeightRecord::class,
                    timeRangeFilter = TimeRangeFilter.after(from),
                    ascendingOrder = false,
                    pageSize = MAX_WEIGHT_RECORDS,
                ),
            ).records
        }
        // 新しい順なので、その日に初めて見た値がその日の最新
        val latestPerDay = linkedMapOf<LocalDate, Double>()
        for (r in records) {
            val day = r.time.atZone(r.zoneOffset ?: zone.rules.getOffset(r.time)).toLocalDate()
            latestPerDay.putIfAbsent(day, r.weight.inKilograms)
        }
        return latestPerDay.entries.sortedBy { it.key }.map { HealthDailyValue(it.key, it.value) }
    }

    override suspend fun saveWeight(kg: Double) {
        requirePermissions(writePermissions())
        val now = Instant.now()
        call {
            client.insertRecords(
                listOf(
                    WeightRecord(
                        time = now,
                        zoneOffset = zone.rules.getOffset(now),
                        weight = Mass.kilograms(kg),
                        metadata = Metadata.manualEntry(),
                    ),
                ),
            )
        }
    }

    /** 端末の事情(入っていない・更新が要る)を先に、次に権限を見る。 */
    private suspend fun requirePermissions(permissions: Set<String>) {
        when (availability()) {
            HealthAvailability.UNAVAILABLE -> throw HealthDataException(HealthFailure.UNAVAILABLE)
            HealthAvailability.UPDATE_REQUIRED -> throw HealthDataException(HealthFailure.UPDATE_REQUIRED)
            HealthAvailability.AVAILABLE -> Unit
        }
        if (!granted().containsAll(permissions)) throw HealthDataException(HealthFailure.DENIED)
    }

    /** Health Connect の例外は部品に出す 1 文に落とす(OS の文は出さない)。 */
    private suspend fun <T> call(body: suspend () -> T): T = try {
        body()
    } catch (e: CancellationException) {
        throw e
    } catch (e: HealthDataException) {
        throw e
    } catch (e: IllegalStateException) {
        throw HealthDataException(HealthFailure.UNAVAILABLE, e)
    } catch (e: SecurityException) {
        throw HealthDataException(HealthFailure.DENIED, e)
    } catch (e: Exception) {
        throw HealthDataException(HealthFailure.FAILED, e)
    }

    private companion object {
        /** 体重は 30 日分でも足りる件数(iOS の limit 200 と同じ)。 */
        const val MAX_WEIGHT_RECORDS = 200
    }
}
