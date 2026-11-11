package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.notifications.ActivityStatReading
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.lists.ProgressValue
import jp.kz2601.metamon.renderer.components.lists.StatValue

/** startActivity の `stat`: stat / progressBar の部品を読む(iOS の `LiveActivityContent.statReading`)。 */
object ActivityStat {
    fun read(id: String, dsl: AppDSL, data: LeafData): ActivityStatReading? {
        val c = dsl.screens.asSequence().flatMap { it.components.asSequence().flatMap(::walk) }.firstOrNull { it.id == id } ?: return null
        val label = c.label?.takeIf { it.isNotBlank() }
        return when (c.type) {
            ComponentType.STAT -> ActivityStatReading(label, StatValue.resolve(c, data).text)
            ComponentType.PROGRESS_BAR -> {
                if (ProgressValue.missingTarget(c, data)) return ActivityStatReading(label)
                val ratio = ProgressValue.ratio(c, data)
                ActivityStatReading(label, ProgressValue.percentText(ratio, c.tone?.wire), ratio)
            }
            else -> null
        }
    }

    private fun walk(c: Component): Sequence<Component> = sequenceOf(c) + c.children.asSequence().flatMap(::walk)
}
