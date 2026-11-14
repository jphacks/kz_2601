package jp.kz2601.metamon.renderer.components.route

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** 道案内の Leaf(#868)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerRouteLeaves() {
    register(ComponentType.ROUTE_GUIDE) { c, ctx, m -> RouteGuideLeaf(c, ctx, m) }
}
