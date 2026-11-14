# デスクトップ版の配布物の作り方

#2305(親 #1755)。Windows・Linux 版(`desktop/`)の配布物を手元で作る・入れる・消す手順と、まだ決まっていないこと。アプリの中身は [desktop/README.md](../../desktop/README.md)、CI は [desktop-ci.yml](../../.github/workflows/desktop-ci.yml)。

- 書いた元: `origin/epic/god`(7d96d09c2)の [desktop/app/build.gradle.kts](../../desktop/app/build.gradle.kts) の `compose.desktop { nativeDistributions }`・`desktop/whisper/native/`・`desktop-ci.yml`。
- **推測** と書いた所は、コードに無く Compose / jpackage の既定の動きから書いたもの。手元でなぞって確かめたら印を外す。
- 金額・署名の鍵などの秘密は書かない。

## 目次

1. [今の設定](#1-今の設定)
2. [前提](#2-前提)
3. [OS ごとの作り方](#3-os-ごとの作り方)
4. [できる物の場所](#4-できる物の場所)
5. [入れる・消す](#5-入れる消す)
6. [whisper のライブラリを入れる](#6-whisper-のライブラリを入れる)
7. [配布物の大きさを測る](#7-配布物の大きさを測る)
8. [CI で作る物](#8-ci-で作る物)
9. [決まっていないこと](#9-決まっていないこと)
10. [人の作業](#10-人の作業)

## 1. 今の設定

| 項目 | 値(`build.gradle.kts`) |
|---|---|
| 形 | `TargetFormat.Msi`・`Deb`・`Rpm`・`Dmg`(jpackage の既定の形だけ。MSIX・AppImage・Flatpak は無い) |
| 名前 | `packageName = "Tailor It"` |
| 版 | `packageVersion = "0.1.0"`。アプリが申告する版 `DesktopApi.VERSION` と手で合わせる(`net/DesktopApi.kt`) |
| 同梱の実行環境 | jpackage が JDK から作る。`modules("jdk.accessibility")` で読み上げ(Java Access Bridge)を足す |
| 同梱の資源 | `appResourcesRootDir` = `desktop/whisper/native/out`(whisper の JNI のライブラリ。作っていなければ入らない) |
| 署名 | しない |
| 起動の判定 | 配った形では `jpackage.app-path` が付く。「ログイン時に開く」はこれがある時だけ出る(`Autostart.kt`) |

## 2. 前提

- JDK 17 以上で、`jpackage` が入っている物(Temurin など)。CI は Temurin 17。
- 作る OS と同じ OS で作る(jpackage はクロスビルドしない)。
- Linux の deb は `dpkg-deb`・`fakeroot`、rpm は `rpmbuild` が要る(**推測**: jpackage の要件。Ubuntu の CI には `rpmbuild` が無いので rpm は作っていない)。
- Windows の msi は WiX Toolset が要る(**推測**: Compose の Gradle プラグインが初回に取得する)。

## 3. OS ごとの作り方

`desktop/` で動かす。

| OS | コマンド | できる物 |
|---|---|---|
| Linux(Ubuntu など) | `./gradlew :desktop:packageDeb` | `.deb` |
| Linux(Fedora など) | `./gradlew :desktop:packageRpm` | `.rpm` |
| Windows | `.\gradlew.bat :desktop:packageMsi` | `.msi` |
| macOS | `./gradlew :desktop:packageDmg` | `.dmg`(Mac は SwiftUI の Mac 版が受け持つので配らない。下の 9 章) |
| どれでも | `./gradlew :desktop:packageDistributionForCurrentOS` | 今の OS で作れる形を全部 |
| どれでも | `./gradlew :desktop:createDistributable` | 入れずに動く形(フォルダ)。`./gradlew :desktop:runDistributable` で起動 |

- 入れる前の確かめは `createDistributable` → `runDistributable` が速い。`jpackage.app-path` が付くので「ログイン時に開く」も出る(**推測**)。
- 音声入力も入れるなら、先に 6 章でライブラリを作る。

## 4. できる物の場所

`desktop/app/build/compose/binaries/main/` の下。CI が残すのもここ(`desktop-ci.yml` の「配布物を残す」)。

| 形 | 場所 |
|---|---|
| deb | `.../main/deb/` |
| rpm | `.../main/rpm/` |
| msi | `.../main/msi/` |
| dmg | `.../main/dmg/` |
| 入れずに動く形 | `.../main/app/Tailor It/`(**推測**) |

## 5. 入れる・消す

ファイル名・パッケージ名は jpackage が `packageName` から作る。下の名前は **推測**(`tailor-it`)。作った後に `dpkg-deb -I <deb>` / `rpm -qip <rpm>` で本当の名前を見て直す。

| OS | 入れる | 消す |
|---|---|---|
| Ubuntu | `sudo apt install ./<ファイル>.deb` | `sudo apt remove tailor-it` |
| Fedora | `sudo dnf install ./<ファイル>.rpm` | `sudo dnf remove tailor-it` |
| Windows | `.msi` を開く(署名が無いので SmartScreen の警告が出る) | 設定 > アプリ > インストールされているアプリ > Tailor It > アンインストール |

- 入る場所(**推測**): Linux は `/opt/tailor-it/`、Windows は `C:\Program Files\Tailor It\`。
- 消しても端末の保存先は残る(パッケージの外に書くため): Windows `%APPDATA%\TailorIt`・Linux `$XDG_DATA_HOME/tailorit`(無ければ `~/.local/share/tailorit`)。消すならアプリの「この端末のデータを消す」を先に使うか、フォルダを手で消す。whisper のモデル(`models/`)もここにある。
- 「ログイン時に開く」を on にしたまま消すと、ログインの登録(Windows の `HKCU\...\Run` の `TailorIt`・Linux の `~/.config/autostart/tailorit.desktop`)が残る(**推測**: パッケージの消す処理は触らない)。消す前に off にする。

## 6. whisper のライブラリを入れる

音声入力(`TAILORIT_VOICE=1` の時だけ使う)の JNI のライブラリは git に入れず、作る機械で作ってから配布物を作る。

1. Linux: `desktop/whisper/native/build.sh`。Windows(PowerShell・Visual Studio 2022 の C++): `desktop\whisper\native\build.ps1 [-Arch x64|arm64]`。
2. `desktop/whisper/native/out/<os>-<arch>/` に `libmetamon_whisper.so` / `metamon_whisper.dll` ができる。
3. 3 章のコマンドで作ると、配布物の中に入る。起動した時は `compose.application.resources.dir` から読む(`NativeLibrary.kt`)。
4. 無ければ音声入力は「この PC では用意できていません」になる。CI はライブラリを作らないので、CI の配布物には入っていない。

- モデル(`ggml-base-q5_1.bin`)は配布物に入れない。初回に端末の保存先の `models/` へ落とす。
- 1 つの配布物に入るのは、作った機械の `<os>-<arch>` の分だけ。x64 と arm64 は別々に作る。

## 7. 配布物の大きさを測る

```
du -h desktop/app/build/compose/binaries/main/*/*
du -sh "desktop/app/build/compose/binaries/main/app/Tailor It"
```

- 中身の内訳は 入れずに動く形 で見る: 実行環境(`runtime/`)・アプリの jar(`app/`)・whisper のライブラリ。
- 大きさに効く依存は [build.gradle.kts](../../desktop/app/build.gradle.kts) のコメントに書いてある(PDFBox・JNA など)。
- 測った値は PR に書く(この文書には書かない。版で変わるため)。

## 8. CI で作る物

| 起きた時 | テスト | 配布物 |
|---|---|---|
| PR | ubuntu・windows | 作らない |
| develop・main への push | ubuntu・windows | deb(ubuntu)・msi(windows)を作り、Actions の artifact に 7 日残す |
| 手動(workflow_dispatch) | 必ず | 同上 |

- 署名しない・秘密を使わない。rpm・dmg は作らない。
- 回す条件の詳細は `desktop-ci.yml` の頭の表。

## 9. 決まっていないこと

推奨は各 Issue の「やること」に書かれた物。決めるのはオーナー。

| 論点 | Issue | 選択肢 | 推奨 / 状態 |
|---|---|---|---|
| Windows の配り方 | [#1755](https://github.com/jphacks/kz_2601/issues/1755) | Microsoft Store / 自前で配る | 未確定 |
| Windows の形 | #1755 | MSI(今)/ MSIX | 未確定。Store・トースト通知・自動更新には MSIX が向く(#1755 の背景)。jpackage の出力を MSIX にする手順はまだ無い |
| Windows の署名 | #1755 | OV / EV の証明書・Azure Trusted Signing | 未確定。費用・本人確認は人の作業 |
| Windows の CPU | #1755 | x64 のみ / x64 + arm64 | 未確定。whisper の `build.ps1` は両方作れる |
| Linux の主な形 | [#1756](https://github.com/jphacks/kz_2601/issues/1756) | Flatpak(Flathub)・AppImage・deb / rpm | 推奨 Flatpak + AppImage(#1756)。未確定 |
| Flatpak の権限 | #1756 | ネットワーク・マイク・通知・ファイル選択のポータル | マニフェストはまだ無い。保存先は `XDG_DATA_HOME` に従うので Flatpak の専用の場所に入る(`AppDataDir.kt`) |
| AppImage | #1756 | 作る / 作らない | 未確定。Compose の `TargetFormat` には AppImage があるが、出るのはフォルダで `.AppImage` の 1 ファイルにはしない(**推測**)。別の道具で包む |
| 自動更新 | [#1758](https://github.com/jphacks/kz_2601/issues/1758) | Store・Flathub は OS に任せる / 自前の MSIX は App Installer / AppImage は zsync / アプリの中で「新しい版があります」 | 未確定。最低の版を appconfig に置く案(iOS の強制更新と同じ考え) |
| 版の上げ方 | #1758 | `packageVersion` と `DesktopApi.VERSION` を手で合わせる(今)/ 1 か所から作る | 決めること。Windows で上書きして入れるには、版を上げることと `upgradeUuid` を固定することが要る(**推測**: 今は設定していない) |
| macOS の dmg | [#363](../decisions/363-desktop-strategy.md) | 作る / 形から外す | Mac は SwiftUI の Mac 版と決まっている(#363)。`Dmg` を `targetFormats` に残すかは決めること。jpackage の dmg は版の頭が 0 だと作れない(**推測**: `0.1.0` では落ちる) |

## 10. 人の作業

- [ ] 手元の Linux(Ubuntu)で 3〜5 章をなぞり、コマンドが通ることと **推測** の所(パッケージ名・入る場所)を確かめて印を外す。
- [ ] Windows でも同じ(msi・SmartScreen の警告の出方)。
- [ ] 9 章の表を #1755・#1756・#1758 で決める。
