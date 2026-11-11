package jp.kz2601.metamon.renderer.components.map

import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.lists.rememberLeafData
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors

/**
 * `mapView`(iOS の `MapViewLeaf`): `entity.locationField`("lat,lng")を持つ記録ごとにピン。ピンのタップで `onItemTap`(recordId 付き)。
 * `zoom`: near(約 1km)/ city(約 10km・既定)/ wide(約 100km)。ピンが無ければ東京駅を中心に出す。
 * `kind: "route"`: 点を `dateField`(省略時は作成順)の時刻順に線で結ぶ。10 分以上空いた所は別の記録として線を切る。
 * TalkBack: 地図は「N 件の場所の地図」と読み、ピンは操作(カスタムアクション)で開ける。
 */
@Composable
fun MapViewLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val data = rememberLeafData(ctx.runtime)
    val entityId = c.entity.orEmpty()
    val records = data.viewRecords(entityId)
    val shape = RoundedCornerShape(RenderTheme.Radius.card)
    val box = modifier.fillMaxWidth().fillMaxHeight().heightIn(min = 160.dp).clip(shape)
    val tint = MaterialTheme.colorScheme.primary.toArgb()
    val handle = remember { MapHandle() }
    val style = handle.style

    if (MapRules.isRoute(c)) {
        val segments = MapRules.segments(records, c)
        val latest = segments.lastOrNull().orEmpty()
        val startTint = RenderTheme.colors.positive.toArgb()
        LaunchedEffect(style, segments) { style?.let { MapLayers.route(it, segments, tint, startTint) } }
        val description = stringResource(R.string.renderer_map_route, latest.size)
        MapLibreView(
            camera = MapRules.fit(latest, MapRules.routeMinSpan(c.zoom)),
            cameraKey = latest.size,
            modifier = box.testTag("mapView.route").semantics { contentDescription = description },
            handle = handle,
        )
        return
    }

    val entity = data.entity(entityId)
    val titleField = c.titleField ?: entity?.titleField
    val pins = MapRules.pins(records, c.locationField) { r -> titleField?.let { data.display(r[it], entity?.field(it)) }.orEmpty() }
    LaunchedEffect(style, pins) { style?.let { MapLayers.pins(it, pins, tint) } }
    val action = c.onItemTap
    fun open(id: String) {
        action?.let { ctx.dispatch(it, ActionContext(ctx.route, ctx.form, recordId = id)) }
    }
    val slop = with(LocalDensity.current) { 16.dp.toPx() }
    val description = pluralStringResource(R.plurals.renderer_map_pins, pins.size, pins.size)
    val untitled = stringResource(R.string.renderer_map_pin_untitled)
    MapLibreView(
        camera = MapRules.camera(pins.map { it.at }, MapRules.spanMeters(c.zoom)),
        cameraKey = pins.map { it.id },
        modifier = box.testTag("mapView").semantics {
            contentDescription = description
            if (action != null) {
                customActions = pins.map { p -> CustomAccessibilityAction(p.title.ifEmpty { untitled }) { open(p.id); true } }
            }
        },
        handle = handle,
        onTap = { map, at ->
            val id = if (action != null) MapLayers.pinAt(map, at.x, at.y, slop) else null
            id?.let(::open)
            id != null
        },
    )
}
