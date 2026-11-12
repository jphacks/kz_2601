import org.gradle.api.artifacts.component.ModuleComponentIdentifier
import org.gradle.api.artifacts.result.ResolvedArtifactResult
import org.gradle.maven.MavenModule
import org.gradle.maven.MavenPomArtifact
import java.io.File
import javax.xml.XMLConstants
import java.util.Properties
import java.util.zip.ZipFile
import javax.xml.parsers.DocumentBuilderFactory
import org.w3c.dom.Element

// デスクトップに同梱するライセンスの一覧(licenses/third_party_licenses.json)をビルド時に作る。
// 形は Android の res/raw/third_party_licenses.json(android/scripts/third-party-licenses.py)と同じ。
// 材料:
//   - リポジトリの LICENSE(このアプリ自身)
//   - runtimeClasspath の jar と、Gradle で解決した POM の <licenses>(無ければ親 POM)
//   - jar の META-INF の NOTICE と、LICENSE のうち Apache-2.0 の本文の後ろ(取り込んだ部品の表示。PDFBox の Adobe・Liberation など)
//   - 本文の正本は android/scripts/license-texts(POM には本文が無い)
//   - Maven を通らない同梱物は licenses/vendored/*.properties(1 件 1 ファイル。whisper.cpp・ggml・音声モデルは #2004 でここに足す)
// 種類が判定できない・本文の正本が無い依存があれば、推測で埋めずにビルドを止める。

/** POM の読み込み。DOCTYPE・外部実体を受け付けない(POM はリポジトリから落とした物なので信用しない)。 */
object PomXml {
    private val factory: DocumentBuilderFactory = DocumentBuilderFactory.newInstance().apply {
        setFeature(XMLConstants.FEATURE_SECURE_PROCESSING, true)
        setFeature("http://apache.org/xml/features/disallow-doctype-decl", true)
        setFeature("http://xml.org/sax/features/external-general-entities", false)
        setFeature("http://xml.org/sax/features/external-parameter-entities", false)
        setFeature("http://apache.org/xml/features/nonvalidating/load-external-dtd", false)
        setAttribute(XMLConstants.ACCESS_EXTERNAL_DTD, "")
        setAttribute(XMLConstants.ACCESS_EXTERNAL_SCHEMA, "")
        isXIncludeAware = false
        isExpandEntityReferences = false
    }

    fun parse(file: File): Element = synchronized(factory) { factory.newDocumentBuilder() }.parse(file).documentElement

    fun child(e: Element, name: String): Element? {
        val nodes = e.childNodes
        for (i in 0 until nodes.length) (nodes.item(i) as? Element)?.takeIf { it.tagName == name }?.let { return it }
        return null
    }

    fun text(e: Element, name: String): String? = child(e, name)?.textContent?.trim()?.takeIf { it.isNotEmpty() }

    fun licenseNames(root: Element): List<String> = child(root, "licenses")?.let { ls ->
        (0 until ls.childNodes.length).mapNotNull { (ls.childNodes.item(it) as? Element)?.let { l -> text(l, "name") } }
    }.orEmpty()

    /** 親 POM の group:artifact:version。無ければ null。 */
    fun parent(root: Element): String? = child(root, "parent")?.let { p ->
        listOf(text(p, "groupId"), text(p, "artifactId"), text(p, "version")).takeIf { it.all { v -> v != null } }?.joinToString(":")
    }
}

abstract class GenerateThirdPartyLicenses : DefaultTask() {
    /** group:artifact:version → jar の場所(場所は機械ごとに違うので入力は [jars] と [components] で見る)。 */
    @get:Internal
    abstract val jarsByComponent: MapProperty<String, String>

    @get:Input
    val components: Provider<List<String>> get() = jarsByComponent.map { it.keys.sorted() }

    @get:InputFiles
    @get:PathSensitive(PathSensitivity.NAME_ONLY)
    abstract val jars: ConfigurableFileCollection

    @get:InputFile
    @get:PathSensitive(PathSensitivity.RELATIVE)
    abstract val appLicense: RegularFileProperty

    @get:InputDirectory
    @get:PathSensitive(PathSensitivity.RELATIVE)
    abstract val textsDir: DirectoryProperty

    @get:InputDirectory
    @get:PathSensitive(PathSensitivity.RELATIVE)
    abstract val vendoredDir: DirectoryProperty

    /** vendored の text の相対パスの基点(リポジトリの根)。 */
    @get:Internal
    abstract val repoDir: DirectoryProperty

    /** group:artifact:version → Gradle で解決した POM(<licenses> が無い物は親 POM も)。 */
    @get:Internal
    abstract val pomsByComponent: MapProperty<String, String>

    @get:InputFiles
    @get:PathSensitive(PathSensitivity.NAME_ONLY)
    abstract val poms: ConfigurableFileCollection

    @get:OutputDirectory
    abstract val outputDir: DirectoryProperty

    // POM の <license><name> → SPDX(android/scripts/third-party-licenses.py の SPDX_BY_NAME と揃える)
    private val spdxByName = mapOf(
        "apache software license, version 2.0" to "Apache-2.0",
        "apache license, version 2.0" to "Apache-2.0",
        "apache-2.0" to "Apache-2.0",
        "apache 2.0" to "Apache-2.0",
        "bsd-3-clause" to "BSD-3-Clause",
        "mit license" to "MIT",
        "mit" to "MIT",
    )

    // 本文の正本(license-texts のファイル)。Apache-2.0 以外は著作権者の表示が要るので、依存ごとの正本を足してから使う
    private val textFiles = mapOf("Apache-2.0" to "Apache-2.0.txt")

    companion object {
        /** 親 POM を辿る深さの上限。 */
        const val MAX_PARENT_DEPTH = 5
    }

    @TaskAction
    fun generate() {
        val errors = mutableListOf<String>()
        class Group(val group: String, val license: String) {
            val artifacts = sortedMapOf<String, String>()
            var url: String? = null
            val notices = mutableListOf<Pair<String, String>>()
        }
        val groups = sortedMapOf<String, Group>()
        for ((coord, jarPath) in jarsByComponent.get().toSortedMap()) {
            val (group, artifact, version) = coord.split(":")
            val pom = pomLicenses(group, artifact, version)
            if (pom == null) {
                errors += "POM を解決できません: $coord"
                continue
            }
            if (pom.isEmpty()) {
                errors += "$coord のライセンスが POM にありません"
                continue
            }
            var notices = jarNotices(File(jarPath), artifact)
            for (name in pom) {
                val spdx = spdxByName[normalize(name)]
                if (spdx == null) {
                    errors += "$coord のライセンス「$name」を判定できません(third-party-licenses.gradle.kts の spdxByName に足す)"
                    continue
                }
                val g = groups.getOrPut("$group\u0000$spdx") { Group(group, spdx) }
                g.artifacts[artifact] = version
                g.url = g.url ?: pomUrl(group, artifact, version)
                g.notices += notices
                notices = emptyList() // 同じ jar の表示は 1 回だけ
            }
        }
        val perGroup = groups.values.groupingBy { it.group }.eachCount()
        val seen = mutableSetOf<String>()
        val deps = groups.values.mapNotNull { g ->
            val file = textFiles[g.license]
            if (file == null) {
                errors += "${g.group}(${g.license})の本文の正本がありません(license-texts と textFiles に足す)"
                return@mapNotNull null
            }
            val versions = g.artifacts.values.toSortedSet()
            var id = slug(g.group)
            if ((perGroup[g.group] ?: 0) > 1) id += "-" + slug(g.license)
            mapOf(
                "kind" to "dependency",
                "id" to id,
                "name" to g.group,
                "version" to versions.singleOrNull(),
                "url" to g.url,
                "license" to g.license,
                "copyright" to emptyList<String>(),
                "artifacts" to g.artifacts.map { (a, v) -> "$a:$v" },
                "text" to readText(textsDir.file(file).get().asFile),
                // 同じ表示(NOTICE が同じ文の jar)は一覧全体で 1 回だけ
                "notices" to g.notices.filter { seen.add(it.second) }.sortedBy { it.first.lowercase() }
                    .map { mapOf("title" to it.first, "text" to it.second) },
            )
        }
        val vendored = vendoredDir.get().asFile.listFiles { f -> f.extension == "properties" }.orEmpty().sortedBy { it.name }.map { f ->
            val p = Properties().apply { f.reader(Charsets.UTF_8).use { load(it) } }
            fun req(key: String) = p.getProperty(key)?.trim()?.takeIf { it.isNotEmpty() }
                ?: throw GradleException("${f.name} に $key がありません")
            val text = readText(repoDir.get().asFile.resolve(req("text")))
            mapOf(
                "kind" to "dependency",
                "id" to req("id"),
                "name" to req("name"),
                "version" to p.getProperty("version")?.trim()?.takeIf { it.isNotEmpty() },
                "url" to p.getProperty("url")?.trim()?.takeIf { it.isNotEmpty() },
                "license" to req("license"),
                "copyright" to (p.getProperty("copyright")?.split("|")?.map { it.trim() }?.filter { it.isNotEmpty() }
                    ?: text.lines().filter { it.startsWith("Copyright") }),
                "artifacts" to p.getProperty("artifacts").orEmpty().split("|").map { it.trim() }.filter { it.isNotEmpty() },
                "text" to text,
                "notices" to emptyList<Any>(),
            )
        }
        if (errors.isNotEmpty()) throw GradleException(errors.joinToString("\n"))
        val appText = readText(appLicense.get().asFile)
        val app = mapOf(
            "kind" to "app",
            "id" to "tailor-it",
            "name" to "Tailor It",
            "version" to null,
            "url" to "https://github.com/jphacks/kz_2601",
            "license" to "MIT",
            "copyright" to appText.lines().filter { it.startsWith("Copyright") },
            "artifacts" to emptyList<String>(),
            "text" to appText,
            "notices" to emptyList<Any>(),
        )
        val out = outputDir.get().asFile.resolve("licenses/third_party_licenses.json")
        out.parentFile.mkdirs()
        out.writeText(json(mapOf("source" to "desktop :desktop runtimeClasspath", "entries" to listOf(app) + vendored + deps)) + "\n")
    }

    private fun pomFile(coord: String): File? = pomsByComponent.get()[coord]?.let(::File)

    /** POM の <licenses> の名前。無ければ親を辿る。POM が無ければ null。 */
    private fun pomLicenses(group: String, artifact: String, version: String): List<String>? {
        var root = PomXml.parse(pomFile("$group:$artifact:$version") ?: return null)
        repeat(MAX_PARENT_DEPTH) {
            val licenses = PomXml.licenseNames(root)
            if (licenses.isNotEmpty()) return licenses
            val parent = PomXml.parent(root) ?: return emptyList()
            root = PomXml.parse(pomFile(parent) ?: return null)
        }
        return PomXml.licenseNames(root)
    }

    private fun pomUrl(group: String, artifact: String, version: String): String? {
        val root = PomXml.parse(pomFile("$group:$artifact:$version") ?: return null)
        return PomXml.child(root, "scm")?.let { PomXml.text(it, "url") } ?: PomXml.text(root, "url")
    }

    /** jar の META-INF の NOTICE(そのまま)と LICENSE(Apache-2.0 の本文より後ろだけ・違う種類なら全文)。 */
    private fun jarNotices(jar: File, artifact: String): List<Pair<String, String>> {
        if (!jar.isFile || jar.extension != "jar") return emptyList()
        val notice = Regex("""META-INF/(?:.*/)?NOTICE(?:\.txt|\.md)?""", RegexOption.IGNORE_CASE)
        val license = Regex("""META-INF/(?:.*/)?LICENSE(?:\.txt|\.md)?""", RegexOption.IGNORE_CASE)
        val out = mutableListOf<Pair<String, String>>()
        ZipFile(jar).use { z ->
            for (e in z.entries().asSequence().filter { !it.isDirectory }.sortedBy { it.name }) {
                val raw = { normalizeText(z.getInputStream(e).readBytes().toString(Charsets.UTF_8)) }
                when {
                    notice.matches(e.name) -> out += "$artifact (NOTICE)" to raw()
                    license.matches(e.name) -> licenseExtras(raw())?.let { out += "$artifact (LICENSE)" to it }
                }
            }
        }
        return out
    }

    private fun licenseExtras(text: String): String? {
        val end = text.indexOf("END OF TERMS AND CONDITIONS")
        if (end < 0) return text.takeIf { it.isNotBlank() }
        var rest = text.substring(end + "END OF TERMS AND CONDITIONS".length)
        val appendix = rest.indexOf("APPENDIX: How to apply the Apache License")
        if (appendix >= 0) {
            val tail = "limitations under the License."
            val at = rest.indexOf(tail, appendix)
            rest = if (at >= 0) rest.substring(at + tail.length) else ""
        }
        return rest.trim('\n', ' ').takeIf { it.isNotBlank() }?.let { normalizeText(it) }
    }

    private fun normalize(name: String) = name.trim().replace(Regex("\\s+"), " ").lowercase().removePrefix("the ")

    private fun slug(s: String) = s.lowercase().replace(Regex("[^a-z0-9]+"), "-").trim('-')

    private fun readText(f: File) = normalizeText(f.readText(Charsets.UTF_8))

    private fun normalizeText(s: String) =
        s.replace("\r\n", "\n").lines().joinToString("\n") { it.trimEnd() }.trim('\n') + "\n"

    private fun json(v: Any?): String = when (v) {
        null -> "null"
        is String -> buildString {
            append('"')
            for (c in v) when {
                c == '"' -> append("\\\"")
                c == '\\' -> append("\\\\")
                c == '\n' -> append("\\n")
                c == '\t' -> append("\\t")
                c < ' ' -> append("\\u%04x".format(c.code))
                else -> append(c)
            }
            append('"')
        }
        is Map<*, *> -> v.entries.joinToString(",", "{", "}") { (k, x) -> json(k.toString()) + ":" + json(x) }
        is Iterable<*> -> v.joinToString(",", "[", "]") { json(it) }
        else -> json(v.toString())
    }
}

val runtimeArtifacts = configurations.named("runtimeClasspath").flatMap { it.incoming.artifacts.resolvedArtifacts }

/**
 * POM を Gradle の解決で取る(キャッシュの置き場所に依存しない)。<licenses> が無い POM は親 POM も段ごとに解決する。
 * 解決できなかった物は入れない(タスクがエラーにする)。
 */
fun resolvePoms(coords: Collection<String>): Map<String, String> {
    val out = sortedMapOf<String, String>()
    var pending = coords.toSortedSet()
    repeat(GenerateThirdPartyLicenses.MAX_PARENT_DEPTH + 1) {
        if (pending.isEmpty()) return out
        val query = dependencies.createArtifactResolutionQuery()
        pending.forEach { val (g, a, v) = it.split(":"); query.forModule(g, a, v) }
        val next = sortedSetOf<String>()
        for (c in query.withArtifacts(MavenModule::class.java, MavenPomArtifact::class.java).execute().resolvedComponents) {
            val id = c.id as? ModuleComponentIdentifier ?: continue
            val pom = c.getArtifacts(MavenPomArtifact::class.java).filterIsInstance<ResolvedArtifactResult>().firstOrNull()?.file ?: continue
            out["${id.group}:${id.module}:${id.version}"] = pom.absolutePath
            val root = PomXml.parse(pom)
            if (PomXml.licenseNames(root).isEmpty()) PomXml.parent(root)?.takeIf { it !in out }?.let { next += it }
        }
        pending = next
    }
    return out
}

tasks.register<GenerateThirdPartyLicenses>("thirdPartyLicenses") {
    group = "build"
    description = "同梱するライセンスの一覧(licenses/third_party_licenses.json)を runtimeClasspath の POM から作る"
    jarsByComponent.set(runtimeArtifacts.map { arts ->
        arts.mapNotNull { a ->
            (a.id.componentIdentifier as? ModuleComponentIdentifier)?.let { "${it.group}:${it.module}:${it.version}" to a.file.absolutePath }
        }.toMap()
    })
    // 解決は 1 回だけ(一覧とタスクの入力の両方で使う)
    val resolvedPoms = objects.mapProperty(String::class.java, String::class.java).apply {
        set(runtimeArtifacts.map { arts ->
            resolvePoms(arts.mapNotNull { (it.id.componentIdentifier as? ModuleComponentIdentifier)?.let { c -> "${c.group}:${c.module}:${c.version}" } })
        })
        finalizeValueOnRead()
    }
    pomsByComponent.set(resolvedPoms)
    poms.from(resolvedPoms.map { it.values.map(::File) })
    jars.from(runtimeArtifacts.map { arts -> arts.filter { it.id.componentIdentifier is ModuleComponentIdentifier }.map { it.file } })
    val repo = rootDir.resolve("..")
    repoDir.set(repo)
    appLicense.set(repo.resolve("LICENSE"))
    textsDir.set(repo.resolve("android/scripts/license-texts"))
    vendoredDir.set(projectDir.resolve("licenses/vendored"))
    outputDir.set(layout.buildDirectory.dir("generated/thirdPartyLicenses"))
}
