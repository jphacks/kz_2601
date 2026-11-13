package jp.kz2601.metamon.desktop.io

import java.io.File
import java.net.URI
import java.nio.file.Files
import java.nio.file.LinkOption
import java.nio.file.Path
import java.nio.file.attribute.PosixFileAttributeView
import java.nio.file.attribute.PosixFilePermissions
import java.util.Locale

/**
 * アプリが手元に書くフォルダ(貼った写真・引き出す一時ファイル・カレンダーに渡すファイル)。利用者ごと・本人だけが読み書きできる。
 * Linux は XDG のキャッシュ(~/.cache)、他(macOS・Windows は一時フォルダが利用者ごと)は一時フォルダの下に利用者名付きで。
 */
object UserDirs {
    val root: File by lazy { resolveRoot() }

    /** 貼った・選んだ・描いた写真。記録の値の file URI はここの下だけ読む。 */
    val photos: File by lazy { sub("photos") }

    /** 外へ引き出すファイル。 */
    val drag: File by lazy { sub("drag") }

    /** カレンダーに渡す .ics。 */
    val calendar: File by lazy { sub("calendar") }

    private fun sub(name: String): File = ownedDir(File(root, name).toPath())?.toFile()
        ?: Files.createTempDirectory("tailorit-$name-").toFile()

    private fun resolveRoot(): File {
        val os = System.getProperty("os.name").orEmpty().lowercase(Locale.ROOT)
        val base = if (os.contains("linux")) {
            System.getenv("XDG_CACHE_HOME")?.takeIf { it.startsWith("/") }?.let(::File)
                ?: File(System.getProperty("user.home"), ".cache")
        } else {
            File(System.getProperty("java.io.tmpdir"))
        }
        val dir = if (os.contains("linux")) File(base, "tailorit") else File(base, "tailorit-" + userTag())
        return ownedDir(dir.toPath())?.toFile() ?: Files.createTempDirectory("tailorit-").toFile()
    }

    /** ファイル名に使える利用者名。 */
    fun userTag(): String =
        System.getProperty("user.name").orEmpty().map { if (it.isLetterOrDigit() || it == '-' || it == '_' || it == '.') it else '_' }
            .joinToString("").trim('.').ifEmpty { "user" }

    /**
     * [path] を本人だけのフォルダとして用意する。リンク・他の利用者の物・作れない時は null。
     * POSIX では 700 で作り、既にあれば持ち主が本人かを見て 700 にそろえる。
     */
    fun ownedDir(path: Path): Path? = runCatching {
        val posix = "posix" in path.fileSystem.supportedFileAttributeViews()
        if (Files.isSymbolicLink(path)) return null
        if (!Files.exists(path, LinkOption.NOFOLLOW_LINKS)) {
            path.parent?.let { Files.createDirectories(it) }
            if (posix) {
                Files.createDirectory(path, PosixFilePermissions.asFileAttribute(PosixFilePermissions.fromString("rwx------")))
            } else {
                Files.createDirectory(path)
            }
        }
        if (!Files.isDirectory(path, LinkOption.NOFOLLOW_LINKS)) return null
        if (posix) {
            val me = path.fileSystem.userPrincipalLookupService.lookupPrincipalByName(System.getProperty("user.name"))
            val view = Files.getFileAttributeView(path, PosixFileAttributeView::class.java, LinkOption.NOFOLLOW_LINKS)
            if (view.owner != me) return null
            view.setPermissions(PosixFilePermissions.fromString("rwx------"))
        }
        path
    }.getOrNull()

    /**
     * 記録の値の file URI を、写真フォルダ([dir])の下の画像のファイルとして開く。
     * ネットワークの場所(`//`・`\\`・ホスト付きの URI)と、フォルダの外は null。
     */
    fun photoFile(uri: String, dir: File = photos): File? {
        if (!uri.startsWith("file:")) return null
        val rest = uri.removePrefix("file:")
        if (rest.contains('\\')) return null
        val u = runCatching { URI(uri) }.getOrNull() ?: return null
        if (!u.host.isNullOrEmpty() || !u.rawAuthority.isNullOrEmpty()) return null
        val path = u.path ?: return null
        if (path.startsWith("//") || path.startsWith("\\\\")) return null
        val f = runCatching { File(u) }.getOrNull() ?: return null
        if (f.path.startsWith("\\\\") || f.path.startsWith("//")) return null
        val canon = runCatching { f.canonicalFile }.getOrNull() ?: return null
        val base = runCatching { dir.canonicalFile }.getOrNull() ?: return null
        return canon.takeIf { it.toPath().startsWith(base.toPath()) && it != base }
    }

    /** 選んだ・落とした写真を写真フォルダへ写す(既に下にあればそのまま)。 */
    fun adoptPhoto(file: File, dir: File = photos): File {
        photoFile(file.toURI().toString(), dir)?.let { return it }
        val ext = file.name.substringAfterLast('.', "png").lowercase(Locale.ROOT).filter { it.isLetterOrDigit() }.ifEmpty { "png" }
        val copy = File.createTempFile("pick-", ".$ext", dir)
        file.inputStream().use { input -> copy.outputStream().use { input.copyTo(it) } }
        return copy
    }
}
