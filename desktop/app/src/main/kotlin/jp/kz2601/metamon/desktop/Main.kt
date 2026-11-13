package jp.kz2601.metamon.desktop

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.key
import androidx.compose.runtime.remember
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.ApplicationScope
import androidx.compose.ui.window.Window
import androidx.compose.ui.window.WindowPosition
import androidx.compose.ui.window.application
import androidx.compose.ui.window.rememberWindowState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.launch
import java.awt.Dimension
import java.awt.GraphicsEnvironment
import java.awt.Rectangle
import java.awt.event.WindowAdapter
import java.awt.event.WindowEvent
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.io.DragOut
import jp.kz2601.metamon.desktop.platform.SendToArgs
import jp.kz2601.metamon.desktop.platform.SendToInbox
import jp.kz2601.metamon.desktop.platform.SendToShortcut
import jp.kz2601.metamon.desktop.store.DesktopStore
import jp.kz2601.metamon.desktop.store.DeviceWipe
import jp.kz2601.metamon.desktop.store.SessionPersistence
import jp.kz2601.metamon.desktop.theme.TailorItTheme
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.dsl.screen
import java.io.File
import java.util.prefs.Preferences
import javax.swing.SwingUtilities

/**
 * 起動。引数 1 つ目(または -Dtailorit.example)で同梱の例を選ぶ。既定は shopping。
 * 引数 2 つ目に記録の JSON(`{"<entity>": [{"id", "data"}]}`・case.json の形も可)を渡すとモックに入れて開く(端末には残さない)。3 つ目は最初の画面の id。
 * 引数が無ければ最後に開いたアプリ。記録は端末に残す(SessionPersistence)。端末の記録は裏で読み、読み終えるまでスケルトンを出す。
 * ウィンドウはアプリごとに 1 枚、記録の詳細も別の 1 枚で開ける(#1763・Windows.kt)。同じアプリのウィンドウは記録を共有する。
 * ウィンドウの大きさ・位置は種類(アプリ / 詳細)ごとに覚えて次の起動で戻す。トレイ / メニューバーに常駐する(Tray.kt)。サインインはまだ無い(モック)。
 * 同じ保存先ではプロセスを 1 つだけ起動する。2 つ目は起動中の方へ引数を渡して前に出させる(SingleInstance.kt)。
 * 記録の JSON を渡した時(開発・スクリーンショット)は端末に残さないので、ロックを取らずに何本でも起動できる。
 */
fun main(argv: Array<String>) {
    // 「送る」(#3209)は `--send-to` の後ろに選んだファイルが付く。ほかの引数の並びは崩さない
    val sent = SendToArgs.files(argv.toList())
    val args = SendToArgs.strip(argv.toList()).toTypedArray()
    // 同じ保存先で 2 つ目は起動しない(#1754 の JSON を 2 つのプロセスで書き合わない)
    if (args.getOrNull(1) == null && !SingleInstance.claim()) {
        if (!SingleInstance.handOff(args = argv.toList())) SingleInstance.reportRunning()
        return
    }
    runApp(args, sent)
}

private fun runApp(args: Array<String>, sent: List<File>? = null) = application {
    // 端末の保存(#1887): 記録・設定は再起動しても残る。最後に開いたアプリから開く
    val desktopStore = remember { DesktopStore.open() }
    val first = remember {
        args.firstOrNull() ?: System.getProperty("tailorit.example")
            ?: desktopStore.lastAppId?.takeIf { it in BundledApps.ids() } ?: "shopping"
    }
    // 端末の記録は裏で読む。読み終えるまでウィンドウはスケルトン(SharedRecords.loading)
    val loadScope = remember { CoroutineScope(SupervisorJob() + Dispatchers.IO) }
    // 前に落ちた時などに残った引き出し・写真・カレンダーに渡した一時フォルダ(1 日より古い物)を裏で消す
    remember {
        loadScope.launch {
            runCatching { DragOut.sweep() }
            runCatching { DragOut.sweep(root = PhotoOpen.tempRoot()) }
            runCatching { DesktopActionHost.sweepCalendar() }
        }
    }
    val hub = remember {
        // 記録の JSON を渡した時(開発・スクリーンショット)は最初のアプリだけそれを使い、端末に残さない
        val fixture = args.getOrNull(1)?.let { MockStore.fromJson(File(it).readText()) }
        AppHub(
            openRecords = { id, dsl ->
                if (fixture != null && id == first) {
                    SharedRecords(fixture)
                } else {
                    val p = SessionPersistence.create(desktopStore, id)
                    val models = dsl.entityModels()
                    // 記録は Swing の画面のスレッドでだけ書き換える(断られた分を混ぜる時も)
                    SharedRecords.opening(p, loadScope, post = { SwingUtilities.invokeLater(it) }) { p.load(models) }
                }
            },
            wipeStore = {
                desktopStore.wipe()
                DeviceWipe.run(desktopStore.root)
            },
        )
    }
    DisposableEffect(hub) {
        onDispose {
            // 終わる時: 全部のアプリの積んだ書き込みを書き終えるのを待ってから(3 秒まで)閉じる
            hub.close()
            loadScope.cancel()
        }
    }
    val registry = remember { WindowRegistry().also { it.open(WindowKey.app(first)) } }
    val firstWindowId = remember { registry.windows.first().id }
    // 「送る」で届いたファイル(#3209)。起動の時の分は最初のウィンドウへ
    val sendTo = remember { SendToInbox().also { box -> sent?.let { box.offer(SendToArgs.readable(it), firstWindowId) } } }
    // 入れた物(MSI)で動いている Windows なら「送る」にショートカットを置く(置いてあれば何もしない)
    remember { loadScope.launch { SendToShortcut.create().ensure() } }
    // トレイに常駐(#1761): × はトレイに隠す(設定で off・トレイが無ければ終わる)。終わるのはトレイの「終了」
    val tray = remember { TrayController.create(hub.dsl(first)) }
    tray.onOpenApp = { registry.open(WindowKey.app(it)) }
    // 2 つ目の起動: 渡されたアプリ(無ければ前にあるアプリ)を開いて前に出す。トレイに隠れていても出す
    remember {
        SingleInstance.onHandoff { argv ->
            val files = SendToArgs.files(argv)
            val a = SendToArgs.strip(argv)
            SwingUtilities.invokeLater {
                val id = a.firstOrNull()?.takeIf { it in BundledApps.ids() } ?: registry.active?.key?.appId ?: first
                tray.openApp(id)
                // 送られたファイルは前にあるアプリの、アプリのウィンドウへ(すぐ記録と同じ行き先)
                if (files != null) sendTo.offer(SendToArgs.readable(files), registry.quickRecordWindow()?.id)
            }
        }
    }
    // すぐ記録は前にあるアプリの、アプリのウィンドウへ(詳細には入れない・無ければ開く)
    tray.findQuickRecordWindow = { registry.quickRecordWindow()?.id }
    // トレイの名前・すぐ記録・集計は前にあるウィンドウのアプリ。閉じて開き直すと記録が替わるのでウィンドウごとに見直す
    val active = registry.active
    LaunchedEffect(active?.id) {
        val appId = active?.key?.appId ?: return@LaunchedEffect
        tray.switchApp(hub.dsl(appId), hub.opened().firstOrNull { it.id == appId }?.records)
    }
    DesktopTray(tray)
    registry.windows.toList().forEach { w ->
        key(w.id) {
            DesktopWindow(w, registry, hub, tray, sendTo, startScreen = args.getOrNull(2)?.takeIf { w.id == firstWindowId })
        }
    }
}

/** ウィンドウ 1 枚。画面の状態(遷移・下書き・キー)はウィンドウごと、記録はアプリごとに共有。 */
@OptIn(FlowPreview::class)
@Composable
private fun ApplicationScope.DesktopWindow(w: WindowEntry, registry: WindowRegistry, hub: AppHub, tray: TrayController, sendTo: SendToInbox, startScreen: String?) {
    // アプリの記録はウィンドウの数で数える。最後の 1 枚を閉じたら書き終えて閉じる
    val app = remember(w) { hub.acquire(w.key.appId) }
    DisposableEffect(w) { onDispose { hub.release(w.key.appId) } }
    val kind = w.key.kind
    val saved = remember(w) {
        WindowBounds.load(kind)?.let { WindowBounds.fit(WindowBounds.cascade(it, registry.indexInKind(w)), WindowBounds.screens()) }
    }
    val state = rememberWindowState(
        size = saved?.size ?: WindowBounds.defaultSize(kind),
        position = saved?.position ?: WindowPosition.PlatformDefault,
    )
    val close = {
        if (registry.requestClose(w.id) { tray.onCloseRequest() }) exitApplication()
    }
    val screenTitle = w.key.screen?.let { app.dsl.screen(it)?.title?.ifEmpty { null } }
    Window(
        onCloseRequest = close,
        visible = tray.windowVisible,
        title = if (screenTitle != null) Strings.format("desktop_window_title_record", app.dsl.app.name, screenTitle) else Strings.format("desktop_window_title", app.dsl.app.name),
        state = state,
    ) {
        // トレイの「開く」は前にあるウィンドウ、同じ物をもう一度開いた時はそのウィンドウを前に出す
        LaunchedEffect(tray.focusRequests) {
            if (tray.focusRequests > 0 && registry.active?.id == w.id) {
                state.isMinimized = false
                window.toFront()
            }
        }
        LaunchedEffect(w.focusRequests) {
            if (w.focusRequests > 0) {
                state.isMinimized = false
                window.toFront()
                window.requestFocus()
            }
        }
        DisposableEffect(window) {
            val l = object : WindowAdapter() {
                override fun windowGainedFocus(e: WindowEvent?) {
                    registry.activate(w.id)
                    // 前に出た: 続けて失敗して止まった送り待ちを起こす
                    app.records.persistence?.resume()
                }

                // 外で離した Space は届かない: 押しっぱなしの覚えを捨てる(次の押し下げを取りこぼさない)
                override fun windowLostFocus(e: WindowEvent?) = resetSpaceRepeats()
            }
            window.addWindowFocusListener(l)
            onDispose { window.removeWindowFocusListener(l) }
        }
        LaunchedEffect(Unit) {
            // 6 列が潰れない最小(1 列 ≈ 50dp)
            window.minimumSize = Dimension(360, 560)
            snapshotFlow { state.size to state.position }.debounce(500).collect { (size, pos) -> WindowBounds.save(kind, size, pos) }
        }
        // 明暗は OS に従う・文言は OS の言語(日本語か英語)・#1759
        CompositionLocalProvider(LocalTray provides tray) {
            TailorItTheme {
                if (app.records.loading) {
                    LoadingSkeleton()
                } else {
    DesktopApp(
                        app.dsl,
                        startScreen = w.key.screen ?: startScreen,
                        onOpenApp = { registry.open(WindowKey.app(it)) },
                        records = app.records,
                        startRecordId = w.key.recordId,
                        onOpenRecord = { screen, id -> registry.open(WindowKey.record(app.id, screen, id)) },
                        // 画面の中の「閉じる」・Esc は終わらせない(最後の 1 枚ならアプリのウィンドウを開いてから閉じる)。トレイ・終了はタイトルバーの × だけ
                        onCloseWindow = { registry.closeInApp(w.id) },
                        isActive = { registry.active?.id == w.id },
                        windowId = w.id,
                        onWipeLocalData = hub::wipeLocalData,
                        sendTo = sendTo,
                    )
                }
            }
        }
    }
}

/** ウィンドウの大きさ・位置(java.util.prefs: Windows はレジストリ、Linux は ~/.java)。種類(アプリ / 詳細)ごとに覚える。 */
internal object WindowBounds {
    private val prefs by lazy { Preferences.userRoot().node("jp/kz2601/tailorit/desktop") }

    data class Bounds(val size: DpSize, val position: WindowPosition)

    /** 種類ごとの鍵の頭。アプリのウィンドウは前からの鍵(w・h・x・y)のまま。 */
    fun prefix(kind: WindowKind): String = when (kind) {
        WindowKind.APP -> ""
        WindowKind.RECORD -> "record."
    }

    /** 覚えた物が無い時の大きさ。詳細は 1 件のフォームなので少し小さく。 */
    fun defaultSize(kind: WindowKind): DpSize = when (kind) {
        WindowKind.APP -> DpSize(480.dp, 820.dp)
        WindowKind.RECORD -> DpSize(440.dp, 640.dp)
    }

    fun load(kind: WindowKind = WindowKind.APP): Bounds? {
        val k = prefix(kind)
        val w = prefs.getFloat("${k}w", -1f)
        val h = prefs.getFloat("${k}h", -1f)
        if (w < 360 || h < 560) return null
        val x = prefs.getFloat("${k}x", Float.NaN)
        val y = prefs.getFloat("${k}y", Float.NaN)
        val pos = if (x.isNaN() || y.isNaN()) WindowPosition.PlatformDefault else WindowPosition(x.dp, y.dp)
        return Bounds(DpSize(w.dp, h.dp), pos)
    }

    /** 同じ種類の [index] 枚目は右下へずらす(同じ場所にぴったり重ねない)。画面に収めるのは [fit]。 */
    fun cascade(b: Bounds, index: Int): Bounds {
        val pos = b.position as? WindowPosition.Absolute ?: return b
        if (index <= 0) return b
        return Bounds(b.size, WindowPosition(pos.x + CASCADE * index, pos.y + CASCADE * index))
    }

    private val CASCADE = 32.dp

    /** 今つながっている画面(外した画面に開かないため)。分からなければ空。 */
    fun screens(): List<Rectangle> = runCatching {
        if (GraphicsEnvironment.isHeadless()) emptyList() else GraphicsEnvironment.getLocalGraphicsEnvironment().screenDevices.map { it.defaultConfiguration.bounds }
    }.getOrDefault(emptyList())

    /**
     * 覚えた位置・大きさを今の画面に収める。大きさは一番大きい画面まで。
     * 見出し(左上)がどの画面にも乗らない位置(外した画面・解像度を下げた)は OS の既定の位置に戻す。
     */
    fun fit(b: Bounds, screens: List<Rectangle>): Bounds {
        if (screens.isEmpty()) return b
        val maxW = screens.maxOf { it.width }.toFloat()
        val maxH = screens.maxOf { it.height }.toFloat()
        val size = DpSize(b.size.width.value.coerceAtMost(maxW).dp, b.size.height.value.coerceAtMost(maxH).dp)
        val pos = b.position as? WindowPosition.Absolute ?: return Bounds(size, b.position)
        val title = Rectangle(pos.x.value.toInt(), pos.y.value.toInt(), size.width.value.toInt().coerceIn(1, TITLE_W), TITLE_H)
        val onScreen = screens.firstOrNull { it.intersection(title).let { r -> !r.isEmpty && r.width >= TITLE_MIN_VISIBLE && r.height >= TITLE_H / 2 } }
            ?: return Bounds(size, WindowPosition.PlatformDefault)
        // 乗っている画面の中に収まるよう右下・左上を寄せる
        val x = pos.x.value.coerceIn(onScreen.x.toFloat(), (onScreen.x + onScreen.width - size.width.value).coerceAtLeast(onScreen.x.toFloat()))
        val y = pos.y.value.coerceIn(onScreen.y.toFloat(), (onScreen.y + onScreen.height - size.height.value).coerceAtLeast(onScreen.y.toFloat()))
        return Bounds(size, WindowPosition(x.dp, y.dp))
    }

    private const val TITLE_W = 200
    private const val TITLE_H = 32
    private const val TITLE_MIN_VISIBLE = 48

    fun save(kind: WindowKind, size: DpSize, pos: WindowPosition) {
        if (!size.width.value.isFinite() || !size.height.value.isFinite()) return
        val k = prefix(kind)
        prefs.putFloat("${k}w", size.width.value)
        prefs.putFloat("${k}h", size.height.value)
        if (pos is WindowPosition.Absolute) {
            prefs.putFloat("${k}x", pos.x.value)
            prefs.putFloat("${k}y", pos.y.value)
        }
    }
}
