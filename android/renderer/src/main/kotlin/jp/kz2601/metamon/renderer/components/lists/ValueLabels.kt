package jp.kz2601.metamon.renderer.components.lists

import android.content.res.Resources
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import jp.kz2601.metamon.renderer.R

/**
 * 一覧・集計・グラフの値に入る文言(#843)。画面は端末の言語([rememberValueLabels])、
 * 適合テストの描画の木は iOS の日本語の元文([JAPANESE])を渡す。書式は String.format(リソースは位置指定 `%1$s`)。
 */
data class ValueLabels(
    val empty: String,
    val count: String,
    val sum: String,
    val avg: String,
    val min: String,
    val max: String,
    val countUnit: String,
    val noData: String,
    val chartOne: String,
    val chartMany: String,
    val everyone: String,
    val member: String,
) {
    companion object {
        /** iOS の日本語の元文(適合テストの期待値と同じ)。文言の正は strings_lists.xml。 */
        val JAPANESE = ValueLabels(
            empty = "まだありません",
            count = "件数",
            sum = "%sの合計",
            avg = "%sの平均",
            min = "%sの最小",
            max = "%sの最大",
            countUnit = "件",
            noData = "データがありません",
            chartOne = "%d 項目。%s の %s",
            chartMany = "%d 項目。最大は %s の %s、最小は %s の %s",
            everyone = "全員",
            member = "参加者",
        )

        fun from(res: Resources) = ValueLabels(
            empty = res.getString(R.string.lists_empty),
            count = res.getString(R.string.lists_count),
            sum = res.getString(R.string.lists_sum),
            avg = res.getString(R.string.lists_avg),
            min = res.getString(R.string.lists_min),
            max = res.getString(R.string.lists_max),
            countUnit = res.getString(R.string.lists_count_unit),
            noData = res.getString(R.string.lists_no_data),
            chartOne = res.getString(R.string.lists_chart_summary_one),
            chartMany = res.getString(R.string.lists_chart_summary_many),
            everyone = res.getString(R.string.lists_everyone),
            member = res.getString(R.string.lists_member),
        )
    }
}

/** 端末(アプリ)の言語の [ValueLabels]。言語が変わると作り直す。 */
@Composable
fun rememberValueLabels(): ValueLabels {
    val context = LocalContext.current
    val config = LocalConfiguration.current
    return remember(context, config) { ValueLabels.from(context.resources) }
}
