package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.hasTabs
import jp.kz2601.metamon.renderer.navigation.NavigationState
import jp.kz2601.metamon.renderer.navigation.Route
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/**
 * プロセスが落とされた後に戻す画面の位置(#891)。どのアプリのどの画面かだけを持つ(記録・下書きは持たない)。
 * Activity の保存状態に文字列で入れ、[AppRuntime.offerNavigationRestore] が起動の後に当てる。
 * 今の DSL に無い画面・タブを指していたら [restoreIn] は null(アプリを開いた直後の画面に戻す)。
 */
data class NavigationSnapshot(val appId: String, val navigation: NavigationState) {
    fun encode(): String = JsonObject(
        mapOf(
            "appId" to JsonPrimitive(appId),
            "selectedTab" to JsonPrimitive(navigation.selectedTab),
            "paths" to JsonObject(navigation.paths.mapValues { (_, routes) -> JsonArray(routes.map(::route)) }),
            "sheet" to (navigation.sheet?.let(::route) ?: JsonNull),
            "fullScreen" to (navigation.fullScreen?.let(::route) ?: JsonNull),
        ),
    ).toString()

    /** [dsl] の画面だけでできていれば、その遷移。 */
    fun restoreIn(appId: String, dsl: AppDSL): NavigationState? {
        if (appId != this.appId) return null
        val screens = dsl.screens.map { it.id }.toSet()
        val routes = navigation.paths.values.flatten() + listOfNotNull(navigation.sheet, navigation.fullScreen)
        if (routes.any { it.screen !in screens }) return null
        val tabs = dsl.app.tabs.map { it.screen }
        if (dsl.app.hasTabs) {
            if (navigation.selectedTab !in tabs) return null
        } else if (navigation.selectedTab.isNotEmpty()) {
            return null
        }
        val stacks = setOf(Route.MAIN, Route.SHEET, Route.FULL) + tabs.map(Route::tabStack)
        if (navigation.paths.keys.any { it !in stacks }) return null
        return navigation
    }

    companion object {
        /** 壊れた・古い形の文字列は null(戻さずに初めの画面)。 */
        fun decode(text: String?): NavigationSnapshot? = text?.let {
            runCatching {
                val o = Json.parseToJsonElement(it).jsonObject
                NavigationSnapshot(
                    appId = o.getValue("appId").jsonPrimitive.content,
                    navigation = NavigationState(
                        paths = o["paths"]?.jsonObject.orEmpty().mapValues { (_, v) -> v.jsonArray.map { r -> route(r.jsonObject) } },
                        selectedTab = o["selectedTab"]?.jsonPrimitive?.content.orEmpty(),
                        sheet = (o["sheet"] as? JsonObject)?.let(::route),
                        fullScreen = (o["fullScreen"] as? JsonObject)?.let(::route),
                    ),
                )
            }.getOrNull()
        }

        private fun route(r: Route) = JsonObject(
            mapOf(
                "screen" to JsonPrimitive(r.screen),
                "recordId" to JsonPrimitive(r.recordId),
                "stack" to JsonPrimitive(r.stack),
            ),
        )

        private fun route(o: JsonObject) = Route(
            screen = o.getValue("screen").jsonPrimitive.content,
            recordId = o["recordId"]?.jsonPrimitive?.contentOrNull,
            stack = o["stack"]?.jsonPrimitive?.contentOrNull ?: Route.MAIN,
        )
    }
}
