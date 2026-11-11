package jp.kz2601.metamon.renderer.conformance

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.logic.BracketDraw
import jp.kz2601.metamon.dsl.logic.FlashcardDeck
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.components.play.BracketRules
import jp.kz2601.metamon.renderer.components.play.FlashcardRules
import jp.kz2601.metamon.renderer.components.play.PlayRecords
import jp.kz2601.metamon.renderer.components.play.settlementTransfers
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/** 描画の木のうち、計算・遊び系の Leaf 5 種(#860)の部分(iOS の RenderTree.swift の declared / resolved)。 */
object PlayNodes {
    val types = setOf(
        ComponentType.BRACKET,
        ComponentType.FLASHCARDS,
        ComponentType.RANDOM_PICKER,
        ComponentType.SETTLEMENT,
        ComponentType.TIMER_DISPLAY,
    )

    /** DSL に書いた値(mode / seconds)+ 記録から解決した値。5 種でなければ空(entity と rowTap は RenderTree の共通部分が出す)。 */
    fun props(c: Component, runtime: DSLRuntime): Map<String, JsonElement> {
        if (c.type !in types) return emptyMap()
        val all = runtime.recordsFlow?.value.orEmpty()
        val p = LinkedHashMap<String, JsonElement>()
        c.mode?.let { p["mode"] = JsonPrimitive(it.wire) }
        c.seconds?.let { p["seconds"] = JsonPrimitive(it.toDouble()) }
        when (c.type) {
            ComponentType.SETTLEMENT -> {
                p["title"] = JsonPrimitive(c.title ?: "精算")
                p["transfers"] = JsonArray(
                    settlementTransfers(c, runtime, all).map {
                        JsonObject(mapOf("from" to JsonPrimitive(it.from), "to" to JsonPrimitive(it.to), "amount" to JsonPrimitive(it.amount)))
                    },
                )
            }
            ComponentType.FLASHCARDS -> p += flashcards(c, runtime, all)
            ComponentType.BRACKET -> p += bracket(c, runtime, all)
            else -> Unit
        }
        return p
    }

    /** 開いた直後の山札(filter → 登録順)。1 枚目が表で出る。 */
    private fun flashcards(c: Component, runtime: DSLRuntime, all: Map<String, List<Record>>): Map<String, JsonElement> {
        val entity = runtime.entityModel(c.entity.orEmpty())
        val toggle = FlashcardRules.toggleField(c, entity)
        val cards = FlashcardRules.cards(c, runtime, all, emptyMap())
        fun text(r: Record, f: String?) = PlayRecords.display(runtime, all, r[f.orEmpty()], entity?.field(f.orEmpty()))
        val (position, total) = FlashcardDeck().progress(cards)
        return mapOf(
            "cards" to JsonArray(
                cards.map { r ->
                    JsonObject(
                        buildMap {
                            put("recordId", JsonPrimitive(r.id))
                            put("front", JsonPrimitive(text(r, c.front)))
                            put("back", JsonPrimitive(text(r, c.back)))
                            if (toggle != null) put("learned", JsonPrimitive(FlashcardDeck.isLearned(r, toggle)))
                        },
                    )
                },
            ),
            "progress" to JsonPrimitive("$position / $total"),
        )
    }

    /** 回戦ごとの試合(枠の上 a・下 b・勝者 winner・不戦勝 bye)と優勝。 */
    private fun bracket(c: Component, runtime: DSLRuntime, all: Map<String, List<Record>>): Map<String, JsonElement> {
        val draw = BracketRules.draw(c, runtime, all, emptyMap())
        val entity = runtime.entityModel(c.entity.orEmpty())
        val f = c.titleField.orEmpty()
        fun entrant(r: Record?): JsonElement = r?.let {
            JsonObject(mapOf("recordId" to JsonPrimitive(it.id), "title" to JsonPrimitive(PlayRecords.display(runtime, all, it[f], entity?.field(f)))))
        } ?: JsonNull
        val rounds = draw.rounds.mapIndexed { i, matches ->
            JsonObject(
                mapOf(
                    "title" to JsonPrimitive(BracketRules.roundTitle(i + 1, draw.rounds.size)),
                    "matches" to JsonArray(
                        matches.map { m ->
                            JsonObject(
                                buildMap {
                                    put("a", entrant(m.a))
                                    put("b", entrant(m.b))
                                    put("winner", m.winner?.let { JsonPrimitive(if (it == BracketDraw.Side.A) "a" else "b") } ?: JsonNull)
                                    if (m.bye) put("bye", JsonPrimitive(true))
                                },
                            )
                        },
                    ),
                ),
            )
        }
        return mapOf("rounds" to JsonArray(rounds), "champion" to entrant(draw.champion))
    }
}
