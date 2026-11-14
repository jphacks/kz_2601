package jp.kz2601.metamon.renderer.components.map

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.systemBarsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.components.inputs.FormInput
import jp.kz2601.metamon.renderer.components.location.LocalLocationPermission
import jp.kz2601.metamon.renderer.components.location.LocationFix
import jp.kz2601.metamon.renderer.components.location.OneShotLocation
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * `locationPicker`(iOS の `LocationPickerLeaf`): 現在地以外の場所を選ぶ入力欄。押すと全画面で開き、
 * 地名・住所で検索(端末の Geocoder)/ 地図を動かして中央のピン / 現在地 から選ぶ。
 * 保存は locationButton と同じ "lat,lng"。`nameField` があれば検索で選んだ場所の名前も入れる。
 * フォームには選んだ場所の名前(無ければ座標)と小さな地図を出す。
 */
@Composable
fun LocationPickerLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    var open by rememberSaveable { mutableStateOf(false) }
    var pickedName by rememberSaveable { mutableStateOf<String?>(null) }
    val at = MapRules.coordinate(form.values[field])
    val title = c.label ?: stringResource(R.string.renderer_location_picker_choose)

    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs)) {
        if (at != null) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                SymbolIcon("mappin", 18.dp, MaterialTheme.colorScheme.primary)
                Text(
                    displayName(c, form, field, pickedName),
                    Modifier.testTag("locationPicker.value").semantics { liveRegion = LiveRegionMode.Polite },
                    style = MaterialTheme.typography.titleSmall.copy(fontWeight = FontWeight.SemiBold),
                    maxLines = 2,
                )
            }
            val handle = remember { MapHandle() }
            val tint = MaterialTheme.colorScheme.primary.toArgb()
            val style = handle.style
            LaunchedEffect(style, at) { style?.let { MapLayers.pins(it, listOf(MapPin("picked", "", at)), tint) } }
            // 地図は名前の行と同じことしか言わないので TalkBack では読まない(帰属表示は MapLibreView の中で読む)
            MapLibreView(
                camera = MapCamera(at, 800.0),
                cameraKey = at,
                modifier = Modifier.fillMaxWidth().height(110.dp).clip(RoundedCornerShape(RenderTheme.Radius.card)),
                handle = handle,
                interactive = false,
            )
        }
        ActionButton(
            label = if (at == null) title else stringResource(R.string.renderer_location_picker_change),
            kind = ActionButtonKind.SECONDARY,
            icon = "magnifyingglass",
            onClick = { open = true },
            modifier = Modifier.testTag("locationPicker.open"),
        )
    }
    if (open) {
        LocationPickerDialog(title, initial = at, onDismiss = { open = false }) { place ->
            MapRules.apply(place.at, place.name, form.values, field, c.nameField)
            pickedName = place.name
            open = false
        }
    }
}

private fun displayName(c: Component, form: FormDraft, field: String, picked: String?): String {
    c.nameField?.let { nf -> form.values[nf]?.stringValue?.takeIf { it.isNotEmpty() }?.let { return it } }
    picked?.takeIf { it.isNotEmpty() }?.let { return it }
    return form.values[field]?.stringValue.orEmpty()
}

/** 場所を選ぶ全画面: 検索欄 +(検索中は結果 / それ以外は地図と中央のピン)+ 現在地 / この場所に決める。 */
@Composable
private fun LocationPickerDialog(title: String, initial: LatLng?, onDismiss: () -> Unit, onPick: (FoundPlace) -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var query by rememberSaveable { mutableStateOf("") }
    var results by remember { mutableStateOf<List<FoundPlace>>(emptyList()) }
    var searching by remember { mutableStateOf(false) }
    var camera by remember { mutableStateOf(MapCamera(initial ?: MapRules.TOKYO, 2_000.0)) }
    var locating by remember { mutableStateOf(false) }
    var problem by remember { mutableStateOf(false) }
    val handle = remember { MapHandle() }

    LaunchedEffect(query) {
        val q = query.trim()
        if (q.isEmpty()) {
            results = emptyList()
            return@LaunchedEffect
        }
        searching = true
        delay(400) // 打鍵ごとに検索しない
        results = PlaceGeocoder.search(context, q)
        searching = false
    }

    fun locate() {
        locating = true
        scope.launch {
            when (val fix = OneShotLocation.request(context)) {
                is LocationFix.Got -> {
                    problem = false
                    camera = MapCamera(LatLng(fix.location.latitude, fix.location.longitude), 800.0)
                }
                is LocationFix.Problem -> problem = true
            }
            locating = false
        }
    }
    val ask = LocalLocationPermission.current { granted -> if (granted) locate() else problem = true }

    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Column(
            Modifier.fillMaxSize().background(MaterialTheme.colorScheme.surface).systemBarsPadding().imePadding(),
        ) {
            Row(
                Modifier.fillMaxWidth().padding(horizontal = RenderTheme.Spacing.sm),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                TextButton(onClick = onDismiss) { Text(stringResource(R.string.inputs_cancel)) }
                Text(
                    title,
                    Modifier.weight(1f).semantics { heading() },
                    style = MaterialTheme.typography.titleMedium,
                    maxLines = 1,
                )
            }
            OutlinedTextField(
                value = query,
                onValueChange = { query = it },
                modifier = Modifier.fillMaxWidth().padding(horizontal = RenderTheme.Spacing.md, vertical = RenderTheme.Spacing.xs)
                    .testTag("locationPicker.search"),
                placeholder = { Text(stringResource(R.string.renderer_location_picker_search)) },
                leadingIcon = { SymbolIcon("magnifyingglass", 18.dp, MaterialTheme.colorScheme.onSurfaceVariant) },
                trailingIcon = if (query.isEmpty()) null else {
                    {
                        val clear = stringResource(R.string.renderer_location_picker_clear)
                        IconButton(onClick = { query = "" }, Modifier.semantics { contentDescription = clear }) {
                            SymbolIcon("xmark", 18.dp, MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                },
                singleLine = true,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
            )
            if (query.isNotBlank()) {
                ResultList(results, searching, onPick)
            } else {
                Box(Modifier.weight(1f).fillMaxWidth()) {
                    val mapLabel = stringResource(R.string.renderer_location_picker_map)
                    MapLibreView(
                        camera = camera,
                        cameraKey = camera,
                        modifier = Modifier.fillMaxSize().semantics { contentDescription = mapLabel },
                        handle = handle,
                    )
                    // 中央のピン(先端が地図の中心に来るよう上にずらす)
                    Box(Modifier.align(Alignment.Center).offset(y = (-18).dp).clearAndSetSemantics { }) {
                        SymbolIcon("mappin", 36.dp, MaterialTheme.colorScheme.error)
                    }
                }
                Column(
                    Modifier.fillMaxWidth().padding(RenderTheme.Spacing.md),
                    verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
                ) {
                    Text(
                        stringResource(R.string.renderer_location_picker_hint),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    if (problem) {
                        Text(
                            stringResource(R.string.renderer_location_failed),
                            Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.error,
                        )
                    }
                    Row(horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                        ActionButton(
                            label = stringResource(R.string.renderer_location_picker_current),
                            kind = ActionButtonKind.SECONDARY,
                            icon = "location",
                            running = locating,
                            onClick = ask,
                            modifier = Modifier.weight(1f).testTag("locationPicker.current"),
                        )
                        ActionButton(
                            label = stringResource(R.string.renderer_location_picker_use_pin),
                            kind = ActionButtonKind.PRIMARY,
                            icon = "checkmark",
                            onClick = { (handle.center() ?: camera.center).let { onPick(FoundPlace(null, null, it)) } },
                            modifier = Modifier.weight(1f).testTag("locationPicker.usePin"),
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun androidx.compose.foundation.layout.ColumnScope.ResultList(results: List<FoundPlace>, searching: Boolean, onPick: (FoundPlace) -> Unit) {
    if (results.isEmpty()) {
        Box(Modifier.weight(1f).fillMaxWidth(), contentAlignment = Alignment.Center) {
            if (searching) {
                CircularProgressIndicator()
            } else {
                Text(
                    stringResource(R.string.renderer_location_picker_not_found),
                    Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        return
    }
    LazyColumn(Modifier.weight(1f).fillMaxWidth()) {
        itemsIndexed(results) { i, place ->
            Column(
                Modifier.fillMaxWidth().clickable { onPick(place) }
                    .padding(horizontal = RenderTheme.Spacing.md, vertical = RenderTheme.Spacing.sm)
                    .testTag("locationPicker.result.$i"),
            ) {
                Text(place.name ?: place.address ?: MapRules.value(place.at), style = MaterialTheme.typography.bodyLarge)
                if (place.name != null && !place.address.isNullOrEmpty()) {
                    Text(place.address, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
                }
            }
            HorizontalDivider()
        }
    }
}
