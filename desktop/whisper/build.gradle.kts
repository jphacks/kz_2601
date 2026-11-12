import org.jetbrains.kotlin.gradle.dsl.JvmTarget

// :whisper — 端末内の文字起こし(whisper.cpp を JNI で呼ぶ・#1751)。android/whisper の純 Kotlin の部分を同じソースのまま JVM でビルドする。
// android/ は変えない。Android の API を使うファイル(AudioFileDecoder・WhisperTranscriber)と .so の読み方(WhisperNative)は含めず、
// WhisperNative だけデスクトップの読み方(src/main/kotlin)にする。JNI の関数の名前と引数は Android と同じ。
// ネイティブの部分は native/(build.sh・build.ps1 で out/<os>-<arch>/ に作る。git には入れない)。
plugins {
    alias(libs.plugins.kotlin.jvm)
}

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

kotlin {
    compilerOptions { jvmTarget.set(JvmTarget.JVM_17) }
}

val sharedWhisper = rootDir.resolve("../android/whisper/src")

// 共有するファイルだけをビルド時に写す(app の syncSharedRenderer と同じやり方)。WhisperNative はデスクトップの物(src/main/kotlin)を使う
val syncSharedWhisper by tasks.registering(Sync::class) {
    from(sharedWhisper.resolve("main/kotlin")) {
        include(
            "jp/kz2601/metamon/whisper/AudioBlocks.kt",
            "jp/kz2601/metamon/whisper/IdleReleasing.kt",
            "jp/kz2601/metamon/whisper/WhisperEngine.kt",
            "jp/kz2601/metamon/whisper/WhisperModelManager.kt",
            "jp/kz2601/metamon/whisper/WhisperModels.kt",
        )
    }
    into(layout.buildDirectory.dir("generated/sharedWhisper"))
}

kotlin.sourceSets.named("main") { kotlin.srcDir(syncSharedWhisper) }
// Android と同じテスト(音の区切り・モデルの取得・手放す時計)をそのまま回す
kotlin.sourceSets.named("test") { kotlin.srcDir(sharedWhisper.resolve("test/kotlin")) }

dependencies {
    api(libs.kotlinx.coroutines.core)
    implementation(libs.okhttp)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
}

/** 手元で作ったライブラリ(native/out/<os>-<arch>/)。 */
val nativeOutDir = layout.projectDirectory.dir("native/out")

// 手元の確かめ: ./gradlew :whisper:transcribeWav --args="<16kHz などの wav> [ja|en]"(ライブラリは native/build.sh で作っておく。モデルは無ければ落とす)
tasks.register<JavaExec>("transcribeWav") {
    group = "verification"
    description = "wav を whisper.cpp で文字にして、かかった時間を出す(手元の確かめ用・#1751)"
    classpath = sourceSets["main"].runtimeClasspath
    mainClass.set("jp.kz2601.metamon.whisper.desktop.TranscribeWavKt")
    systemProperty("tailorit.whisper.nativeDir", nativeOutDir.asFile.absolutePath)
    systemProperty("tailorit.whisper.modelDir", layout.buildDirectory.dir("whisper-models").get().asFile.absolutePath)
}
