package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.expr.Expr
import jp.kz2601.metamon.dsl.expr.ExprEvaluator
import jp.kz2601.metamon.dsl.expr.ExprValue
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** iOS の SingleEntityTests・RecordRetentionTests・RecordPermissionTests を写したもの。 */
class RecordRulesTest {
    // MARK: - single(#267)

    private fun settings(extra: String = "") =
        Entity.parse("""{"id":"settings","name":"設定","fields":[{"id":"cycle_days","name":"周期","type":"integer","defaultValue":28}]$extra}""")

    @Test
    fun decodesSingleLeniently() {
        assertTrue(settings(""","single":true""").single)
        assertFalse(settings().single)
        assertFalse(settings(""","single":false""").single)
        assertFalse("壊れた値は entity ごと落とさず false", settings(""","single":"yes"""").single)
    }

    @Test
    fun settingExprRoundTrips() {
        val e = Expr.parse("""{"setting":"cycle_days"}""")
        assertEquals(Expr.Setting("cycle_days"), e)
        assertEquals("""{"setting":"cycle_days"}""", e.toJsonElement().toString())
        assertEquals(emptySet<String>(), e.referencedEntities)
        val nested = Expr.parse("""{"op":"addDays","args":[{"op":"today","args":[]},{"setting":"cycle_days"}]}""") as Expr.Op
        assertEquals("addDays", nested.name)
        assertEquals(Expr.Setting("cycle_days"), nested.args[1])
    }

    @Test
    fun pickNewestRecord() {
        assertNull(SingleRecord.pick(emptyList()))
        val a = Record("a", createdAt = "2026-10-01T00:00:00Z")
        val b = Record("b", createdAt = "2026-10-05T00:00:00Z")
        assertEquals("b", SingleRecord.pick(listOf(b, a))?.id)
        assertEquals("b", SingleRecord.pick(listOf(a, b))?.id)
        assertEquals("同じ(どちらも無い)なら後の方", "y", SingleRecord.pick(listOf(Record("x"), Record("y")))?.id)
    }

    @Test
    fun settingReadsDefaultThenSaved() {
        val s = settings(""","single":true""")
        var rows = emptyList<Record>()
        fun make() = ExprEvaluator(listOf(s), { if (it == "settings") rows else emptyList() })
        assertEquals(ExprValue.Number(28.0), make().evaluate(Expr.Setting("cycle_days")))
        rows = listOf(Record("s", mapOf("cycle_days" to JSONValue.number(30))))
        assertEquals(ExprValue.Number(30.0), make().evaluate(Expr.Setting("cycle_days")))
        assertNull(make().evaluate(Expr.Setting("nope")))
        assertNull(make().evaluate(Expr.Setting("\$createdAt")))
        // single でない entity の field は読まない
        assertNull(ExprEvaluator(listOf(settings()), { rows }).evaluate(Expr.Setting("cycle_days")))
    }

    // MARK: - 保存期間(#482)

    private fun diary(extra: String = "") = Entity.parse("""{"id":"diary","name":"日記","fields":[{"id":"body","name":"本文","type":"text"}]$extra}""")

    private val now: Instant = Instant.parse("2026-10-31T12:00:00Z")

    private fun record(id: String, daysAgo: Double?, author: String? = null) =
        Record(id, createdAt = daysAgo?.let { FieldCoding.formatDatetime(now.minusMillis((it * 86_400_000).toLong())) }, author = author)

    @Test
    fun decodesRetentionDaysLeniently() {
        assertEquals(7, diary(""","retentionDays":7""").retentionDays)
        assertEquals(30, diary(""","retentionDays":30.0""").retentionDays)
        assertNull(diary().retentionDays)
        assertNull(diary(""","retentionDays":"7"""").retentionDays)
        assertNull(diary(""","retentionDays":0""").retentionDays)
        assertNull(diary(""","retentionDays":-3""").retentionDays)
        assertNull(diary(""","retentionDays":1.5""").retentionDays)
    }

    @Test
    fun expiredUsesCreatedAt() {
        val e = diary(""","retentionDays":7""")
        val rows = listOf(
            record("new", 1.0), record("edge", 7.0), record("old", 7.01), record("older", 40.0), record("nodate", null),
            Record("bad", createdAt = "yesterday"),
        )
        assertEquals(listOf("old", "older"), RecordRetention.expired(rows, e, 7, now, "me").map { it.id })
        // 小数秒つきの RFC3339(appconfig の形)
        val frac = Record("frac", createdAt = "2026-10-01T00:00:00.123Z")
        assertEquals(listOf("frac"), RecordRetention.expired(listOf(frac), e, 7, now, null).map { it.id })
    }

    @Test
    fun sharedEntityDeletesOnlyOwnRecords() {
        val e = diary(""","scope":"public","retentionDays":7""")
        val rows = listOf(record("mine", 10.0, "me"), record("theirs", 10.0, "other"), record("legacy", 10.0))
        assertEquals(listOf("mine"), RecordRetention.expired(rows, e, 7, now, "me").map { it.id })
        assertTrue("uid 不明は消さない", RecordRetention.expired(rows, e, 7, now, null).isEmpty())
        assertEquals("private は全部自分のもの", 3, RecordRetention.expired(rows, diary(""","retentionDays":7"""), 7, now, null).size)
    }

    @Test
    fun consentGatesShorterPeriods() {
        val e = diary(""","retentionDays":7""")
        assertNull(RecordRetention.activeDays(e, null))
        assertEquals(7, RecordRetention.activeDays(e, 7))
        assertEquals(7, RecordRetention.activeDays(e, 3))
        assertNull(RecordRetention.activeDays(e, 30))
        assertNull(RecordRetention.activeDays(diary(), 7))

        val entities = listOf(
            Entity.parse("""{"id":"diary","name":"日記","retentionDays":7,"fields":[]}"""),
            Entity.parse("""{"id":"tags","name":"タグ","fields":[]}"""),
            Entity.parse("""{"id":"logs","name":"ログ","retentionDays":30,"fields":[]}"""),
        )
        assertEquals(listOf("diary", "logs"), RecordRetention.pending(entities, emptyMap()).map { it.id })
        assertEquals(listOf("logs"), RecordRetention.pending(entities, mapOf("diary" to 7, "logs" to 60)).map { it.id })
        assertTrue(RecordRetention.pending(entities, mapOf("diary" to 7, "logs" to 30)).isEmpty())
    }

    @Test
    fun periodLabel() {
        assertEquals("1週間", RecordRetention.periodLabel(7))
        assertEquals("2週間", RecordRetention.periodLabel(14))
        assertEquals("30日", RecordRetention.periodLabel(30))
        assertEquals("1年", RecordRetention.periodLabel(365))
        assertEquals("1日", RecordRetention.periodLabel(1))
        assertEquals("1 week", RecordRetention.periodLabel(7, english = true))
        assertEquals("90 days", RecordRetention.periodLabel(90, english = true))
    }

    // MARK: - 権限(#249 / #252 / #272)

    private fun chore(scope: String?, edit: String? = null): Entity {
        var json = """{"id":"chore","name":"家事","fields":[{"id":"title","name":"家事","type":"string"}]"""
        if (scope != null) json += ""","scope":"$scope""""
        if (edit != null) json += ""","edit":"$edit""""
        return Entity.parse("$json}")
    }

    private val mine = Record("r1", author = "me")
    private val theirs = Record("r2", author = "other")
    private val unsent = Record("local-x")

    @Test
    fun decodesScopeEditAndAuthor() {
        val e = chore("public", "members")
        assertTrue(e.isShared)
        assertTrue(e.membersCanEdit)
        assertEquals("u1", Record.parse("""{"id":"r","data":{},"author":"u1","authorName":"さくら"}""").author)
        assertFalse("private では無視", chore(null, "members").membersCanEdit)
        assertFalse("未知の scope は private 扱い", chore("team").isShared)
    }

    @Test
    fun authorOnlySharedEntity() {
        val e = chore("public")
        assertTrue(RecordPermission.canEdit(mine, e, "me"))
        assertTrue(RecordPermission.canDelete(mine, e, "me"))
        assertFalse(RecordPermission.canEdit(theirs, e, "me"))
        assertFalse(RecordPermission.canDelete(theirs, e, "me"))
        assertTrue("未送信の自分の行は author がまだ無い", RecordPermission.canEdit(unsent, e, "me"))
    }

    @Test
    fun membersEntityUpdatesButDeletesOnlyOwn() {
        val e = chore("shared", "members")
        assertTrue(RecordPermission.canEdit(theirs, e, "me"))
        assertFalse(RecordPermission.canDelete(theirs, e, "me"))
        assertTrue(RecordPermission.canDelete(mine, e, "me"))
    }

    @Test
    fun privateOrUnknownUserIsNotBlocked() {
        assertTrue(RecordPermission.canEdit(theirs, chore(null), "me"))
        assertTrue(RecordPermission.canDelete(theirs, chore("public"), null))
        assertTrue(RecordPermission.canEdit(theirs, null, "me"))
    }

    @Test
    fun deleteBy() {
        fun shared(deleteBy: String) = Entity.parse(
            """{"id":"s","name":"シフト","scope":"shared","deleteBy":"$deleteBy","unique":["title"],"fields":[{"id":"title","name":"t","type":"string","editableBy":"admins"}]}""",
        )
        val admins = shared("admins")
        assertEquals(listOf("title"), admins.unique)
        assertTrue(admins.fields[0].isAdminOnly)
        assertFalse(RecordPermission.canDelete(mine, admins, "me", isAdmin = false))
        assertTrue(RecordPermission.canDelete(theirs, admins, "me", isAdmin = true))
        assertTrue("分からない時は作った人", RecordPermission.canDelete(mine, admins, "me"))
        assertFalse(RecordPermission.canDelete(theirs, admins, "me"))
        val either = shared("authorOrAdmins")
        assertTrue(RecordPermission.canDelete(mine, either, "me", isAdmin = false))
        assertTrue(RecordPermission.canDelete(theirs, either, "me", isAdmin = true))
        assertFalse(RecordPermission.canDelete(theirs, either, "me", isAdmin = null))
    }
}
