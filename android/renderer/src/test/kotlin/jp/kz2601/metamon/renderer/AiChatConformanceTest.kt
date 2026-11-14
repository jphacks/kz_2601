package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.RecordPermission
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.conformance.RenderTree
import jp.kz2601.metamon.renderer.navigation.rootRouteFor
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * 描画の木の aiChat(#1170)。DSL の値と会話の並び(役・本文)が木に入るか。
 * iOS の RenderTree はまだ aiChat の値を出さず(coverage.json の notYetCovered)、共有の expected.json が無いので Android だけで確かめる。
 */
class AiChatConformanceTest {
    private fun dsl(chat: String, scope: String = "private") = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"home",
         "entities":[
          {"id":"turn","name":"会話","scope":"$scope","fields":[{"id":"text","name":"本文","type":"text"},{"id":"role","name":"話した人","type":"enum","options":["user","assistant"]}]},
          {"id":"meal","name":"食事","fields":[{"id":"food","name":"料理","type":"string"}]},
          {"id":"diary","name":"日記","fields":[{"id":"body","name":"本文","type":"text"}]}],
         "screens":[{"id":"home","title":"相談","components":[$chat]}]}
        """.trimIndent(),
    )

    private class CaseRuntime(dsl: AppDSL, records: Map<String, List<Record>>, private val uid: String?) : DSLRuntime {
        override val entityModels: List<Entity> = dsl.entityModels()
        override val recordsFlow: StateFlow<Map<String, List<Record>>> = MutableStateFlow(records)
        override fun onVoiceInput(componentId: String?) = Unit
        override fun canEdit(entity: String, record: Record) = RecordPermission.canEdit(record, entityModel(entity), uid)
        override fun canDelete(entity: String, record: Record) = RecordPermission.canDelete(record, entityModel(entity), uid)
        override fun currentUid(): String? = uid
    }

    private fun records(json: String): List<Record> = DslJson.parseToJsonElement(json).jsonArray.map { Record.from(it) }

    private fun node(dsl: AppDSL, records: List<Record>, uid: String? = "me"): JsonObject {
        val tree = RenderTree(dsl, ComponentRegistry.standard.registeredTypes, CaseRuntime(dsl, mapOf("turn" to records), uid))
        return tree.screen(dsl.rootRouteFor("home"))["nodes"]!!.jsonArray.single().jsonObject
    }

    private fun json(s: String) = DslJson.parseToJsonElement(s)

    @Test
    fun roleFieldGivesRoleFromRecord() {
        val dsl = dsl(
            """{"type":"aiChat","id":"talk","area":"A1:F12","entity":"turn","textField":"text","roleField":"role",
               "saveTo":{"entity":"meal","fields":["food"]},"inputEntity":"diary","persona":"聞き役","placeholder":"いま気になっていること"}""",
        )
        // 作成時刻の順に並べる(配列の順ではない)。本文が空の記録は飛ばす
        val recs = records(
            """[
              {"id":"t3","data":{"text":"どうしましたか","role":"assistant"},"createdAt":"2026-11-12T10:02:00+09:00"},
              {"id":"t1","data":{"text":"眠れない","role":"user"},"createdAt":"2026-11-12T10:00:00+09:00"},
              {"id":"t2","data":{"text":"  ","role":"assistant"},"createdAt":"2026-11-12T10:01:00+09:00"},
              {"id":"t4","data":{"text":"仕事のこと","role":"user"},"createdAt":"2026-11-12T10:03:00+09:00"}
            ]""",
        )
        val expected = json(
            """{"id":"talk","type":"aiChat","cell":{"row":0,"col":0,"rowSpan":12,"colSpan":6},"state":"rendered",
               "props":{"placeholder":"いま気になっていること","entity":"turn","textField":"text","roleField":"role",
                 "saveTo":{"entity":"meal"},"inputEntity":"diary",
                 "turns":[{"recordId":"t1","role":"user","text":"眠れない"},
                          {"recordId":"t3","role":"assistant","text":"どうしましたか"},
                          {"recordId":"t4","role":"user","text":"仕事のこと"}]}}""",
        )
        assertEquals(expected, node(dsl, recs))
    }

    @Test
    fun legacyDslWithoutRoleFieldAlternates() {
        val dsl = dsl("""{"type":"aiChat","id":"talk","area":"A1:F12","entity":"turn","textField":"text"}""")
        // roleField が無い古い DSL: 古い順に 自分 → AI の交互。本文の無い記録は交互の数えに入れない
        val recs = records(
            """[
              {"id":"b","data":{"text":"こんにちは"},"createdAt":"2026-11-12T10:01:00+09:00"},
              {"id":"a","data":{"text":"やあ"},"createdAt":"2026-11-12T10:00:00+09:00"},
              {"id":"x","data":{},"createdAt":"2026-11-12T10:02:00+09:00"},
              {"id":"c","data":{"text":"元気?"},"createdAt":"2026-11-12T10:03:00+09:00"}
            ]""",
        )
        val expected = json(
            """{"id":"talk","type":"aiChat","cell":{"row":0,"col":0,"rowSpan":12,"colSpan":6},"state":"rendered",
               "props":{"entity":"turn","textField":"text",
                 "turns":[{"recordId":"a","role":"user","text":"やあ"},
                          {"recordId":"b","role":"assistant","text":"こんにちは"},
                          {"recordId":"c","role":"user","text":"元気?"}]}}""",
        )
        assertEquals(expected, node(dsl, recs))
    }

    @Test
    fun sharedEntityShowsOnlyOwnTurns() {
        val dsl = dsl("""{"type":"aiChat","id":"talk","entity":"turn","textField":"text","roleField":"role"}""", scope = "shared")
        val recs = records(
            """[
              {"id":"m","data":{"text":"自分","role":"user"},"author":"me","createdAt":"2026-11-12T10:00:00+09:00"},
              {"id":"o","data":{"text":"他の人","role":"user"},"author":"other","createdAt":"2026-11-12T10:01:00+09:00"}
            ]""",
        )
        val turns = node(dsl, recs)["props"]!!.jsonObject["turns"]
        assertEquals(json("""[{"recordId":"m","role":"user","text":"自分"}]"""), turns)
    }

    @Test
    fun emptyConversationHasEmptyTurns() {
        val dsl = dsl("""{"type":"aiChat","id":"talk","entity":"turn","textField":"text","roleField":"role"}""")
        assertEquals(json("[]"), node(dsl, emptyList())["props"]!!.jsonObject["turns"])
    }
}
