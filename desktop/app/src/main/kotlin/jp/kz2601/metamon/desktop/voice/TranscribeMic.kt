package jp.kz2601.metamon.desktop.voice

import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.renderer.form.FormDraft
import java.awt.EventQueue

/** transcribeField の下のマイク。押すと録り、もう一度押すと止めて文字にし、欄の後ろに足す(端末の中だけ)。 */
@Composable
fun TranscribeMic(voice: VoiceInput, form: FormDraft, field: String, onMessage: (String) -> Unit) {
    var state by remember { mutableStateOf(voice.state) }
    val label = when (state) {
        VoiceInput.State.IDLE -> Strings["desktop_input_transcribe_speak"]
        VoiceInput.State.RECORDING -> Strings["desktop_input_transcribe_stop"]
        VoiceInput.State.TRANSCRIBING -> Strings["desktop_voice_transcribing"]
    }
    OutlinedButton(
        onClick = {
            val now = voice.toggleInto(form, field, post = { msg ->
                EventQueue.invokeLater {
                    state = voice.state
                    onMessage(msg)
                }
            })
            state = voice.state
            onMessage(now)
        },
        enabled = !form.isReadOnly && state != VoiceInput.State.TRANSCRIBING,
    ) { Text(label) }
}
