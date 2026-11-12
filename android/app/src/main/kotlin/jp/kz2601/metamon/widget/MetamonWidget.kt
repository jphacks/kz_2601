package jp.kz2601.metamon.widget

import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.compose.runtime.Composable
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.glance.GlanceId
import androidx.glance.GlanceModifier
import androidx.glance.GlanceTheme
import androidx.glance.LocalContext
import androidx.glance.LocalSize
import androidx.glance.action.clickable
import androidx.glance.appwidget.GlanceAppWidget
import androidx.glance.appwidget.GlanceAppWidgetManager
import androidx.glance.appwidget.GlanceAppWidgetReceiver
import androidx.glance.appwidget.SizeMode
import androidx.glance.appwidget.action.actionStartActivity
import androidx.glance.appwidget.cornerRadius
import androidx.glance.appwidget.provideContent
import androidx.glance.appwidget.updateAll
import androidx.glance.background
import androidx.glance.layout.Alignment
import androidx.glance.layout.Box
import androidx.glance.layout.Column
import androidx.glance.layout.Row
import androidx.glance.layout.Spacer
import androidx.glance.layout.fillMaxSize
import androidx.glance.layout.fillMaxWidth
import androidx.glance.layout.height
import androidx.glance.layout.padding
import androidx.glance.layout.width
import androidx.glance.semantics.contentDescription
import androidx.glance.semantics.semantics
import androidx.glance.text.FontWeight
import androidx.glance.text.Text
import androidx.glance.text.TextStyle
import jp.kz2601.metamon.MainActivity
import jp.kz2601.metamon.R
import jp.kz2601.metamon.ReviewBuild
import java.time.LocalDate

/**
 * ホーム画面のウィジェット(#912・iOS の `MetamonStatWidget`)。本体が書いた [WidgetSnapshot] を描くだけ(DSL・記録は読まない)。
 * 大きさは iOS の small / medium / large に合わせて 3 段階。押すとそのアプリを開く。
 * iOS のロック画面の accessory は、Android 12 以降のロック画面にウィジェットが無いので持たない。
 */
class MetamonWidget : GlanceAppWidget() {
    override val sizeMode: SizeMode = SizeMode.Responsive(setOf(SMALL, MEDIUM, LARGE))

    override suspend fun provideGlance(context: Context, id: GlanceId) {
        val snapshot = WidgetSnapshotStore.of(context).load()
        provideContent { GlanceTheme { WidgetContent(snapshot) } }
    }

    companion object {
        val SMALL = DpSize(110.dp, 110.dp)
        val MEDIUM = DpSize(250.dp, 110.dp)
        val LARGE = DpSize(250.dp, 260.dp)

        /** 置かれているウィジェットを描き直す(本体が中身を書いた・消した時)。置かれていなければ何もしない。 */
        suspend fun refresh(context: Context) {
            if (GlanceAppWidgetManager(context).getGlanceIds(MetamonWidget::class.java).isEmpty()) return
            MetamonWidget().updateAll(context)
        }
    }
}

class MetamonWidgetReceiver : GlanceAppWidgetReceiver() {
    override val glanceAppWidget: GlanceAppWidget = MetamonWidget()
}

private enum class Family { SMALL, MEDIUM, LARGE }

@Composable
private fun WidgetContent(snapshot: WidgetSnapshot?) {
    val context = LocalContext.current
    val size = LocalSize.current
    val family = when {
        size.height >= MetamonWidget.LARGE.height -> Family.LARGE
        size.width >= MetamonWidget.MEDIUM.width -> Family.MEDIUM
        else -> Family.SMALL
    }
    val open = Intent(context, MainActivity::class.java)
        .setAction(WidgetLink.ACTION)
        .setData(Uri.parse(WidgetLink.uri(snapshot?.appId)))
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    Column(
        modifier = GlanceModifier.fillMaxSize().background(GlanceTheme.colors.widgetBackground).cornerRadius(16.dp)
            .padding(12.dp).clickable(actionStartActivity(open)),
    ) {
        when {
            snapshot == null -> Message(context.getString(if (ReviewBuild.enabled) R.string.widget_no_app_review else R.string.widget_no_app))
            snapshot.stats.isEmpty() -> {
                AppName(snapshot.appName)
                Message(context.getString(R.string.widget_no_stat, snapshot.appName))
            }
            else -> {
                AppName(snapshot.appName)
                Spacer(GlanceModifier.height(6.dp))
                when (family) {
                    Family.SMALL -> StatCell(snapshot.stats.first(), big = true)
                    Family.MEDIUM -> StatRow(snapshot.stats)
                    Family.LARGE -> Large(snapshot)
                }
            }
        }
    }
}

@Composable
private fun AppName(name: String) {
    Text(name, maxLines = 1, style = TextStyle(color = GlanceTheme.colors.primary, fontSize = 12.sp, fontWeight = FontWeight.Medium))
}

@Composable
private fun Message(text: String) {
    Box(GlanceModifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Text(text, style = TextStyle(color = GlanceTheme.colors.onSurfaceVariant, fontSize = 13.sp))
    }
}

@Composable
private fun StatCell(stat: WidgetSnapshot.Stat, big: Boolean, modifier: GlanceModifier = GlanceModifier) {
    Column(modifier) {
        Text(stat.label, maxLines = if (big) 2 else 1, style = TextStyle(color = GlanceTheme.colors.onSurfaceVariant, fontSize = 12.sp))
        Text(
            stat.value ?: "—",
            maxLines = 1,
            style = TextStyle(color = GlanceTheme.colors.onSurface, fontSize = if (big) 28.sp else 20.sp, fontWeight = FontWeight.Bold),
        )
    }
}

@Composable
private fun StatRow(stats: List<WidgetSnapshot.Stat>) {
    Row(GlanceModifier.fillMaxWidth()) {
        stats.forEach { StatCell(it, big = stats.size == 1, modifier = GlanceModifier.defaultWeight()) }
    }
}

@Composable
private fun Large(snapshot: WidgetSnapshot) {
    val context = LocalContext.current
    StatRow(snapshot.stats)
    snapshot.trend?.let { trend ->
        Spacer(GlanceModifier.height(10.dp))
        Row(GlanceModifier.fillMaxWidth()) {
            Text(
                context.getString(R.string.widget_last_7_days) + " · " + (trend.metric ?: context.getString(R.string.widget_count)),
                maxLines = 1,
                modifier = GlanceModifier.defaultWeight(),
                style = TextStyle(color = GlanceTheme.colors.onSurfaceVariant, fontSize = 11.sp),
            )
            Text(trend.total, maxLines = 1, style = TextStyle(color = GlanceTheme.colors.onSurface, fontSize = 11.sp, fontWeight = FontWeight.Bold))
        }
        Spacer(GlanceModifier.height(4.dp))
        Bars(trend)
    }
    val recent = snapshot.recent ?: return
    Spacer(GlanceModifier.height(10.dp))
    Text(
        context.getString(R.string.widget_recent) + (snapshot.recentTitle?.let { " · $it" } ?: ""),
        maxLines = 1,
        style = TextStyle(color = GlanceTheme.colors.onSurfaceVariant, fontSize = 11.sp),
    )
    if (recent.isEmpty()) {
        Text(context.getString(R.string.widget_no_records), style = TextStyle(color = GlanceTheme.colors.onSurfaceVariant, fontSize = 12.sp))
        return
    }
    recent.forEach { r ->
        Row(GlanceModifier.fillMaxWidth().padding(top = 2.dp), verticalAlignment = Alignment.CenterVertically) {
            Text(r.title, maxLines = 1, modifier = GlanceModifier.defaultWeight(), style = TextStyle(color = GlanceTheme.colors.onSurface, fontSize = 12.sp))
            r.detail?.let {
                Text(it, maxLines = 1, style = TextStyle(color = GlanceTheme.colors.onSurfaceVariant, fontSize = 11.sp))
                Spacer(GlanceModifier.width(6.dp))
            }
            r.value?.let { Text(it, maxLines = 1, style = TextStyle(color = GlanceTheme.colors.onSurface, fontSize = 12.sp, fontWeight = FontWeight.Medium)) }
        }
    }
}

@Composable
private fun Bars(trend: WidgetSnapshot.Trend) {
    val max = trend.points.maxOfOrNull { it.value }?.takeIf { it > 0 } ?: 1.0
    val locale = LocalContext.current.resources.configuration.locales[0]
    val barArea = 40
    Row(GlanceModifier.fillMaxWidth(), verticalAlignment = Alignment.Bottom) {
        trend.points.forEach { p ->
            val day = LocalDate.ofEpochDay(p.epochDay).dayOfWeek.getDisplayName(java.time.format.TextStyle.NARROW, locale)
            Column(GlanceModifier.defaultWeight(), horizontalAlignment = Alignment.CenterHorizontally) {
                val h = maxOf(2, (p.value / max * barArea).toInt())
                Box(
                    GlanceModifier.width(14.dp).height(h.dp).cornerRadius(3.dp).background(GlanceTheme.colors.primary)
                        .semantics { contentDescription = "$day ${p.value}" },
                ) {}
                Text(day, style = TextStyle(color = GlanceTheme.colors.onSurfaceVariant, fontSize = 10.sp))
            }
        }
    }
}
