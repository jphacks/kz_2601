package jp.kz2601.metamon.desktop.io

import java.awt.GraphicsEnvironment
import java.awt.Toolkit
import java.awt.datatransfer.Clipboard
import java.awt.datatransfer.DataFlavor
import java.awt.datatransfer.StringSelection
import java.security.MessageDigest

/**
 * 書き出し・コピーで入れた表(TSV)をクリップボードへ。入れた中身は要約(SHA-256)だけ覚え、
 * 「この端末のデータを消す」でまだ同じ中身ならクリップボードを空にする。
 */
object ExportClipboard {
    @Volatile
    private var lastDigest: ByteArray? = null

    fun system(): Clipboard? =
        if (GraphicsEnvironment.isHeadless()) null else runCatching { Toolkit.getDefaultToolkit().systemClipboard }.getOrNull()

    /** [text] をクリップボードへ入れて覚える。入れられたか。 */
    fun put(text: String, clipboard: Clipboard? = system()): Boolean {
        val cb = clipboard ?: return false
        return runCatching { cb.setContents(StringSelection(text), null) }.isSuccess.also { if (it) lastDigest = digest(text) }
    }

    /** クリップボードがまだ自分の入れた表なら空にする。空にしたか。 */
    fun clearIfOurs(clipboard: Clipboard? = system()): Boolean {
        val mark = lastDigest ?: return false
        val cb = clipboard ?: return false
        val now = runCatching { cb.getData(DataFlavor.stringFlavor) as? String }.getOrNull()
        lastDigest = null
        if (now == null || !MessageDigest.isEqual(mark, digest(now))) return false
        return runCatching { cb.setContents(StringSelection(""), null) }.isSuccess
    }

    private fun digest(s: String): ByteArray = MessageDigest.getInstance("SHA-256").digest(s.toByteArray(Charsets.UTF_8))
}
