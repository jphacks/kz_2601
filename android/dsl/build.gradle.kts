import org.jetbrains.kotlin.gradle.dsl.JvmTarget

// :dsl — 純 Kotlin(Android に依存しない)。iOS の MetamonDSL に当たる。JVM の単体テストで回す。
plugins {
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.serialization)
}

// Android のモジュールと同じ Java 17 のバイトコードにそろえる(ビルドする JDK は問わない)。
java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

kotlin {
    compilerOptions { jvmTarget.set(JvmTarget.JVM_17) }
}

dependencies {
    api(libs.kotlinx.serialization.json)
    testImplementation(libs.junit)
}

// テストは docs/dsl/examples/*.json(正典の例)をそのまま読む。
val dslExamplesDir = rootDir.resolve("../docs/dsl/examples")
// ExprConformanceTest は ../conformance/expr(計算式の共有ケース)を読む。入力に入れないと変えても up-to-date で回らない(#1017)。
val exprConformanceDir = rootDir.resolve("../docs/dsl/conformance/expr")

tasks.test {
    inputs.dir(dslExamplesDir).withPropertyName("dslExamples").withPathSensitivity(PathSensitivity.RELATIVE)
    inputs.dir(exprConformanceDir).withPropertyName("exprConformance").withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("metamon.dslExamplesDir", dslExamplesDir.absolutePath)
}
