package jp.kz2601.metamon

import jp.kz2601.metamon.renderer.components.sensors.MockSensorSource
import jp.kz2601.metamon.renderer.components.sensors.SensorKind
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

/** `METAMON_MOCK=1` の時だけ sensorValue を決まった波形で動かす(#1195・iOS の MockSensorData)。 */
class MockSensorSourceChoiceTest {
    @Test
    fun mockUsesTheFixedWaveform() {
        assertSame(MockSensorSource, AppContainer.sensorSourceFor(mock = true))
    }

    @Test
    fun otherwiseTheComponentReadsTheDeviceSensors() {
        assertNull(AppContainer.sensorSourceFor(mock = false))
    }

    @Test
    fun theTiltCrossesTheThirtyDegreeThreshold() {
        val tilts = (0..120).map { MockSensorSource.frame(SensorKind.TILT, it / 10.0).value }
        assertTrue(tilts.any { it < 30 })
        assertTrue(tilts.any { it > 30 })
        assertTrue(tilts.all { it in -0.001..40.001 })
    }
}
