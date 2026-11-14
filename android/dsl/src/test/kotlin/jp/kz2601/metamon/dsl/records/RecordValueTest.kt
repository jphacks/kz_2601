package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.records.Fixtures.rec
import jp.kz2601.metamon.dsl.records.Fixtures.str
import kotlinx.serialization.json.JsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

/** iOS の RecordValueTests・RecordMarkTests・FollowFilterTests(記録の値・印・フォローの filter)を写したもの。 */
class RecordValueTest {
    private val posts = listOf(
        rec("1", "title" to "傘", createdAt = "2026-10-26T09:00:00Z", author = "me", authorName = "さくら"),
        rec("2", "title" to "鍵", createdAt = "2026-10-26T10:00:00Z", author = "other", authorName = "匿名"),
        rec("3", "title" to "財布", createdAt = "2026-10-26T11:00:00Z"), // 作者の無い自分の未送信
    )

    @Test
    fun schemaRecordValuesMatch() {
        val entity = Fixtures.schemaDefs()["Entity"] as JsonObject
        assertEquals("schema の x-recordValues と RecordValue がずれている", RecordValue.all, (entity["x-recordValues"] as JsonObject).keys)
    }

    @Test
    fun boardExampleDecodesRecordValuesAndMeFilter() {
        val dsl = Fixtures.load("board")
        assertTrue(dsl.entity("items")!!.isShared)
        val stat = dsl.components("mine").first { it.str("type") == "stat" }
        val filters = Filter.oneOrMany(stat["filter"])
        assertEquals("{\"me\":true} が Expr として捨てられない", 1, filters.size)
        assertTrue(filters[0].isMe)
    }

    @Test
    fun recordSubscriptAndVirtualFields() {
        assertEquals(JSONValue.Str("さくら"), posts[0][RecordValue.AUTHOR])
        assertEquals(JSONValue.Str("2026-10-26T09:00:00Z"), posts[0][RecordValue.CREATED_AT])
        assertEquals(JSONValue.Null, posts[2][RecordValue.AUTHOR])

        val shared = Fixtures.load("board").entity("items")!!
        assertEquals(FieldType.STRING, shared.field(RecordValue.AUTHOR)?.type)
        assertEquals(FieldType.DATETIME, shared.field(RecordValue.CREATED_AT)?.type)
        assertNull("保存フィールドには入らない", shared.storedFields.firstOrNull { it.id.startsWith("$") })
        val priv = Fixtures.load("kakeibo").entities.first()
        assertNull("private の entity に投稿者は無い", priv.field(RecordValue.AUTHOR))
        assertNotNull(priv.field(RecordValue.CREATED_AT))
    }

    @Test
    fun meFilterComparesAuthorUid() {
        val me = Filter(RecordValue.AUTHOR, "eq", Filter.meValue)
        assertTrue(me.isMe)
        // uid に置き換える前の me はどれにも一致しない
        assertTrue(Aggregator.apply(me, null, posts).isEmpty())
        assertEquals(listOf("1"), Aggregator.apply(me.resolvingMe("me"), null, posts).map { it.id })
        val others = Aggregator.apply(Filter(RecordValue.AUTHOR, "neq", Filter.meValue).resolvingMe("me"), null, posts)
        assertEquals(listOf("2", "3"), others.map { it.id })
        // サインインしていない時は作者の無い行(自分の未送信)だけ
        assertEquals(listOf("3"), Aggregator.apply(me.resolvingMe(null), null, posts).map { it.id })
    }

    @Test
    fun sortByCreatedAt() {
        assertEquals(listOf("3", "2", "1"), Aggregator.sorted(posts, Sort(RecordValue.CREATED_AT, "desc"), null).map { it.id })
        assertEquals("sort 無しは新しい順", listOf("3", "2", "1"), Aggregator.sorted(posts, null, null).map { it.id })
    }

    @Test
    fun meFilterRoundTrips() {
        val f = Filter.parse("""{"field":"${'$'}author","op":"eq","value":{"me":true}}""")
        assertTrue(f.isMe)
        assertNull(f.valueExpr)
        assertTrue(Filter.from(f.toJsonElement()).isMe)
        // me: false は「自分」ではない(Expr でもないので filter ごと読めない)
        assertThrows(DslDecodeException::class.java) { Filter.parse("""{"field":"${'$'}author","op":"eq","value":{"me":false}}""") }
    }

    // MARK: - 印(#255)

    private val recipes = listOf(
        Record("1", mapOf("title" to JSONValue.Str("肉じゃが")), marks = mapOf("save" to MarkState(3, false))),
        Record("2", mapOf("title" to JSONValue.Str("カレー")), marks = mapOf("save" to MarkState(1, true))),
        Record("3", mapOf("title" to JSONValue.Str("卵焼き"))), // サーバーが marks を返していない
    )

    @Test
    fun recipesExampleDecodesMarks() {
        val dsl = Fixtures.load("recipes")
        assertEquals(listOf(Mark("save", "bookmark", "保存")), dsl.entity("recipes")!!.marks)
        val saved = dsl.components("saved").first { it.str("type") == "list" }
        val filters = Filter.oneOrMany(saved["filter"])
        assertEquals(1, filters.size)
        assertTrue("{\"me\":true} が Expr として捨てられない", filters[0].isMe)
        assertEquals("\$marks.save", filters[0].field)
    }

    @Test
    fun recordDecodesMarksAndSubscript() {
        val r = Record.parse("""{"id":"r1","data":{"title":"肉じゃが"},"marks":{"save":{"count":2,"mine":true}}}""")
        assertEquals(MarkState(2, true), r.markState("save"))
        assertEquals(JSONValue.number(2), r["\$marks.save"])
        assertEquals("無ければ 0 件", JSONValue.number(0), recipes[2]["\$marks.save"])
    }

    @Test
    fun markFieldAndSort() {
        val entity = Fixtures.load("recipes").entity("recipes")!!
        assertEquals(FieldType.INTEGER, entity.field("\$marks.save")?.type)
        assertEquals("保存", entity.field("\$marks.save")?.name)
        assertNull("宣言していない印", entity.field("\$marks.like"))
        assertEquals(listOf("1", "2", "3"), Aggregator.sorted(recipes, Sort("\$marks.save", "desc"), entity).map { it.id })
    }

    @Test
    fun markedByMeFilter() {
        val me = Filter("\$marks.save", "eq", Filter.meValue).resolvingMe("u1")
        assertEquals(listOf("2"), recipes.filter { Aggregator.matches(me, it, null) }.map { it.id })
        val notYet = Filter("\$marks.save", "neq", Filter.meValue).resolvingMe(null)
        assertEquals(listOf("1", "3"), recipes.filter { Aggregator.matches(notYet, it, null) }.map { it.id })
    }

    @Test
    fun toggledNeverBelowZero() {
        assertEquals(MarkState(0, false), MarkState(0, true).toggled())
        assertEquals(MarkState(3, true), MarkState(2, false).toggled())
    }

    // MARK: - フォロー(#270)

    private val timeline = listOf(
        rec("1", "body" to "a", author = "alice"),
        rec("2", "body" to "b", author = "bob"),
        rec("3", "body" to "c", author = "me"),
        rec("4", "body" to "d"), // 作者の無い自分の未送信
    )

    @Test
    fun timelineExampleDecodesMyFollows() {
        val list = Fixtures.load("timeline").components("following").first { it.str("type") == "list" }
        val filters = Filter.oneOrMany(list["filter"])
        assertEquals("{\"myFollows\":true} が Expr として捨てられない", 1, filters.size)
        assertTrue(filters[0].isMyFollows)
    }

    @Test
    fun resolvedFilterKeepsFollowedAuthors() {
        val f = Filter(RecordValue.AUTHOR, "eq", Filter.myFollowsValue)
        fun ids(f: Filter) = timeline.filter { Aggregator.matches(f, it, null) }.map { it.id }
        assertEquals("読み込む前はどの行にも一致しない", emptyList<String>(), ids(f))
        assertEquals(emptyList<String>(), ids(Filter(RecordValue.AUTHOR, "neq", Filter.myFollowsValue)))

        val resolved = f.resolvingFollows(setOf("alice"))
        assertEquals(setOf("alice"), resolved.followedAuthors)
        assertEquals(listOf("1"), ids(resolved))
        assertEquals("フォローしていない人 = 自分も含む", listOf("2", "3", "4"), ids(resolved.copy(op = "neq")))
        assertEquals("誰もフォローしていない", emptyList<String>(), ids(f.resolvingFollows(emptySet())))
    }

    @Test
    fun onlyMyFollowsIsResolved() {
        val me = Filter(RecordValue.AUTHOR, "eq", Filter.meValue)
        assertEquals(me, me.resolvingFollows(setOf("alice")))
        assertNull(me.followedAuthors)
        assertNull(Filter("tags", "eq", JSONValue.Arr(listOf(JSONValue.Str("x")))).followedAuthors)
    }

    @Test
    fun hiddenFieldsRedact() {
        val e = Entity.parse("""{"id":"q","name":"Q","fields":[{"id":"q","name":"問","type":"string"},{"id":"a","name":"答","type":"string","hidden":true}]}""")
        val r = rec("1", "q" to "1+1", "a" to "2", author = "u1")
        assertEquals(setOf("q"), HiddenFields.redact(listOf(r), e)[0].data.keys)
        assertTrue(HiddenFields.canEdit(null, null))
        assertTrue(HiddenFields.canEdit(r, "u1"))
        assertEquals(false, HiddenFields.canEdit(r, null))
        assertEquals("q", e.displayTitleField)
    }
}
