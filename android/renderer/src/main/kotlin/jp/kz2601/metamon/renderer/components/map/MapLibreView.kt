package jp.kz2601.metamon.renderer.components.map

import android.annotation.SuppressLint
import android.view.MotionEvent
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.openExternalUrl
import org.maplibre.android.MapLibre
import org.maplibre.android.camera.CameraPosition
import org.maplibre.android.camera.CameraUpdateFactory
import org.maplibre.android.maps.MapLibreMap
import org.maplibre.android.maps.MapLibreMapOptions
import org.maplibre.android.maps.MapView
import org.maplibre.android.maps.Style
import java.net.URI
import org.maplibre.android.geometry.LatLng as MlLatLng

/** 地図の今の状態。読み込み前は [map] / [style] が null。 */
@Stable
class MapHandle {
    var map: MapLibreMap? by mutableStateOf(null)
        internal set

    /** スタイル(読み込み直すとピン・線の層が消えるので、これを鍵に描き直す)。 */
    var style: Style? by mutableStateOf(null)
        internal set

    /** 地図の中心(locationPicker の「この場所に決める」)。 */
    fun center(): LatLng? = map?.cameraPosition?.target?.let { LatLng(it.latitude, it.longitude) }

    fun move(camera: MapCamera, viewDp: Double) {
        val m = map ?: return
        m.moveCamera(
            CameraUpdateFactory.newCameraPosition(
                CameraPosition.Builder()
                    .target(MlLatLng(camera.center.lat, camera.center.lng))
                    .zoom(MapRules.zoom(camera, viewDp))
                    .build(),
            ),
        )
    }
}

/**
 * MapLibre の地図(OpenFreeMap のタイル・[MapStyle])。右下に帰属表示を必ず出す(押すと OSM の著作権の説明)。
 * MapLibre の「i」ボタンとロゴは出さない(帰属表示は自前で常に見せる)。
 *
 * @param camera 見せる範囲。[cameraKey] が変わった時(と大きさが変わった時)だけ動かす(ユーザーが動かした地図を戻さない)
 * @param interactive false なら動かせない(フォームの小さな地図)
 * @param onTap 地図を押した点(画面の px)。ピンの判定は呼び出し側
 */
@SuppressLint("ClickableViewAccessibility")
@Composable
fun MapLibreView(
    camera: MapCamera,
    cameraKey: Any?,
    modifier: Modifier = Modifier,
    handle: MapHandle = remember { MapHandle() },
    interactive: Boolean = true,
    onTap: ((MapLibreMap, android.graphics.PointF) -> Boolean)? = null,
) {
    val context = LocalContext.current
    val density = LocalDensity.current
    val dark = MaterialTheme.colorScheme.background.luminance() < 0.5f
    val tap by rememberUpdatedState(onTap)
    var size by remember { mutableStateOf(IntSize.Zero) }
    val mapView = remember {
        MapLibre.getInstance(context)
        MapView(context, MapLibreMapOptions.createFromAttributes(context).textureMode(true)).apply {
            if (interactive) {
                // 縦に流れる画面の中でも、地図の上の指は地図が受ける
                setOnTouchListener { v, e ->
                    if (e.actionMasked == MotionEvent.ACTION_DOWN) v.parent?.requestDisallowInterceptTouchEvent(true)
                    false
                }
            }
        }
    }

    val lifecycle = LocalLifecycleOwner.current.lifecycle
    DisposableEffect(lifecycle, mapView) {
        var created = false
        val observer = LifecycleEventObserver { _, event ->
            when (event) {
                Lifecycle.Event.ON_CREATE -> { mapView.onCreate(null); created = true }
                Lifecycle.Event.ON_START -> mapView.onStart()
                Lifecycle.Event.ON_RESUME -> mapView.onResume()
                Lifecycle.Event.ON_PAUSE -> mapView.onPause()
                Lifecycle.Event.ON_STOP -> mapView.onStop()
                else -> Unit
            }
        }
        lifecycle.addObserver(observer)
        onDispose {
            lifecycle.removeObserver(observer)
            if (created) {
                if (lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED)) mapView.onPause()
                if (lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) mapView.onStop()
                mapView.onDestroy()
            }
            handle.map = null
            handle.style = null
        }
    }

    LaunchedEffect(mapView, dark) {
        mapView.getMapAsync { m ->
            m.uiSettings.isAttributionEnabled = false
            m.uiSettings.isLogoEnabled = false
            m.uiSettings.isCompassEnabled = interactive
            m.uiSettings.setAllGesturesEnabled(interactive)
            if (handle.map == null) m.addOnMapClickListener { at -> tap?.invoke(m, m.projection.toScreenLocation(at)) ?: false }
            handle.map = m
            m.setStyle(Style.Builder().fromUri(MapStyle.url(dark))) { s -> handle.style = s }
        }
    }

    val map = handle.map
    LaunchedEffect(map, cameraKey, size) {
        if (map == null || size == IntSize.Zero) return@LaunchedEffect
        val shortSideDp = with(density) { minOf(size.width, size.height).toDp().value.toDouble() }
        handle.move(camera, shortSideDp)
    }

    Box(modifier.onSizeChanged { size = it }) {
        AndroidView(factory = { mapView }, modifier = Modifier.matchParentSize())
        MapAttribution(Modifier.align(Alignment.BottomEnd))
    }
}

/** 帰属表示(OpenFreeMap © OpenMapTiles © OpenStreetMap contributors)。TalkBack はリンクとして読む。 */
@Composable
fun MapAttribution(modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val open = stringResource(R.string.renderer_map_attribution_open)
    Text(
        MapStyle.ATTRIBUTION,
        modifier
            .padding(4.dp)
            .background(MaterialTheme.colorScheme.surface.copy(alpha = 0.85f), RoundedCornerShape(4.dp))
            .clickable(onClickLabel = open, role = Role.Button) { openExternalUrl(context, URI(MapStyle.COPYRIGHT_URL)) }
            .padding(horizontal = 6.dp, vertical = 2.dp),
        style = MaterialTheme.typography.labelSmall,
        color = MaterialTheme.colorScheme.onSurface,
        maxLines = 2,
    )
}
