# whisper.cpp(MIT)を取り込み、JNI の口(libmetamon_whisper.so)を 1 つ作る。
# 版はここで固定する(タグの tar.gz と SHA-256)。上げる時は両方を変え、ライセンスの一覧(android/scripts/third-party-licenses.py)の版も直す。
cmake_minimum_required(VERSION 3.22.1)
project(metamon_whisper LANGUAGES C CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

set(WHISPER_CPP_VERSION 1.9.5)
set(WHISPER_CPP_SHA256 ff1a9053feb509ff9d7729703355541ae9690073a6b1c40eb692c962e0dc1720)

include(FetchContent)
FetchContent_Declare(
    whisper_cpp
    URL https://github.com/ggml-org/whisper.cpp/archive/refs/tags/v${WHISPER_CPP_VERSION}.tar.gz
    URL_HASH SHA256=${WHISPER_CPP_SHA256}
)

# 静的に 1 つの .so へまとめる。CPU だけ(GPU・OpenMP・例・テストは入れない)
set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
set(WHISPER_BUILD_TESTS OFF CACHE BOOL "" FORCE)
set(WHISPER_BUILD_EXAMPLES OFF CACHE BOOL "" FORCE)
set(WHISPER_BUILD_SERVER OFF CACHE BOOL "" FORCE)
set(WHISPER_ALL_WARNINGS OFF CACHE BOOL "" FORCE)
set(GGML_NATIVE OFF CACHE BOOL "" FORCE)
set(GGML_OPENMP OFF CACHE BOOL "" FORCE)
set(GGML_BUILD_TESTS OFF CACHE BOOL "" FORCE)
set(GGML_BUILD_EXAMPLES OFF CACHE BOOL "" FORCE)

# debug の版でも認識は最適化して作る(-O0 だと 1 区切りに数分かかる)
if (CMAKE_BUILD_TYPE STREQUAL "Debug")
    set(CMAKE_C_FLAGS_DEBUG "-O2")
    set(CMAKE_CXX_FLAGS_DEBUG "-O2")
endif ()

FetchContent_MakeAvailable(whisper_cpp)

add_library(metamon_whisper SHARED whisper_jni.cpp)
target_link_libraries(metamon_whisper PRIVATE whisper log)
target_compile_options(metamon_whisper PRIVATE -fvisibility=hidden -ffunction-sections -fdata-sections)
target_link_options(metamon_whisper PRIVATE -Wl,--gc-sections -Wl,--exclude-libs,ALL -Wl,-z,max-page-size=16384)
