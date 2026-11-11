package jp.kz2601.metamon.voice.stage

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentStyle
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.hasTabs
import jp.kz2601.metamon.dsl.logic.initialTab
import jp.kz2601.metamon.renderer.layout.GridArea
import jp.kz2601.metamon.renderer.layout.gridPlacement
import jp.kz2601.metamon.services.StreamedComponent
import jp.kz2601.metamon.voice.BuildProgress

/**
 * 生成中の「パーツを選んで置く」演出の段取り(#894・iOS の `BuildChoreography`)。Compose を持たない純粋なロジックだけ。
 * - 待っている間([WaitingScript]): パーツ箱から部品を選んで空の画面に置く → 入れ替える → 並べ直す、を繰り返す
 * - 結果が届いた時([AssemblyPlan]): 本物の結果の部品を上から 1 つずつ置いて、本物の画面に切り替える(最長 1.5 秒)
 * 待っている間の部品は依頼文から当てた「下書き」。SSE で本物の部品が届いたら([LiveDraft])届いた順に下書きと置き換える(仮置き)。
 */
object BuildChoreography {
    /** 部品のシルエットの種類。DSL の [ComponentType] を見た目の近いものにまとめる。 */
    enum class PartKind {
        HEADING, TEXT, BUTTON, FIELD, TOGGLE, LIST, CHART, STAT, CALENDAR, MAP, MEDIA, CARD;

        companion object {
            fun of(type: ComponentType): PartKind = when (type) {
                ComponentType.TEXT, ComponentType.MARKDOWN, ComponentType.LABEL, ComponentType.BADGE, ComponentType.LINK -> TEXT
                ComponentType.BUTTON, ComponentType.OPEN_URL_BUTTON, ComponentType.SHARE_BUTTON, ComponentType.LOCATION_BUTTON,
                ComponentType.AI_BUTTON, ComponentType.RANDOM_PICKER, ComponentType.NEARBY_SHARE, ComponentType.SPEAK_BUTTON,
                -> BUTTON
                ComponentType.TEXT_FIELD, ComponentType.NUMBER_FIELD, ComponentType.DATE_PICKER, ComponentType.PICKER,
                ComponentType.TEXT_EDITOR, ComponentType.SECURE_FIELD, ComponentType.TIME_PICKER, ComponentType.COLOR_PICKER,
                ComponentType.SLIDER, ComponentType.STEPPER, ComponentType.RATING, ComponentType.SEARCH_BAR,
                ComponentType.LOCATION_PICKER, ComponentType.VOICE_INPUT, ComponentType.TRANSCRIBE_FIELD, ComponentType.FILE_IMPORT,
                -> FIELD
                ComponentType.TOGGLE -> TOGGLE
                ComponentType.LIST, ComponentType.TABLE, ComponentType.TIMELINE, ComponentType.GRID_TABLE, ComponentType.MATCH_LIST,
                ComponentType.CHAT_THREAD, ComponentType.AI_CHAT, ComponentType.SETTLEMENT, ComponentType.FEED, ComponentType.BRACKET,
                -> LIST
                ComponentType.CHART -> CHART
                ComponentType.STAT, ComponentType.GAUGE, ComponentType.PROGRESS_BAR, ComponentType.HEALTH_STAT,
                ComponentType.SENSOR_VALUE, ComponentType.TIMER_DISPLAY,
                -> STAT
                ComponentType.CALENDAR -> CALENDAR
                ComponentType.MAP_VIEW, ComponentType.ROUTE_GUIDE, ComponentType.PLACE_SEARCH, ComponentType.LOCATION_TRACKER -> MAP
                ComponentType.PHOTO_FIELD, ComponentType.DRAWING_FIELD, ComponentType.IMAGE, ComponentType.VIDEO_FIELD,
                ComponentType.VIDEO_PLAYER, ComponentType.AUDIO_RECORDER, ComponentType.QR_CODE, ComponentType.QR_SCANNER,
                ComponentType.DASHCAM, ComponentType.MUSIC_PLAYER, ComponentType.AVATAR, ComponentType.ICON, ComponentType.WEB_VIEW,
                -> MEDIA
                else -> CARD
            }
        }
    }

    // MARK: - 依頼文からの当て推量

    private val hintTable: List<Pair<List<String>, PartKind>> = listOf(
        listOf("グラフ", "推移", "チャート", "graph", "chart") to PartKind.CHART,
        listOf("チェック", "やること", "TODO", "完了", "check") to PartKind.TOGGLE,
        listOf("一覧", "リスト", "履歴", "list") to PartKind.LIST,
        listOf("記録", "入力", "メモ", "日記", "record", "log") to PartKind.FIELD,
        listOf("合計", "集計", "平均", "残り", "回数", "total") to PartKind.STAT,
        listOf("カレンダー", "予定", "スケジュール", "calendar") to PartKind.CALENDAR,
        listOf("地図", "場所", "散歩", "ルート", "map") to PartKind.MAP,
        listOf("写真", "画像", "カメラ", "photo") to PartKind.MEDIA,
    )

    /** 依頼文の語から「出てきそうな部品」を最大 3 つ(出てきた順)。当てずっぽうなので控えめにする。 */
    fun hints(transcript: String): List<PartKind> = hintTable
        .mapNotNull { (words, kind) ->
            words.mapNotNull { w -> transcript.indexOf(w, ignoreCase = true).takeIf { it >= 0 } }.minOrNull()?.let { it to kind }
        }
        .sortedBy { it.first }
        .map { it.second }
        .take(3)

    /** パーツ箱の中身(6 個)。当て推量を先頭に、残りはよく使う部品で埋める。 */
    fun tray(hints: List<PartKind>): List<PartKind> =
        (hints + listOf(PartKind.HEADING, PartKind.FIELD, PartKind.BUTTON, PartKind.LIST, PartKind.STAT, PartKind.TOGGLE, PartKind.CHART))
            .distinct()
            .take(6)

    // MARK: - 待っている間

    /** 画面に置かれた 1 つの部品。[id] は置いた時に決まり、入れ替え・並べ直しの間も変わらない(動きのアニメ用)。 */
    data class PlacedPart(val id: String, val kind: PartKind, val area: GridArea, val caption: String? = null)

    enum class Move { PLACE, SWAP, REORDER, RELAYOUT }

    /** 待っている間の 1 コマ。[picking] は今パーツ箱から選んでいる部品(箱の中で持ち上がる)。 */
    data class WaitingFrame(
        val placed: List<PlacedPart>,
        val picking: PartKind?,
        val move: Move,
        /** 画面の縦の行数(仮置きが 12 行を超えたら伸ばす) */
        val rows: Int = GridArea.VISIBLE_ROWS,
    )

    /** 待っている間の台本。step ごとに 1 手進む(置く → 入れ替える → 並べ直す → 配置を変える…)。 */
    class WaitingScript(transcript: String) {
        val tray: List<PartKind> = tray(hints(transcript))

        /** 仮置き([LiveDraft])を混ぜたコマ。本物の部品を置き、重ならない所だけ下書きを残す(合わせて 5 枠まで)。 */
        fun frame(step: Int, live: LiveDraft?): WaitingFrame {
            val guess = frame(step)
            val real = live?.parts.orEmpty()
            if (real.isEmpty()) return guess
            val rest = guess.placed.filter { g -> real.none { it.area.overlaps(g.area) } }
            val keep = maxOf(0, LAYOUTS[0].size - real.size)
            return WaitingFrame(
                placed = real + rest.take(keep),
                picking = live?.latestKind ?: guess.picking,
                move = Move.PLACE,
                rows = maxOf(GridArea.VISIBLE_ROWS, real.maxOf { it.area.row2 }),
            )
        }

        /** step 手目のコマ。0 手目から順に再生して決める(決定的・数十手なので軽い)。 */
        fun frame(step: Int): WaitingFrame {
            val slots = LAYOUTS[0].mapNotNull(GridArea::parse)
            val placed = mutableListOf<PlacedPart>()
            var serial = 0
            var nextTray = 0
            fun takeFromTray(): PartKind = tray[nextTray++ % tray.size]
            var current = WaitingFrame(emptyList(), null, Move.PLACE)
            var layout = 0
            for (s in 0..maxOf(0, step)) {
                if (placed.size < slots.size) {
                    // 置く: 空いている枠に上から
                    val kind = takeFromTray()
                    placed += PlacedPart("p${serial++}", kind, slots[placed.size])
                    current = WaitingFrame(placed.toList(), kind, Move.PLACE)
                    continue
                }
                val k = s - slots.size
                current = when (k % 3) {
                    0 -> {
                        // 入れ替える: 1 枠を別の部品に(同じ種類なら次の部品)
                        val i = (k / 3 + 1) % placed.size
                        var kind = takeFromTray()
                        if (kind == placed[i].kind) kind = takeFromTray()
                        placed[i] = PlacedPart("p${serial++}", kind, placed[i].area)
                        WaitingFrame(placed.toList(), kind, Move.SWAP)
                    }
                    1 -> {
                        // 並べ直す: 隣り合う 2 つの場所を交換
                        val i = (k / 3 + 2) % placed.size
                        val j = (i + 1) % placed.size
                        val a = placed[i].area
                        placed[i] = placed[i].copy(area = placed[j].area)
                        placed[j] = placed[j].copy(area = a)
                        WaitingFrame(placed.toList(), null, Move.REORDER)
                    }
                    else -> {
                        // 配置を変える: 同じ部品のまま別の配置案へ
                        layout = (layout + 1) % LAYOUTS.size
                        val areas = LAYOUTS[layout].mapNotNull(GridArea::parse)
                        val order = placed.sortedWith(compareBy({ it.area.row1 }, { it.area.col1 }))
                        placed.clear()
                        order.forEachIndexed { n, p -> placed += p.copy(area = areas[n % areas.size]) }
                        WaitingFrame(placed.toList(), null, Move.RELAYOUT)
                    }
                }
            }
            return current
        }

        companion object {
            /** 下書きの配置案(実 DSL と同じ area 記法・上から順) */
            val LAYOUTS: List<List<String>> = listOf(
                listOf("A1:F2", "A3:F6", "A7:C9", "D7:F9", "A10:F12"),
                listOf("A1:F2", "A3:C6", "D3:F6", "A7:F10", "A11:F12"),
                listOf("A1:F3", "A4:F5", "A6:F9", "A10:C12", "D10:F12"),
            )

            /** 1 手の秒数(動きを減らす設定ではゆっくり・フェードだけ) */
            fun stepSeconds(reduceMotion: Boolean): Double = if (reduceMotion) 2.4 else 0.85

            fun step(elapsedSeconds: Double, reduceMotion: Boolean): Int =
                maxOf(0, (elapsedSeconds / stepSeconds(reduceMotion)).toInt())
        }
    }

    // MARK: - 結果が届いた時

    /** 結果の部品を置いていく段取り。 */
    data class AssemblyPlan(
        val steps: List<Step>,
        /** 演出の全体の秒数(この後に本物の画面へ切り替える) */
        val total: Double,
        /** 画面の縦の行数(12 未満にしない。積むだけの部品は下に足す) */
        val rows: Int,
        /** パーツ箱に出す種類(結果に出てくる順・最大 6) */
        val tray: List<PartKind>,
        /** タブの見出し(タブが 2 つ以上の時だけ・画面の下に出す) */
        val tabs: List<String> = emptyList(),
    ) {
        /** [delay] は演出の開始から置くまでの秒数 */
        data class Step(val part: PlacedPart, val delay: Double)

        companion object {
            /** 置く部品の上限。超えた分は最後の手でまとめて置く */
            const val MAX_STEPS = 10
            const val LEAD_IN = 0.12
            const val HOLD = 0.3

            /** 1 手の秒数の上限・置く時間の合計の上限(全体は leadIn + 0.9 + hold ≒ 1.3 秒以内) */
            const val MAX_PER_STEP = 0.16
            const val MAX_PLACING = 0.9

            /** 動きを減らす設定: 全部を一度にフェードで出す */
            const val REDUCED_TOTAL = 0.45

            /** [shown]: 仮置きで既に見せた部品。結果と同じ部品(id・種類・場所が一致)は最初から置いておき、残りだけ 1 つずつ置く。 */
            fun make(dsl: AppDSL, reduceMotion: Boolean, shown: List<PlacedPart> = emptyList()): AssemblyPlan {
                val parts = initialParts(dsl)
                val tray = parts.map { it.kind }.distinct().take(6)
                val rows = maxOf(GridArea.VISIBLE_ROWS, parts.maxOfOrNull { it.area.row2 } ?: 0)
                val tabs = if (dsl.app.hasTabs) dsl.app.tabs.map { it.title } else emptyList()
                if (reduceMotion || parts.isEmpty()) {
                    return AssemblyPlan(parts.map { Step(it, 0.0) }, if (reduceMotion) REDUCED_TOTAL else LEAD_IN + HOLD, rows, tray, tabs)
                }
                val kept = parts.filter { p -> shown.any { it.id == p.id && it.kind == p.kind && it.area == p.area } }
                val rest = parts.filter { it !in kept }
                if (rest.isEmpty()) return AssemblyPlan(kept.map { Step(it, 0.0) }, LEAD_IN + HOLD, rows, tray, tabs)
                val slots = minOf(rest.size, MAX_STEPS)
                val per = minOf(MAX_PER_STEP, MAX_PLACING / slots)
                val steps = kept.map { Step(it, 0.0) } +
                    rest.mapIndexed { i, p -> Step(p, LEAD_IN + minOf(i, slots - 1) * per) }
                return AssemblyPlan(steps, LEAD_IN + slots * per + HOLD, rows, tray, tabs)
            }

            /** 最初に見える画面(タブがあれば初期タブ)の直下の部品を、上から(行 → 列)の順に。area の無い部品は下に全幅で積む。 */
            fun initialParts(dsl: AppDSL): List<PlacedPart> {
                val sid = if (dsl.app.hasTabs) dsl.initialTab else dsl.initialScreen
                val screen = dsl.screens.firstOrNull { it.id == sid } ?: dsl.screens.firstOrNull() ?: return emptyList()
                val placement = screen.gridPlacement
                val out = placement.placed
                    .sortedWith(compareBy({ it.area.row1 }, { it.area.col1 }))
                    .mapIndexed { i, p -> PlacedPart(p.component.id ?: "c$i", kind(p.component, p.area), p.area, caption(p.component)) }
                    .toMutableList()
                var row = placement.lastRow + 1
                placement.stacked.forEachIndexed { i, c ->
                    val height = if (c.type == ComponentType.LIST || c.type == ComponentType.CHART) 4 else 2
                    GridArea.parse("A$row:F${row + height - 1}")?.let { out += PlacedPart(c.id ?: "s$i", kind(c, null), it, caption(c)) }
                    row += height
                }
                return out
            }

            /** 見出しらしい text(1 行の高さ・style が title / headline)は heading にする。 */
            private fun kind(c: Component, area: GridArea?): PartKind {
                if (c.type == ComponentType.TEXT &&
                    (c.style == ComponentStyle.TITLE || c.style == ComponentStyle.HEADLINE || (area?.rowSpan ?: 3) <= 1)
                ) {
                    return PartKind.HEADING
                }
                return PartKind.of(c.type)
            }

            /** シルエットに添える短い文字(本物の結果の部品の名前)。長い文は出さない。 */
            fun caption(c: Component): String? = shortCaption(listOf(c.label, c.title, if (c.type == ComponentType.TEXT) c.text else null))

            fun shortCaption(candidates: List<String?>): String? {
                val raw = candidates.mapNotNull { it?.trim() }.firstOrNull { it.isNotEmpty() } ?: return null
                return if (raw.length > 12) raw.take(11) + "…" else raw
            }
        }
    }

    // MARK: - 届いた本物の部品(仮置き)

    /**
     * SSE で届いた途中経過([BuildProgress])の見え方。どれも仮置きで、最後の結果が正。
     * やり直し(restart)で部品を捨てるのは [jp.kz2601.metamon.voice.VoiceViewModel] が済ませている。
     */
    class LiveDraft(progress: BuildProgress) {
        val appName: String? = progress.appName
        val tabs: List<String> = progress.tabs

        /** 届いた部品(同じ index は 1 回だけ) */
        val components: List<StreamedComponent> = progress.components.distinctBy { it.index }

        /** 届いた部品のシルエット。area のある部品はその場所、無い部品はその下に全幅で積む([AssemblyPlan.initialParts] と同じ)。 */
        val parts: List<PlacedPart> by lazy {
            val out = mutableListOf<PlacedPart>()
            val stacked = mutableListOf<StreamedComponent>()
            for (c in components.sortedBy { it.index }) {
                val area = GridArea.parse(c.area)
                if (area != null) out += part(c, area) else stacked += c
            }
            var row = (out.maxOfOrNull { it.area.row2 } ?: 0) + 1
            for (c in stacked) {
                val type = typeOf(c)
                val height = if (type == ComponentType.LIST || type == ComponentType.CHART) 4 else 2
                GridArea.parse("A$row:F${row + height - 1}")?.let { out += part(c, it) }
                row += height
            }
            // 同じ id の部品が来ても描画の id は重ねない
            val seen = mutableSetOf<String>()
            out.mapIndexed { i, p -> if (seen.add(p.id)) p else p.copy(id = "stream-dup-$i") }
        }

        /** 最後に届いた部品の種類(パーツ箱で持ち上げる) */
        val latestKind: PartKind? get() = components.lastOrNull()?.let { kind(it, GridArea.parse(it.area)) }

        /** パーツ箱: 届いた部品の種類を先頭に、残りは下書きの箱で埋める */
        fun tray(base: List<PartKind>): List<PartKind> =
            (components.map { kind(it, GridArea.parse(it.area)) } + base).distinct().take(6)

        private fun part(c: StreamedComponent, area: GridArea) = PlacedPart(
            id = c.id?.takeIf { it.isNotEmpty() } ?: "stream-${c.index}",
            kind = kind(c, area),
            area = area,
            caption = AssemblyPlan.shortCaption(listOf(c.label)),
        )

        private fun typeOf(c: StreamedComponent): ComponentType? = ComponentType.entries.firstOrNull { it.wire == c.type && it != ComponentType.UNKNOWN }

        /** 見出しらしい text(1 行の高さ)は heading。style は届かないので高さだけで見る */
        private fun kind(c: StreamedComponent, area: GridArea?): PartKind {
            val type = typeOf(c) ?: return PartKind.CARD
            if (type == ComponentType.TEXT && (area?.rowSpan ?: 3) <= 1) return PartKind.HEADING
            return PartKind.of(type)
        }
    }
}
