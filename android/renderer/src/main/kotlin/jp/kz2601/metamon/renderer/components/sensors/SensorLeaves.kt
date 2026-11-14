package jp.kz2601.metamon.renderer.components.sensors

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** センサーの Leaf(#871)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerSensorLeaves() {
    register(ComponentType.SENSOR_VALUE) { c, ctx, m -> SensorValueLeaf(c, ctx, m) }
}
