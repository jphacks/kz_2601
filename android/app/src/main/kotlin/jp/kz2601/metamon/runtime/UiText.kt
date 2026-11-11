package jp.kz2601.metamon.runtime

import android.content.res.Resources
import androidx.annotation.PluralsRes
import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.platform.LocalResources

/** 画面で解決する文言。ランタイムは Context を持たないので、文字列の代わりに ID と引数を渡す。 */
sealed interface UiText {
    /** [args] は String・数・[UiText](先に解決する)。 */
    data class Res(@StringRes val id: Int, val args: List<Any> = emptyList()) : UiText {
        constructor(@StringRes id: Int, vararg args: Any) : this(id, args.toList())
    }

    /** 数で形が変わる文(英語の単数・複数)。[count] で形を選び、[args] で埋める(数も args に入れる)。 */
    data class Plural(@PluralsRes val id: Int, val count: Int, val args: List<Any>) : UiText

    /** そのまま出す文字(DSL の名前など)。 */
    data class Raw(val text: String) : UiText

    fun resolve(res: Resources): String = when (this) {
        is Raw -> text
        is Res -> res.getString(id, *args.map { if (it is UiText) it.resolve(res) else it }.toTypedArray())
        is Plural -> res.getQuantityString(id, count, *args.map { if (it is UiText) it.resolve(res) else it }.toTypedArray())
    }
}

@Composable
fun UiText.resolve(): String = resolve(LocalResources.current)
