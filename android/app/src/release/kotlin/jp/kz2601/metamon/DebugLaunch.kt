package jp.kz2601.metamon

import android.content.Intent

/** release ではつまみを読まない(#844)。debug の版は src/debug にある。 */
object DebugLaunch {
    fun knobs(intent: Intent?): DebugKnobs = DebugKnobs.NONE
}
