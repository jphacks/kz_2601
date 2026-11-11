package jp.kz2601.metamon.desktop

import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.remember
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Window
import androidx.compose.ui.window.application
import androidx.compose.ui.window.rememberWindowState

/** 起動。引数(または -Dtailorit.example)で同梱の例を選ぶ。既定は shopping。サインインはまだ無い(モック)。 */
fun main(args: Array<String>) = application {
    val name = args.firstOrNull() ?: System.getProperty("tailorit.example") ?: "shopping"
    val dsl = remember { Examples.load(name) }
    Window(
        onCloseRequest = ::exitApplication,
        title = "Tailor It",
        state = rememberWindowState(width = 480.dp, height = 820.dp),
    ) {
        MaterialTheme { DesktopApp(dsl) }
    }
}
