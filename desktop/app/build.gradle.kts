import org.jetbrains.compose.desktop.application.dsl.TargetFormat
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

// :desktop — Windows / Linux / macOS のアプリ本体。DSL の読み込み・式・レコードの規則は Android と同じ :dsl を使う。
plugins {
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.compose.multiplatform)
}

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

kotlin {
    compilerOptions { jvmTarget.set(JvmTarget.JVM_17) }
}

dependencies {
    implementation(project(":dsl"))
    implementation(compose.desktop.currentOs)
    implementation(compose.material3)

    testImplementation(libs.junit)
}

val dslDocsDir = rootDir.resolve("../docs/dsl")

// 初期の DSL は docs/dsl/examples をそのまま同梱する(iOS・Android と同じ)。jar の中では examples/<名前>.json。
tasks.processResources {
    from(dslDocsDir.resolve("examples")) {
        include("*.json")
        into("examples")
    }
}

// 適合テストは docs/dsl/conformance/cases(全 OS 共通の期待値)を読む。
tasks.test {
    inputs.dir(dslDocsDir.resolve("conformance")).withPropertyName("conformanceCases").withPathSensitivity(PathSensitivity.RELATIVE)
    inputs.dir(dslDocsDir.resolve("examples")).withPropertyName("dslExamples").withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("metamon.dslDocsDir", dslDocsDir.absolutePath)
}

compose.desktop {
    application {
        mainClass = "jp.kz2601.metamon.desktop.MainKt"
        nativeDistributions {
            // 配布の形(MSIX・AppImage / Flatpak)と署名は後の Issue で決める。今は jpackage の既定の形だけ。
            targetFormats(TargetFormat.Msi, TargetFormat.Deb, TargetFormat.Rpm, TargetFormat.Dmg)
            packageName = "Tailor It"
            packageVersion = "0.1.0"
        }
    }
}
