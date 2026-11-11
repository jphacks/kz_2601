package jp.kz2601.metamon.services.purchase

import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import org.junit.Assert.assertEquals
import org.junit.Test
import java.time.LocalDate

/** iOS の CreationQuotaTests と同じ判定(#1261)。 */
class CreationQuotaTest {
    private val quota = CreationQuota(dailyFreeLimit = 3)

    @Test
    fun proAndDisabledAreUnlimited() {
        assertEquals(CreationQuota.Decision.Unlimited, quota.decide(used = 99, isPro = true, limitDisabled = false))
        assertEquals(CreationQuota.Decision.Unlimited, quota.decide(used = 99, isPro = false, limitDisabled = true))
    }

    @Test
    fun countsDownThenBlocks() {
        assertEquals(CreationQuota.Decision.Allowed(3), quota.decide(used = 0, isPro = false, limitDisabled = false))
        assertEquals(CreationQuota.Decision.Allowed(1), quota.decide(used = 2, isPro = false, limitDisabled = false))
        assertEquals(CreationQuota.Decision.Blocked, quota.decide(used = 3, isPro = false, limitDisabled = false))
        assertEquals(CreationQuota.Decision.Allowed(3), quota.decide(used = -1, isPro = false, limitDisabled = false))
        assertEquals(CreationQuota.Decision.Blocked, CreationQuota(-1).decide(used = 0, isPro = false, limitDisabled = false))
    }

    @Test
    fun ledgerResetsOnANewDay() {
        val day1 = LocalDate.of(2026, 11, 11)
        val ledger = CreationLedger().recording(day1).recording(day1)
        assertEquals(2, ledger.used(day1))
        assertEquals(0, ledger.used(day1.plusDays(1)))
        assertEquals(CreationLedger("2026-11-12", 1), ledger.recording(day1.plusDays(1)))
    }

    @Test
    fun storeKeepsTheLedger() {
        var today = LocalDate.of(2026, 11, 11)
        val kv = InMemoryKeyValueStore()
        val store = CreationUsageStore(kv, today = { today })
        assertEquals(0, store.used())
        store.record()
        store.record()
        assertEquals(2, CreationUsageStore(kv, today = { today }).used())
        assertEquals("2026-11-11|2", kv.getString(CreationUsageStore.KEY))
        today = today.plusDays(1)
        assertEquals(0, store.used())
        store.reset()
        assertEquals(CreationLedger(), store.ledger)
    }

    @Test
    fun brokenValueIsEmpty() {
        assertEquals(CreationLedger(), CreationLedger.decode("x"))
        assertEquals(CreationLedger(), CreationLedger.decode(null))
    }
}
