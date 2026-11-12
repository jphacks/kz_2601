// 版は ../android/gradle/libs.versions.toml(+ settings.gradle.kts で足した分)。ここでは読み込むだけ。
plugins {
    alias(libs.plugins.kotlin.jvm) apply false
    alias(libs.plugins.kotlin.compose) apply false
    alias(libs.plugins.kotlin.serialization) apply false
    alias(libs.plugins.compose.multiplatform) apply false
}

// :dsl のソースは android/dsl と同じだが、出力は Android のビルドと混ぜない(同時に回しても android/dsl/build を取り合わない)。
project(":dsl") {
    layout.buildDirectory.set(rootProject.layout.buildDirectory.dir("dsl"))
}
