package jp.kz2601.metamon.renderer.conformance

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonPrimitive

/** 描画の木のうち、センサーの Leaf(#871)の部分(iOS の RenderTree.swift の declared の sensor / seconds)。 */
object SensorNodes {
    fun props(c: Component): Map<String, JsonElement> {
        // locationTracker(#869)は seconds だけ(iOS の declared の seconds)
        if (c.type == ComponentType.LOCATION_TRACKER) return c.seconds?.let { mapOf("seconds" to JsonPrimitive(it.toDouble())) }.orEmpty()
        if (c.type != ComponentType.SENSOR_VALUE) return emptyMap()
        return buildMap {
            c.sensor?.let { put("sensor", JsonPrimitive(it.wire)) }
            c.seconds?.let { put("seconds", JsonPrimitive(it.toDouble())) }
        }
    }
}
