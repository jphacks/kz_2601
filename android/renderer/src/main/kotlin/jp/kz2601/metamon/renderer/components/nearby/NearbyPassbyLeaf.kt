package jp.kz2601.metamon.renderer.components.nearby

import android.text.format.DateFormat
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.nearby.NearbyLimits
import jp.kz2601.metamon.dsl.nearby.NearbyPassby
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

/**
 * `nearbyShare` mode:passby(すれ違い通信・#1383・iOS の `NearbyPassbyLeaf`)。オンの間、この画面を開いていて前面にいる時だけ
 * 近くの同じアプリの人と `fields` の項目を自動で交換し、会った人と日時を並べる。初回オンの前に同意を取る(相手に渡る表示名と
 * 項目を見せ、匿名も選べる)。オンにする時に近くのデバイスの権限を聞く。通信はホスト([NearbyHost.passby])。
 */
@Composable
fun NearbyPassbyLeaf(c: Component, config: NearbyPassby.Config, ctx: RenderContext, modifier: Modifier = Modifier) {
    val host = ctx.runtime.nearby?.passby ?: remember { StaticNearbyPassbyController() }
    val state by host.state.collectAsState()
    val fieldNames = PassbyCopy.fieldNames(config, ctx.runtime.entityModel(config.entityId))
    var askConsent by rememberSaveable { mutableStateOf(false) }

    // 前面(STARTED 以上)で画面にある間だけ探す。裏に回る・画面を離れると止める(iOS と同じ・Foreground Service は使わない)
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    var foreground by remember { mutableStateOf(lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) }
    DisposableEffect(lifecycle) {
        val observer = LifecycleEventObserver { _, _ -> foreground = lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED) }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer) }
    }
    LaunchedEffect(host, config, foreground) { if (foreground) host.attach(config) else host.detach(config) }
    DisposableEffect(host, config) { onDispose { host.detach(config) } }

    var pendingOn by remember { mutableStateOf(false) }
    val askPermission = LocalNearbyPermission.current { granted ->
        val turnOn = pendingOn && granted
        pendingOn = false
        if (turnOn) {
            when (NearbyPassby.toggle(on = true, consentedVersion = host.state.value.consentedVersion)) {
                NearbyPassby.ToggleOutcome.ASK_CONSENT -> askConsent = true
                NearbyPassby.ToggleOutcome.START -> host.setOptIn(true, config)
                NearbyPassby.ToggleOutcome.STOP -> Unit
            }
        }
    }
    val optedIn = host.isOptedIn(config) || config.componentId in state.optedIn
    val anonymousLabel = stringResource(R.string.passby_anonymous)

    Column(
        modifier.renderCard().testTag("passby.${config.componentId}"),
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        Row(Modifier.fillMaxWidth().defaultMinSize(minHeight = 48.dp), verticalAlignment = Alignment.CenterVertically) {
            SymbolIcon("person.2.wave.2", 20.dp, RenderTheme.colors.brandA)
            Text(
                config.label,
                Modifier.weight(1f).padding(start = RenderTheme.Spacing.sm).semantics { heading() },
                style = MaterialTheme.typography.titleMedium,
            )
            Switch(
                checked = optedIn,
                onCheckedChange = { on ->
                    if (on) {
                        pendingOn = true
                        askPermission()
                    } else {
                        host.setOptIn(false, config)
                    }
                },
                modifier = Modifier.testTag("passby.toggle.${config.componentId}"),
            )
        }
        Status(state, optedIn, fieldNames)
        if (state.hasConsent) {
            Row(
                Modifier.fillMaxWidth().defaultMinSize(minHeight = 48.dp).testTag("passby.anonymous.${config.componentId}"),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(Modifier.weight(1f)) {
                    Text(stringResource(R.string.passby_hide_name), style = MaterialTheme.typography.bodyMedium)
                    Text(
                        stringResource(R.string.passby_public_name, NearbyLimits.displayName(state.publicName, anonymousLabel)),
                        style = MaterialTheme.typography.bodySmall,
                        color = RenderTheme.colors.secondaryLabel,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                Switch(checked = state.isAnonymous, onCheckedChange = host::setAnonymous)
            }
        }
        Note("info.circle", stringResource(R.string.passby_stops_note))
        // 同じ OS どうしだけつながる(#1384・近くの人に送るシートと同じ文)
        Note("info.circle", stringResource(R.string.passby_ios_not_supported), Modifier.testTag("passby.iosNote"))
        state.notice?.let { Note("exclamationmark.triangle", it, tint = RenderTheme.colors.warning) }
        if (config.distance) DistanceSection(state, anonymousLabel)
        HorizontalDivider()
        Encounters(state.encounters, anonymousLabel)
    }

    if (askConsent) {
        PassbyConsentSheet(
            displayName = state.displayName,
            anonymous = state.isAnonymous,
            fieldNames = fieldNames,
            onDismiss = { askConsent = false },
            onAgree = { anonymous ->
                askConsent = false
                host.agree(anonymous)
                host.setOptIn(true, config)
            },
        )
    }
}

@Composable
private fun Status(state: PassbyState, optedIn: Boolean, fieldNames: List<String>) {
    when {
        state.isAdvertising -> Row(
            Modifier.testTag("passby.advertising"),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
        ) {
            CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp, color = RenderTheme.colors.brandA)
            Text(
                stringResource(R.string.passby_searching),
                style = MaterialTheme.typography.bodyMedium,
                fontWeight = FontWeight.SemiBold,
                color = RenderTheme.colors.brandA,
            )
        }
        optedIn -> Text(stringResource(R.string.passby_paused), style = MaterialTheme.typography.bodyMedium, color = RenderTheme.colors.secondaryLabel)
        else -> Text(
            if (fieldNames.isEmpty()) stringResource(R.string.passby_no_fields)
            else stringResource(R.string.passby_off_hint, PassbyCopy.quoted(fieldNames)),
            style = MaterialTheme.typography.bodySmall,
            color = RenderTheme.colors.secondaryLabel,
        )
    }
}

@Composable
private fun Note(icon: String, text: String, modifier: Modifier = Modifier, tint: androidx.compose.ui.graphics.Color = RenderTheme.colors.secondaryLabel) {
    Row(modifier, horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs)) {
        SymbolIcon(icon, 14.dp, tint, Modifier.padding(top = 2.dp))
        Text(text, style = MaterialTheme.typography.bodySmall, color = tint)
    }
}

/** distance: Android は距離・方向を測らない(UWB の端末が限られる)。iOS の測れない端末と同じ形で出す。 */
@Composable
private fun DistanceSection(state: PassbyState, anonymousLabel: String) {
    when (val d = state.distanceSupport) {
        is PassbyDistanceSupport.Unsupported -> Text(
            d.reason ?: stringResource(R.string.passby_distance_unsupported),
            style = MaterialTheme.typography.bodySmall,
            color = RenderTheme.colors.secondaryLabel,
            modifier = Modifier.testTag("passby.distance.unsupported"),
        )
        PassbyDistanceSupport.Supported -> state.peers.forEach { peer ->
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                Text(
                    NearbyLimits.displayName(peer.name, anonymousLabel),
                    Modifier.weight(1f),
                    style = MaterialTheme.typography.bodyMedium,
                    fontWeight = FontWeight.SemiBold,
                    maxLines = 1,
                )
                Text(peer.distance?.let(NearbyPassby::distanceText) ?: stringResource(R.string.passby_measuring), style = MaterialTheme.typography.bodyMedium)
            }
        }
    }
}

@Composable
private fun Encounters(encounters: List<NearbyPassby.Encounter>, anonymousLabel: String) {
    if (encounters.isEmpty()) {
        Text(stringResource(R.string.passby_no_encounters), style = MaterialTheme.typography.bodySmall, color = RenderTheme.colors.secondaryLabel)
        return
    }
    val format = rememberMetFormat()
    Column(Modifier.fillMaxWidth().testTag("passby.encounters"), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        encounters.forEach { e ->
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                SymbolIcon("person.crop.circle", 24.dp, RenderTheme.colors.secondaryLabel)
                Column(Modifier.weight(1f)) {
                    Text(
                        NearbyLimits.displayName(e.name, anonymousLabel),
                        style = MaterialTheme.typography.bodyMedium,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                    Text(format(e.lastMet), style = MaterialTheme.typography.bodySmall, color = RenderTheme.colors.secondaryLabel)
                }
                Text(pluralStringResource(R.plurals.passby_times, e.times, e.times), style = MaterialTheme.typography.bodySmall, color = RenderTheme.colors.secondaryLabel)
            }
        }
    }
}

/** 会った日時(月・日・時・分。iOS の `.dateTime.month().day().hour().minute()`)。 */
@Composable
private fun rememberMetFormat(): (Instant) -> String = remember {
    val locale = Locale.getDefault()
    val f = DateTimeFormatter.ofPattern(DateFormat.getBestDateTimePattern(locale, "MMMdjmm"), locale)
    val zone = ZoneId.systemDefault()
    val format: (Instant) -> String = { t -> f.format(t.atZone(zone)) }
    format
}

/** すれ違い通信の同意(iOS の `PassbyConsentSheet`)。相手に渡るもの(表示名・項目)を並べ、名前を出さない(匿名)を選べる。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PassbyConsentSheet(
    displayName: String,
    anonymous: Boolean,
    fieldNames: List<String>,
    onDismiss: () -> Unit,
    onAgree: (Boolean) -> Unit,
) {
    var hide by rememberSaveable { mutableStateOf(anonymous) }
    val shownName = NearbyLimits.displayName(NearbyLimits.publicName(displayName, hide), stringResource(R.string.passby_anonymous))
    val fieldsText = if (fieldNames.isEmpty()) stringResource(R.string.passby_consent_no_fields) else PassbyCopy.quoted(fieldNames)
    val consent = PassbyCopy.consent(
        intro = stringResource(R.string.passby_consent_intro),
        nameLine = stringResource(R.string.passby_consent_name_line, shownName),
        fieldsLine = stringResource(R.string.passby_consent_fields_line, fieldsText),
        records = stringResource(R.string.passby_consent_records),
        stops = stringResource(R.string.passby_consent_stops),
    )
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)) {
        Column(
            Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 20.dp).navigationBarsPadding(),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(
                stringResource(R.string.passby_consent_title),
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.semantics { heading() },
            )
            Text(stringResource(R.string.passby_consent_shared), style = MaterialTheme.typography.titleSmall, modifier = Modifier.semantics { heading() })
            Row(Modifier.fillMaxWidth().testTag("passby.consent.name"), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(stringResource(R.string.passby_consent_display_name), Modifier.weight(1f), style = MaterialTheme.typography.bodyLarge)
                Text(shownName, style = MaterialTheme.typography.bodyLarge, color = RenderTheme.colors.secondaryLabel)
            }
            if (fieldNames.isEmpty()) {
                Text(stringResource(R.string.passby_no_fields), color = RenderTheme.colors.secondaryLabel)
            } else {
                fieldNames.forEach { name ->
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                        SymbolIcon("checkmark.circle", 18.dp, RenderTheme.colors.brandA)
                        Text(name, style = MaterialTheme.typography.bodyLarge)
                    }
                }
            }
            HorizontalDivider()
            Row(
                Modifier.fillMaxWidth().defaultMinSize(minHeight = 48.dp).testTag("passby.consent.anonymous"),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(stringResource(R.string.passby_hide_name), Modifier.weight(1f), style = MaterialTheme.typography.bodyLarge)
                Switch(checked = hide, onCheckedChange = { hide = it })
            }
            Text(stringResource(R.string.passby_consent_anonymous_footer), style = MaterialTheme.typography.bodySmall, color = RenderTheme.colors.secondaryLabel)
            HorizontalDivider()
            Text(consent, style = MaterialTheme.typography.bodySmall, color = RenderTheme.colors.secondaryLabel)
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TextButton(onClick = onDismiss) { Text(stringResource(R.string.passby_cancel)) }
                TextButton(onClick = { onAgree(hide) }, modifier = Modifier.testTag("passby.consent.agree")) {
                    Text(stringResource(R.string.passby_agree))
                }
            }
        }
    }
}

/** すれ違い通信の画面の文の組み立て(View から切り離してテストする)。 */
object PassbyCopy {
    /** 送ってよい項目の名前(DSL の並び)。entity が無ければ空。 */
    fun fieldNames(config: NearbyPassby.Config, entity: jp.kz2601.metamon.dsl.records.Entity?): List<String> =
        entity?.let { NearbyPassby.allowedFields(config, it).map { f -> f.name } }.orEmpty()

    /** 「名前」「趣味」の形(iOS と同じく区切りなしで並べる)。 */
    fun quoted(names: List<String>): String = names.joinToString("") { "「$it」" }

    /** 同意文([NearbyPassby.consentText] と同じ形を、画面の言語の文で組む)。 */
    fun consent(intro: String, nameLine: String, fieldsLine: String, records: String, stops: String): String =
        "$intro\n$nameLine\n$fieldsLine\n$records$stops"
}
