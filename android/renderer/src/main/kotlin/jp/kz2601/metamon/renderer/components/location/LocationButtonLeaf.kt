package jp.kz2601.metamon.renderer.components.location

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.components.inputs.FormInput
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import kotlinx.coroutines.launch

/**
 * `locationButton`(iOS の `LocationButtonLeaf`): 押すと現在地を 1 回取り、form の field に "lat,lng" を入れる。
 * `addressField` があれば端末の Geocoder で住所にして入れる(取れなければ座標だけ)。権限は押した時に聞く。
 */
@Composable
fun LocationButtonLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var busy by remember { mutableStateOf(false) }
    var problem by rememberSaveable { mutableStateOf<LocationProblem?>(null) }

    fun capture() {
        busy = true
        scope.launch {
            try {
                when (val fix = OneShotLocation.request(context)) {
                    is LocationFix.Got -> {
                        val lat = fix.location.latitude
                        val lng = fix.location.longitude
                        form.values[field] = JSONValue.Str(LocationButtonRules.coordinate(lat, lng))
                        problem = null
                        LocationButtonRules.addressTarget(c)?.let { af ->
                            OneShotLocation.address(context, lat, lng)?.let { form.values[af] = JSONValue.Str(it) }
                        }
                    }
                    is LocationFix.Problem -> problem = fix.problem
                }
            } finally {
                busy = false
            }
        }
    }

    val ask = LocalLocationPermission.current { granted -> if (granted) capture() else problem = LocationProblem.DENIED }

    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs)) {
        ActionButton(
            label = c.label ?: stringResource(R.string.renderer_location_button),
            kind = ActionButtonKind.SECONDARY,
            icon = "location",
            running = busy,
            onClick = ask,
        )
        val saved = LocationButtonRules.preview(form.values[field])
        val savedDescription = saved?.let { stringResource(R.string.renderer_location_saved, it) }
        val err = problem
        if (saved != null) {
            Text(
                "📍 $saved",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.semantics {
                    contentDescription = savedDescription.orEmpty()
                    liveRegion = LiveRegionMode.Polite
                },
            )
        } else if (err != null) {
            Text(
                stringResource(problemText(err)),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.error,
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
            )
        }
    }
}

@StringRes
private fun problemText(p: LocationProblem): Int = when (p) {
    LocationProblem.DENIED -> R.string.renderer_location_denied
    LocationProblem.OFF -> R.string.renderer_location_off
    LocationProblem.FAILED -> R.string.renderer_location_failed
}
