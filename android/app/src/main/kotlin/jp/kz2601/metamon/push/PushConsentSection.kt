package jp.kz2601.metamon.push

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.services.push.NotifyConsentToggle
import kotlinx.coroutines.launch

/**
 * 設定の「他の人の行動で届く通知」(#339・iOS の `PushConsentSection`)。設定の画面(同意の PR)に置く受け口。
 * 共有・公開の記録が無いアプリ・モックの構成([host] が null)では何も出さない。
 */
@Composable
fun PushConsentSection(host: PushHost?, appId: String, dsl: AppDSL, modifier: Modifier = Modifier) {
    if (host == null || !PushHost.canSetConsent(dsl)) return
    var state by remember(appId) { mutableStateOf(NotifyConsentToggle.State(isOn = host.consentAllowed(appId))) }
    var busy by remember(appId) { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(appId) { state = NotifyConsentToggle.State(isOn = host.consentAllowed(appId)) }

    Column(modifier.testTag("settings.pushConsent")) {
        Text(
            stringResource(R.string.push_consent_header),
            style = MaterialTheme.typography.titleSmall,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
        Row(
            Modifier
                .fillMaxWidth()
                .toggleable(value = state.isOn, enabled = !busy, role = Role.Switch) { new ->
                    busy = true
                    scope.launch {
                        state = host.consent.change(new, host.currentUid(), appId) { state = it }
                        busy = false
                    }
                }
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(stringResource(R.string.push_consent_toggle), Modifier.weight(1f))
            Switch(checked = state.isOn, onCheckedChange = null)
        }
        Text(
            stringResource(if (state.failed) R.string.push_consent_failed else R.string.push_consent_footer),
            style = MaterialTheme.typography.bodySmall,
            color = if (state.failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
    }
}
