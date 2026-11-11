package jp.kz2601.metamon.settings

import android.content.Intent
import android.net.Uri
import android.provider.Settings
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.notifications.LocalNotifications
import jp.kz2601.metamon.notifications.UpcomingNotificationList
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * 設定の「次の通知」(#1124・iOS の `SettingsSheet` の通知の節)。このアプリの近い順に 5 件と場所の通知の案内。
 * 通知の無いアプリでは出さない。開くたびに予約し直してから読む(iOS の `.task { refreshNotifications() }`)。
 */
@Composable
fun UpcomingNotificationsSection(appId: String, hasNotifications: Boolean) {
    if (!hasNotifications) return
    val context = LocalContext.current
    val notifications = LocalNotifications.get(context)
    val list by produceState(UpcomingNotificationList.EMPTY, appId) {
        value = withContext(Dispatchers.IO) {
            notifications.refresh()
            notifications.upcoming(appId)
        }
    }
    SettingsHeader(stringResource(R.string.settings_upcoming_header))
    if (list.locationOff) {
        Text(
            stringResource(R.string.settings_upcoming_places_off),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.primary,
            modifier = Modifier.fillMaxWidth()
                .clickable {
                    val intent = Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", context.packageName, null))
                    runCatching { context.startActivity(intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) }
                }
                .padding(horizontal = 16.dp, vertical = 8.dp)
                .testTag("settings.placesLocationOff"),
        )
    } else if (list.places > 0) {
        SecondaryRow(stringResource(R.string.settings_upcoming_places, list.places))
    }
    if (list.isEmpty) SecondaryRow(stringResource(R.string.settings_upcoming_empty))
    for (n in list.rows) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp).semantics(mergeDescendants = true) {}.testTag("settings.upcoming"),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f)) {
                Text(n.title, style = MaterialTheme.typography.bodyMedium)
                if (n.body.isNotEmpty()) {
                    Text(
                        n.body,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
            Text(notifications.whenText(n.at), style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(start = 8.dp))
        }
    }
    SettingsFooter(stringResource(R.string.settings_upcoming_footer))
    HorizontalDivider(Modifier.padding(vertical = 8.dp))
}

@Composable
private fun SecondaryRow(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
}
