package jp.kz2601.metamon.renderer.registry

import androidx.compose.runtime.Composable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.NoopDSLRuntime
import jp.kz2601.metamon.renderer.components.ButtonLeaf
import jp.kz2601.metamon.renderer.components.AvatarLeaf
import jp.kz2601.metamon.renderer.components.BadgeLeaf
import jp.kz2601.metamon.renderer.components.EmptyStateLeaf
import jp.kz2601.metamon.renderer.components.IconLeaf
import jp.kz2601.metamon.renderer.components.ImageLeaf
import jp.kz2601.metamon.renderer.components.LabelLeaf
import jp.kz2601.metamon.renderer.components.LinkLeaf
import jp.kz2601.metamon.renderer.components.MarkdownLeaf
import jp.kz2601.metamon.renderer.components.CardContainer
import jp.kz2601.metamon.renderer.components.CarouselContainer
import jp.kz2601.metamon.renderer.components.DisclosureContainer
import jp.kz2601.metamon.renderer.components.DividerLeaf
import jp.kz2601.metamon.renderer.components.GridContainer
import jp.kz2601.metamon.renderer.components.HStackContainer
import jp.kz2601.metamon.renderer.components.SectionContainer
import jp.kz2601.metamon.renderer.components.SpacerLeaf
import jp.kz2601.metamon.renderer.components.TextLeaf
import jp.kz2601.metamon.renderer.components.VStackContainer
import jp.kz2601.metamon.renderer.components.VoiceInputLeaf
import jp.kz2601.metamon.renderer.components.actions.registerActionLeaves
import jp.kz2601.metamon.renderer.components.ai.registerAiChatLeaves
import jp.kz2601.metamon.renderer.components.ai.registerAiLeaves
import jp.kz2601.metamon.renderer.components.audio.registerAudioLeaves
import jp.kz2601.metamon.renderer.components.dashcam.registerDashcamLeaves
import jp.kz2601.metamon.renderer.components.feed.registerFeedLeaves
import jp.kz2601.metamon.renderer.components.health.registerHealthLeaves
import jp.kz2601.metamon.renderer.components.importing.registerImportLeaves
import jp.kz2601.metamon.renderer.components.inputs.registerInputLeaves
import jp.kz2601.metamon.renderer.components.lists.registerListLeaves
import jp.kz2601.metamon.renderer.components.play.registerPlayLeaves
import jp.kz2601.metamon.renderer.components.location.registerLocationLeaves
import jp.kz2601.metamon.renderer.components.map.registerMapLeaves
import jp.kz2601.metamon.renderer.components.route.registerRouteLeaves
import jp.kz2601.metamon.renderer.components.media.registerMediaLeaves
import jp.kz2601.metamon.renderer.components.music.registerMusicLeaves
import jp.kz2601.metamon.renderer.components.nearby.registerNearbyLeaves
import jp.kz2601.metamon.renderer.components.scan.registerScanLeaves
import jp.kz2601.metamon.renderer.components.sensors.registerSensorLeaves
import jp.kz2601.metamon.renderer.components.social.registerSocialLeaves
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.navigation.Route

/** 描画中の文脈。どの画面(route)か・能力(runtime)・その画面のフォームの下書き・アクションの実行口。 */
class RenderContext(
    val route: Route,
    val runtime: DSLRuntime = NoopDSLRuntime,
    /** `screen.entity` を持つ画面の下書き(入力部品が書く)。無ければ入力部品は「フォームの外」の枠になる。 */
    val form: FormDraft? = null,
    /** 描いている画面(aiButton の inputFeeds が同じ画面の feed を探す)。無ければ null。 */
    val screen: Screen? = null,
    /** 押した場所つきのアクション。遷移は描画器が、それ以外は runtime が受ける。一覧の行タップは recordId 付きで呼ぶ。 */
    val dispatch: (Action, ActionContext) -> Unit = { _, _ -> },
) {
    /** button などが押された時(この画面・このフォームから)。 */
    val perform: (Action) -> Unit = { dispatch(it, ActionContext(route, form)) }

    fun withForm(form: FormDraft?): RenderContext = RenderContext(route, runtime, form, screen, dispatch)

    fun withScreen(screen: Screen?): RenderContext = RenderContext(route, runtime, form, screen, dispatch)
}

typealias ComponentBuilder = @Composable (component: Component, ctx: RenderContext, modifier: Modifier) -> Unit

/**
 * schema の `type` 文字列 → Composable の辞書(iOS の `ComponentRegistry`)。`DSLComponent` はここを引くだけで、when を持たない。
 *
 * 部品の追加手順:
 * 1. `components/` に `@Composable fun FooLeaf(c, ctx, modifier)` を作る
 * 2. 下の [standard] に `register(ComponentType.FOO) { c, ctx, m -> FooLeaf(c, ctx, m) }` を 1 行足す
 * 3. 適合テスト(`ConformanceTest`)で skip が減ったのを確かめる
 */
class ComponentRegistry private constructor(private val builders: Map<String, ComponentBuilder>) {
    fun builder(type: ComponentType): ComponentBuilder? = builders[type.wire]

    val registeredTypes: Set<String> get() = builders.keys

    /** 1 件足した(上書きした)コピー。アプリ本体が voiceInput を差し替える時などに使う。 */
    fun registering(type: ComponentType, build: ComponentBuilder): ComponentRegistry =
        ComponentRegistry(builders + (type.wire to build))

    class Builder {
        private val builders = linkedMapOf<String, ComponentBuilder>()
        fun register(type: ComponentType, build: ComponentBuilder) {
            require(type != ComponentType.UNKNOWN) { "UNKNOWN は登録できない" }
            builders[type.wire] = build
        }
        fun build() = ComponentRegistry(builders.toMap())
    }

    companion object {
        fun build(block: Builder.() -> Unit): ComponentRegistry = Builder().apply(block).build()

        /** :renderer 同梱の部品。まだ無い部品は「この端末では使えません」の枠になる(#357 の子 Issue で足す)。 */
        val standard: ComponentRegistry = build {
            // container(7)
            register(ComponentType.VSTACK) { c, ctx, m -> VStackContainer(c, ctx, m) }
            register(ComponentType.HSTACK) { c, ctx, m -> HStackContainer(c, ctx, m) }
            register(ComponentType.SECTION) { c, ctx, m -> SectionContainer(c, ctx, m) }
            register(ComponentType.CARD) { c, ctx, m -> CardContainer(c, ctx, m) }
            register(ComponentType.DISCLOSURE) { c, ctx, m -> DisclosureContainer(c, ctx, m) }
            register(ComponentType.CAROUSEL) { c, ctx, m -> CarouselContainer(c, ctx, m) }
            register(ComponentType.GRID) { c, ctx, m -> GridContainer(c, ctx, m) }
            registerAudioLeaves() // leaf: 音(3・#864)・components/audio/AudioLeaves.kt
            // input(12)・登録は components/inputs/InputLeaves.kt
            registerInputLeaves()
            // leaf: 一覧・集計・グラフ(10・#859)・登録は components/lists/ListLeaves.kt
            registerListLeaves()
            registerPlayLeaves() // leaf: 計算・遊び(5・#860)・components/play/PlayLeaves.kt
            // 写真・手書き(#862)・登録は components/media/MediaLeaves.kt
            registerMediaLeaves()
            registerLocationLeaves() // leaf: 現在地(#867)・components/location/LocationLeaves.kt
            registerMapLeaves() // leaf: 地図(#867)・components/map/MapLeaves.kt
            registerRouteLeaves() // leaf: 道案内(#868)・components/route/RouteLeaves.kt
            registerScanLeaves() // leaf: カメラで読む(#866)・components/scan/ScanLeaves.kt
            registerDashcamLeaves() // leaf: ドラレコ(#872)・components/dashcam/DashcamLeaves.kt
            registerSensorLeaves() // leaf: センサー(#871)・components/sensors/SensorLeaves.kt
            registerHealthLeaves() // leaf: ヘルス(#870)・components/health/HealthLeaves.kt
            registerMusicLeaves() // leaf: 音楽の代わり(#879)・components/music/MusicLeaves.kt
            registerNearbyLeaves() // leaf: 近くの端末と送り合う(#1382)・components/nearby/NearbyLeaves.kt
            registerFeedLeaves() // leaf: 外部の公開データ(#876)・components/feed/FeedLeaves.kt
            registerImportLeaves() // leaf: ファイルの取り込み(#865)・components/importing/ImportLeaves.kt
            registerSocialLeaves() // leaf: みんなで話す(#875)・components/social/SocialLeaves.kt
            // leaf: 表示(11・#857)
            register(ComponentType.TEXT) { c, ctx, m -> TextLeaf(c, ctx, m) }
            register(ComponentType.LABEL) { c, _, m -> LabelLeaf(c, m) }
            register(ComponentType.ICON) { c, _, m -> IconLeaf(c, m) }
            register(ComponentType.MARKDOWN) { c, _, m -> MarkdownLeaf(c, m) }
            register(ComponentType.BADGE) { c, _, m -> BadgeLeaf(c, m) }
            register(ComponentType.AVATAR) { c, ctx, m -> AvatarLeaf(c, ctx, m) }
            register(ComponentType.IMAGE) { c, ctx, m -> ImageLeaf(c, ctx, m) }
            register(ComponentType.DIVIDER) { _, _, m -> DividerLeaf(m) }
            register(ComponentType.SPACER) { c, _, m -> SpacerLeaf(c, m) }
            register(ComponentType.LINK) { c, _, m -> LinkLeaf(c, m) }
            register(ComponentType.EMPTY_STATE) { c, _, m -> EmptyStateLeaf(c, m) }
            // leaf: AI・登録は components/ai/AiLeaves.kt
            registerAiLeaves()
            registerAiChatLeaves() // AI と話す(#874)・components/ai/AiChatLeaves.kt
            // leaf: その他
            registerActionLeaves() // 外へ出る 4 種(#861)・components/actions/ActionLeaves.kt
            register(ComponentType.BUTTON) { c, ctx, m -> ButtonLeaf(c, ctx, m) }
            register(ComponentType.VOICE_INPUT) { c, ctx, m -> VoiceInputLeaf(c, ctx, m) }
        }
    }
}

/** 描画に使うレジストリ。既定は [ComponentRegistry.standard]。 */
val LocalComponentRegistry = staticCompositionLocalOf { ComponentRegistry.standard }
