# デスクトップ版(Linux)を Orca で読む

#2066 の選択肢 B(配布物には橋渡しを入れず、ディストリの java-atk-wrapper に頼る)を取った時の、入れ方・確かめ方・限界。**#2066 はまだ決まっていない**。ここに書いた手順は Linux の実機では試していない(下の「確かめたこと」)。

## 目次

- [要約](#要約)
- [仕組み](#仕組み)
- [配布物(jpackage)が持っている物](#配布物jpackageが持っている物)
- [入れ方](#入れ方)
- [Orca で確かめる](#orca-で確かめる)
- [限界](#限界)
- [B を取る時に別にやること](#b-を取る時に別にやること)
- [確かめたこと](#確かめたこと)

## 要約

- Compose Desktop は読み上げの情報を Java の `javax.accessibility`(Swing と同じ口)に出す。Linux でこれを AT-SPI(Orca が聞く口)へ渡すのが java-atk-wrapper(JAW)。JDK 自体は AT-SPI へ渡さない。
- JetBrains は Compose Desktop の Linux の読み上げを「Not supported」としている。JAW を入れても読めるかは確かめていない。
- **今の配布物(deb / rpm)は、ディストリの JAW を入れるだけでは読めない見込み**。同梱の実行環境に JAW が要る `java.management` が無く、JAW の jar と `.so` も同梱の実行環境からは見えない。B を取るなら、ビルドの小さな変更が別に要る(下の「B を取る時に別にやること」)。

## 仕組み

1. AWT が起動する時、`java.awt.Toolkit` が読み上げの道具(assistive technology)の名前を次の順で探す: system property `javax.accessibility.assistive_technologies` → `~/.accessibility.properties` → `$JAVA_HOME/conf/accessibility.properties`(後の 2 つはファイルの中の `assistive_technologies=` の行。ホームのファイルが空でない時は `$JAVA_HOME` の方を読まない)。
2. 名前が `org.GNOME.Accessibility.AtkWrapper` なら、system class loader から `ServiceLoader` で `javax.accessibility.AccessibilityProvider` を探す。JAW の jar がこれを `org.GNOME.Accessibility.AtkProvider` として出している。見つからなければ同じ名前のクラスを `Class.forName` で読む。どちらも **JAW の jar がアプリの class path(system class loader)に見えている**必要がある。
3. JAW は `System.loadLibrary("atk-wrapper")` で JNI の `libatk-wrapper.so` を読む(`java.library.path` に要る)。
4. JAW は起動時に、ルートウィンドウの `AT_SPI_BUS` か、セッションの D-Bus の `org.a11y.Bus` を見て、読み上げの bus があれば有効になる。また `java.lang.management`(GC の通知)を使うので、実行環境に `java.management` モジュールが要る。
5. `GNOME_ACCESSIBILITY=1` は GTK 2 の頃の環境変数で、今の JAW のソースには出てこない(JAW の有効・無効には効かない)。JAW のログは `JAW_DEBUG=1`〜`4` で出る。

## 配布物(jpackage)が持っている物

`desktop/app/build.gradle.kts` の `nativeDistributions` は jpackage の既定の形(Msi・Deb・Rpm・Dmg)で、jlink で切り出した実行環境を同梱する。

| 項目 | 今 | Orca に要るか |
| --- | --- | --- |
| 同梱の実行環境のモジュール | Compose 1.12.2 の既定 `java.base`・`java.desktop`・`java.logging`・`jdk.crypto.ec` + `modules("jdk.accessibility")` | JAW は `java.management` を要る(JAW の `module-info.java` の `requires java.management`)。**入っていない** |
| JAW の jar | 入れていない。class path はアプリの jar だけ | ディストリの jar(`/usr/share/java/java-atk-wrapper.jar` など)は同梱の実行環境の class path に無い |
| `libatk-wrapper.so` | 入れていない | Debian / Ubuntu は `/usr/lib/<triplet>/jni/`。同梱の実行環境(ビルドに使った JDK から作る)がここを `java.library.path` に入れるかは確かめていない |
| `conf/accessibility.properties` | ビルドに使った JDK の物がそのまま入る(中身は確かめていない) | ホームの `~/.accessibility.properties` で足りる(下の「入れ方」) |
| 起動時の引数・環境変数 | 何も足していない | `-Djavax.accessibility.assistive_technologies=...` か `~/.accessibility.properties` のどちらか |

`./gradlew :desktop:run` は Gradle を動かす JDK で起動する。ディストリの OpenJDK なら `java.management` はあるが、JAW の jar を class path に入れる仕組みは今のビルドに無い。

## 入れ方

### Debian / Ubuntu

```
sudo apt install libatk-wrapper-java libatk-wrapper-java-jni orca
```

- `libatk-wrapper-java`: `/usr/share/java/java-atk-wrapper.jar`
- `libatk-wrapper-java-jni`: `/usr/lib/x86_64-linux-gnu/jni/libatk-wrapper.so`(amd64 の場合)
- Ubuntu 22.04 は 0.38.0、24.04 は 0.40.0、26.04 は 0.44.0

### Fedora

```
sudo dnf install java-atk-wrapper orca
```

- Fedora の版で今も出ているかは確かめていない(packages.fedoraproject.org で見えたのは fc34 の 0.38.0)。RHEL / AlmaLinux 系の README は jar と `.so` を `%{_libdir}/java-atk-wrapper/` に置くと書いている。

### 読み上げの道具の名前を渡す

ホームのファイルで渡す(同梱の実行環境を書き換えなくてよい):

```
printf 'assistive_technologies=org.GNOME.Accessibility.AtkWrapper\n\n' > ~/.accessibility.properties
```

最後の行の後に空行が要る(JAW の `accessibility.properties` の注記)。このファイルは同じユーザーで動く全ての Java アプリに効く。

## Orca で確かめる

GNOME の Wayland / X11 のどちらでも、読み上げの bus はログインのセッションで動いている前提。

1. 設定 → アクセシビリティ → スクリーンリーダーを on(または `Super+Alt+S`、または端末で `orca`)
2. 読み上げの bus があるか: `dbus-send --session --dest=org.a11y.Bus --print-reply /org/a11y/bus org.a11y.Bus.GetAddress` が住所を返す
3. Tailor It を起動する。JAW のログを見たい時は `JAW_DEBUG=1` を付けて起動する
4. `accerciser`(`sudo apt install accerciser` / `sudo dnf install accerciser`)でアプリの木に Tailor It が出るか見る。出なければ JAW が読まれていない
5. Orca で読む物(#1871 で付けた物):
   - 見出し
   - 下のタブ(選択中かどうか)
   - sheet のダイアログの名前
   - 保存の失敗などの知らせ(live region)
   - グラフ・進み具合・メーター・星・画像の説明
6. Tab で移って、フォーカスした部品の名前と役割を読むか

## 限界

- JetBrains の文書は Compose Desktop の Linux の読み上げを「Not supported」としている。JAW は Swing の部品を前提に作られていて、Compose の部品(1 枚の描画の中にある)の情報をどこまで AT-SPI に渡せるかは確かめていない。
- JAW の README は「openjdk-8 で動くことが分かっている。新しい版では読めないことがある」と書いている(JDK-8204862)。このアプリは JDK 17 以上。
- `~/.accessibility.properties` は同じユーザーの他の Java アプリにも効く。読み上げの bus が無い時は JAW は何もしない(上の「仕組み」4)。
- 配布の形(Flatpak / AppImage・#1756)は sandbox の中から外の jar・`.so` が見えないので、B の「OS の物に頼る」は deb / rpm の時だけ成り立つ。

## B を取る時に別にやること

このブランチではコードを変えていない。B に決まったら、次を別の PR で入れる(どれも Linux の実機で確かめてから):

1. `nativeDistributions` の `modules(...)` に `java.management` を足す(JAW が要る。実行環境が少し大きくなる)
2. 配布物の起動で、ディストリの JAW を見えるようにする。案:
   - `jvmArgs` で `-Djava.library.path=` に Debian / Ubuntu の `/usr/lib/<triplet>/jni` と Fedora の `/usr/lib64/java-atk-wrapper` を足す
   - jar を class path に入れる。jpackage の class path はアプリのフォルダの中が前提なので、起動の時に `/usr/share/java/java-atk-wrapper.jar` を読ませる方法(独自の class loader で読み `AccessibilityProvider` を自分で起こす・deb の postinst でリンクを張る、など)を選ぶ
3. `./gradlew :desktop:run` で確かめられるよう、Linux の時だけ `run` の class path に JAW の jar を足す

2 が重くなるなら、選択肢 A(配布物に JAW を同梱する)と手間が変わらない。

## 確かめたこと

| 項目 | 確かめ方 | 結果 |
| --- | --- | --- |
| 同梱の実行環境の既定のモジュール | compose-gradle-plugin 1.12.2 の `JvmApplicationDistributionsKt` を javap | `java.base`・`java.desktop`・`java.logging`・`jdk.crypto.ec` |
| JAW が `java.management` を要る | JAW のソース(GNOME の GitLab・2026-06-29 の commit)の `module-info.java` と `AtkWrapper.java.in` | 要る |
| JAW の有効の条件・`GNOME_ACCESSIBILITY` | 同じソースを grep | `AT_SPI_BUS` か `org.a11y.Bus`。`GNOME_ACCESSIBILITY` は出てこない |
| JDK の探し方 | OpenJDK 17u の `Toolkit.java` | 上の「仕組み」1・2 のとおり |
| Debian / Ubuntu のファイルの場所・版 | packages.debian.org・packages.ubuntu.com | 上の「入れ方」のとおり |
| Linux の実機で Orca が読むか | **確かめていない**(Mac しか無い) | 未確認 |
| 同梱の実行環境の `java.library.path`・`conf/accessibility.properties` の中身 | **確かめていない** | 未確認 |
| Fedora の今の版に java-atk-wrapper があるか | **確かめていない** | 未確認 |
