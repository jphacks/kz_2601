// Android アプリ(#354・#356)。モジュールは iOS の Metamon{DSL,Renderer,Services} + アプリ本体と 1 対 1。
pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "metamon-android"

include(":app")
include(":dsl")
include(":renderer")
include(":services")
include(":whisper")
