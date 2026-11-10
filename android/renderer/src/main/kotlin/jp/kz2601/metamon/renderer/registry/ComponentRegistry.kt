package jp.kz2601.metamon.renderer.registry

import androidx.compose.runtime.Composable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
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
import jp.kz2601.metamon.renderer.navigation.Route

/** 描画中の文脈。どの画面(route)か・能力(runtime)・アクションの実行口。 */
class RenderContext(
    val route: Route,
    val runtime: DSLRuntime = NoopDSLRuntime,
    /** button などが押された時。遷移は描画器が、それ以外は runtime が受ける。 */
    val perform: (Action) -> Unit = {},
)

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
            // leaf: 表示(11・#857)
            register(ComponentType.TEXT) { c, _, m -> TextLeaf(c, m) }
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
            // leaf: その他
            register(ComponentType.BUTTON) { c, ctx, m -> ButtonLeaf(c, ctx, m) }
            register(ComponentType.VOICE_INPUT) { c, ctx, m -> VoiceInputLeaf(c, ctx, m) }
        }
    }
}

/** 描画に使うレジストリ。既定は [ComponentRegistry.standard]。 */
val LocalComponentRegistry = staticCompositionLocalOf { ComponentRegistry.standard }
