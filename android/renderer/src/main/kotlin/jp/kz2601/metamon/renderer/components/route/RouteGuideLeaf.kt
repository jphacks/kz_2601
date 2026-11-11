package jp.kz2601.metamon.renderer.components.route

import android.Manifest
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationManager
import android.net.Uri
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.components.actions.ActionNotice
import jp.kz2601.metamon.renderer.components.lists.rememberLeafData
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard

/**
 * `routeGuide`(#868): 目的地までの道案内。Android 版は地図 SDK を持たず、行き先・車 / 徒歩・現在地からの直線距離を見せ、
 * 案内は端末の地図アプリ(Google マップのナビ → 地図アプリ → ブラウザの順)に渡す。位置は権限が既にある時だけ読む(ここでは聞かない)。
 * `address` の行き先は端末の Geocoder で緯度経度にして距離を出す(引けなければ距離を出さない・地図アプリには住所のまま渡す)。
 */
@Composable
fun RouteGuideLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val formEntity = ctx.form?.entity?.id
    if (RouteGuide.isMisconfigured(c, formEntity, ctx.form != null)) {
        ActionNotice(stringResource(R.string.renderer_route_missing_destination), modifier)
        return
    }
    val context = LocalContext.current
    val rt = ctx.runtime
    val address = RouteGuide.address(c)
    val choosesRecord = RouteGuide.choosesRecord(c, formEntity)
    val titleField = c.titleField ?: rt.entityModel(c.entity ?: formEntity.orEmpty())?.titleField
    val data = rememberLeafData(rt)
    val listEntity = c.entity?.takeIf { choosesRecord }
    if (listEntity != null) LaunchedEffect(listEntity) { rt.loadRecords(setOf(listEntity)) }

    val candidates = if (choosesRecord && listEntity != null && c.locationField != null) {
        RouteGuide.places(data.viewRecords(listEntity), c.locationField!!, titleField)
    } else {
        emptyList()
    }
    var selectedId by rememberSaveable { mutableStateOf<String?>(null) }
    val place: RoutePlace? = when {
        address != null -> null
        choosesRecord -> candidates.firstOrNull { it.id == selectedId } ?: candidates.firstOrNull()
        else -> ctx.form?.let { f ->
            c.locationField?.let { RouteGuide.place(f.recordId ?: "draft", f.values, it, titleField) }
        }
    }
    val target: RouteTarget? = address?.let(RouteTarget::Query) ?: place?.let(RouteTarget::Coord)
    var mode by rememberSaveable { mutableStateOf(RouteMode.of(c.mode)) }
    var failed by remember { mutableStateOf(false) }
    val here = remember { lastKnownLocation(context) }
    // 住所の行き先は端末の Geocoder で緯度経度にして距離を出す(#1350)。引いた結果は部品の中で住所ごとに覚える。
    val addressCache = remember { RouteAddressCache { RouteGeocoder.lookup(context, it) } }
    var addressLookup by remember(address) { mutableStateOf(address?.let(addressCache::peek)) }
    if (address != null) {
        LaunchedEffect(address) { if (addressLookup == null) addressLookup = addressCache.resolve(address) }
    }
    val distancePlace = place ?: address?.let { addressLookup.place(it) }
    val destinationLabel = stringResource(R.string.renderer_route_destination)
    fun name(p: RoutePlace) = p.name.ifEmpty { destinationLabel }

    Column(modifier.renderCard(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        c.title?.takeIf { it.isNotEmpty() }?.let {
            Text(
                it,
                Modifier.semantics { heading() },
                style = RenderTheme.Fonts.sectionTitle,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        DestinationRow(
            text = address ?: place?.let(::name) ?: destinationLabel,
            choices = if (candidates.size > 1) candidates else emptyList(),
            choiceName = ::name,
            onSelect = { selectedId = it.id; failed = false },
        )
        if (distancePlace != null && here != null) {
            val meters = RouteGuide.distanceMeters(here.latitude, here.longitude, distancePlace.lat, distancePlace.lng)
            Text(
                stringResource(R.string.renderer_route_distance, RouteGuide.formatDistance(meters)),
                Modifier.testTag("routeGuide.distance"),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        val modes = listOf(RouteMode.AUTOMOBILE to R.string.renderer_route_mode_car, RouteMode.WALKING to R.string.renderer_route_mode_walk)
        val modeLabel = stringResource(R.string.renderer_route_mode)
        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth().testTag("routeGuide.mode").semantics { contentDescription = modeLabel }) {
            modes.forEachIndexed { i, (m, res) ->
                SegmentedButton(
                    selected = mode == m,
                    onClick = { mode = m },
                    shape = SegmentedButtonDefaults.itemShape(i, modes.size),
                ) { Text(stringResource(res), maxLines = 1) }
            }
        }
        ActionButton(
            label = c.label?.takeIf { it.isNotEmpty() } ?: stringResource(R.string.renderer_route_start),
            kind = ActionButtonKind.PRIMARY,
            onClick = { failed = target?.let { !openRoute(context, RouteGuide.uris(it, mode)) } ?: false },
            modifier = Modifier.testTag("routeGuide.start"),
            icon = "location",
            enabled = target != null,
        )
        val note = when {
            failed -> stringResource(R.string.renderer_route_no_app)
            address != null && addressLookup == AddressLookup.NotFound -> stringResource(R.string.renderer_route_address_not_found, address)
            target == null && choosesRecord -> stringResource(R.string.renderer_route_no_places)
            target == null -> stringResource(R.string.renderer_route_choose_place)
            else -> stringResource(R.string.renderer_route_opens_maps)
        }
        Text(
            note,
            style = MaterialTheme.typography.bodySmall,
            color = if (failed) RenderTheme.colors.negative else MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun DestinationRow(text: String, choices: List<RoutePlace>, choiceName: (RoutePlace) -> String, onSelect: (RoutePlace) -> Unit) {
    var open by remember { mutableStateOf(false) }
    val chooseLabel = stringResource(R.string.renderer_route_choose_destination)
    Box {
        Row(
            Modifier
                .fillMaxWidth()
                .heightIn(min = 32.dp)
                .then(if (choices.isNotEmpty()) Modifier.clickable { open = true }.semantics { contentDescription = "$chooseLabel: $text" } else Modifier)
                .testTag("routeGuide.destination"),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            SymbolIcon("mappin", 18.dp, MaterialTheme.colorScheme.primary)
            Text(
                text,
                Modifier.weight(1f, fill = false),
                style = MaterialTheme.typography.titleSmall.copy(fontWeight = FontWeight.SemiBold),
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (choices.isNotEmpty()) SymbolIcon("chevron.down", 14.dp, MaterialTheme.colorScheme.onSurfaceVariant)
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            choices.forEach { p ->
                DropdownMenuItem(text = { Text(choiceName(p)) }, onClick = { open = false; onSelect(p) })
            }
        }
    }
}

/** URI を先頭から開く。開けるアプリが無ければ次を試す。@return 開けたか */
private fun openRoute(context: Context, uris: List<String>): Boolean {
    for (uri in uris) {
        try {
            context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(uri)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
            return true
        } catch (_: ActivityNotFoundException) {
            continue
        }
    }
    return false
}

/** 端末が最後に知っている現在地。位置の権限が無ければ読まない(聞くのはアプリの権限の窓口)。 */
private fun lastKnownLocation(context: Context): Location? {
    val granted = listOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION).any {
        ContextCompat.checkSelfPermission(context, it) == PackageManager.PERMISSION_GRANTED
    }
    if (!granted) return null
    val lm = context.getSystemService(Context.LOCATION_SERVICE) as? LocationManager ?: return null
    return try {
        lm.getProviders(true).mapNotNull { lm.getLastKnownLocation(it) }.maxByOrNull { it.time }
    } catch (_: SecurityException) {
        null
    }
}
