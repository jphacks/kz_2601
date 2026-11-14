package jp.kz2601.metamon.dsl.records

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.io.File
import java.time.Instant
import java.time.LocalDateTime
import java.time.ZoneId

/** 正典の例(docs/dsl/examples)を読む。iOS の ExamplesFixture。entity は Entity.from、部品は JSON のまま。 */
object Fixtures {
    val TOKYO: ZoneId = ZoneId.of("Asia/Tokyo")

    val examplesDir: File = File(
        requireNotNull(System.getProperty("metamon.dslExamplesDir")) { "metamon.dslExamplesDir が未設定(Gradle から実行する)" },
    )
    val schemaFile: File get() = examplesDir.resolve("../dsl.schema.json").normalize()

    /** dsl.schema.json の `$defs`。 */
    fun schemaDefs(): JsonObject = (Json.parseToJsonElement(schemaFile.readText()) as JsonObject)["\$defs"] as JsonObject

    class Example(val root: JsonObject) {
        val entities: List<Entity> = Entity.listFrom((root["entities"] as? JsonArray)?.map { it as JsonObject } ?: emptyList())
        fun entity(id: String): Entity? = entities.firstOrNull { it.id == id }

        /** 全画面の部品(子も含む)。 */
        val allComponents: List<JsonObject>
            get() = (root["screens"] as JsonArray).flatMap { components((it as JsonObject).str("id")!!) }

        /** 画面の部品(子も含む・2 段)。 */
        fun components(screen: String): List<JsonObject> {
            val s = (root["screens"] as JsonArray).map { it as JsonObject }.first { (it["id"] as JsonPrimitive).content == screen }
            return (s["components"] as JsonArray).flatMap { c ->
                c as JsonObject
                listOf(c) + ((c["children"] as? JsonArray)?.map { it as JsonObject } ?: emptyList())
            }
        }
    }

    fun load(name: String): Example = Example(Json.parseToJsonElement(examplesDir.resolve("$name.json").readText()) as JsonObject)

    fun JsonObject.str(key: String): String? = (this[key] as? JsonPrimitive)?.takeIf { it.isString }?.content

    /** 端末のタイムゾーン(Tokyo に固定)の日時。 */
    fun at(y: Int, m: Int, d: Int, h: Int = 12, min: Int = 0, zone: ZoneId = TOKYO): Instant =
        LocalDateTime.of(y, m, d, h, min).atZone(zone).toInstant()

    fun rec(id: String, vararg data: Pair<String, Any?>, createdAt: String? = null, author: String? = null, authorName: String? = null) =
        Record(id = id, data = data.associate { it.first to JSONValue.of(it.second) }, createdAt = createdAt, author = author, authorName = authorName)
}
