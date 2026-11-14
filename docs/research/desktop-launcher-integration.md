# Windows のジャンプリスト・GNOME の検索プロバイダ・.desktop の Actions・KRunner の調査メモ

> 調べた日: 2026-11-13。親: #1818 #1817。調査だけで、決めない。コード・正典(`docs/00_decisions.md`・`docs/decisions/*`)は変えていない。
> 「読んだ」はページを開いて読んだ内容、「推測」「未確認」は読んでいない所。iOS のホーム画面の長押し・Spotlight の代わりを、デスクトップ(Windows・Linux)でどこまで作れるかを並べる。

## 目次

1. [前提](#前提)
2. [OS ごとの表](#os-ごとの表)
3. [Windows のジャンプリスト](#windows-のジャンプリスト)
4. [GNOME の検索プロバイダ](#gnome-の検索プロバイダ)
5. [.desktop の Actions](#desktop-の-actions)
6. [KDE の KRunner](#kde-の-krunner)
7. [決めること](#決めること)
8. [出典](#出典)

## 前提

- デスクトップ版は Compose Multiplatform Desktop(JVM)で、Windows の API は JNA で呼んでいる(`desktop/app/build.gradle.kts` の `net.java.dev.jna:jna:5.19.1`。コメントに「jna-platform は足さない」とある。約 1.9 MB を避けるため)。
- 配布は今、jpackage の既定の形(`Msi`・`Deb`・`Rpm`・`Dmg`)。MSIX・AppImage・Flatpak は「後の Issue で決める」と同じファイルに書いてある。
- トレイの常駐(開く・すぐ記録)は既にある([desktop/README.md](../../desktop/README.md))。この調査は、タスクバー・ランチャー・検索から「作ったアプリを開く・すぐ記録」へ入る口の話。

## OS ごとの表

| OS・仕組み | 出来ること | 手間(推測) | MSIX などのパッケージが要るか |
| --- | --- | --- | --- |
| GNOME の検索プロバイダ | Shell の概要画面の検索に、アプリの結果を出す(アプリごとに 3 件まで) | 中。D-Bus のサービスを作る | 要らない。ini と `.desktop` を決まった場所に置く。入れ物は deb・rpm なら入れやすい(推測) |
| KDE の KRunner | KRunner の検索に結果・操作を出す | 中〜大(読んだ範囲は C++ のプラグイン。D-Bus の方式は未確認) | 要らない(推測) |
| Windows のジャンプリスト(`ICustomDestinationList`) | タスクバーのボタンの右クリックに、固定の「タスク」と、最近・よく使う・自分の項目を出す | 中〜大。JNA で COM を呼ぶ(下を読む) | 要らない(読んだ: Windows 7 以降のデスクトップアプリ向け) |
| `.desktop` の Actions | ランチャーの右クリックに、アプリごとの起動の操作を出す | 小。ファイルに書くだけ | 要らない。deb・rpm に同梱する |

## Windows のジャンプリスト

読んだこと(出典 1・2):

- `ICustomDestinationList` は Windows が実装している物で、アプリは `BeginList` → `AppendCategory` / `AddUserTasks` → `CommitList` の順で呼ぶ。更新は「差し替え」。システムからアプリへ問い合わせることはない。
- 「タスク」はアプリの固定の操作(`IShellLink`)で、ユーザーは外せない。「目的地」は最近・よく使う・自分の分類で、ユーザーが外せる。外された物は守る必要がある。
- 自分で `AppUserModelID` を決めた時は、ジャンプリストの呼び出し・ショートカット・ウィンドウ・プロセスに **同じ ID** を付ける。`SetCurrentProcessExplicitAppUserModelID` は最初の画面を出す前に呼ぶ。
- Java のランタイムは、同じページで「ホストのプロセス」の例に挙がっている(読んだ)。ID を付けないと、ウィンドウが 1 つのアプリとして束ねられない恐れがある(推測)。
- パッケージ(MSIX)は必須とは書かれていない。ショートカットに ID を付ける所は MSI の `MsiShortcutProperty` で出来る(読んだ)。

分からないこと(未確認):

- JNA の COM で `ICustomDestinationList` を呼べるか。`jna-platform` の COM の枠を使うか、vtable を自分で書くことになる(推測)。いずれも `jna-platform` を足すか、自前で書くかで大きさと手間が変わる。**試して確かめる**。
- jpackage が作る exe のショートカットが `AppUserModelID` を持つか。MSI に付ける手段があるか。
- MSIX にした時の挙動(パッケージの ID が別に付く)。未確認。

## GNOME の検索プロバイダ

読んだこと(出典 3):

- `$(datadir)/gnome-shell/search-providers/` に ini(`DesktopId`・`BusName`・`ObjectPath`・`Version=2`)を置く。反映は Shell の再起動のあと。
- D-Bus の `org.gnome.Shell.SearchProvider2`(`GetInitialResultSet`・`GetSubsearchResultSet`・`GetResultMetas`・`ActivateResult`・`LaunchSearch`)を実装する。
- 検索しただけで画面を出してはいけない(サービスとして起動して、`ActivateResult` で窓を開く)。
- `DesktopId` の `.desktop` が要る(ページは明言していない)。

手間: JVM のアプリは D-Bus を話すライブラリを足し、検索のたびに起動する常駐のサービスが要る(推測)。DSL の記録・アプリの一覧を軽く引く口が要る。

## .desktop の Actions

読んだこと(出典 4):

- `Actions` に識別子を並べ、`[Desktop Action <識別子>]` の組に `Name`(必須)・`Icon`・`Exec` を書く。ランチャーが右クリックの項目などに出す(Quicklist・Jumplist と書いてある)。
- 対応しないランチャーは無視する。インストーラは出さないので、汎用の起動の操作だけにする。

手間: 小。`Exec` に「このアプリを開く」「すぐ記録する」の引数を渡す所を作れば足りる(推測)。トレイと同じ口を使える(推測)。生成した個々のアプリ(動的)は、静的なファイルには書けない(推測。一覧を変えるたびにファイルを書き直すことになる)。

## KDE の KRunner

読んだこと(出典 5): C++ の `AbstractRunner` を継承するプラグインの書き方だけ。D-Bus で書く方式があるかは、見つけたページが 404 で未確認(推測では、言語を選ばない D-Bus の方式がある)。

## 決めること

| 番号 | 決めること | 案 |
| --- | --- | --- |
| 1 | 最初に作る物 | `.desktop` の Actions(小)/ Windows のジャンプリスト / 検索プロバイダ |
| 2 | Windows の `AppUserModelID` の付け方と、`jna-platform` を足すか | 足さず vtable を自前 / 足す |
| 3 | 生成した個々のアプリを出すか(動的) | 出さない(開く・すぐ記録だけ)/ 検索プロバイダ・ジャンプリストの目的地に出す |
| 4 | KDE を対象にするか | しない / 後で |

## 出典

1. Microsoft Learn: ICustomDestinationList <https://learn.microsoft.com/en-us/windows/win32/api/shobjidl_core/nn-shobjidl_core-icustomdestinationlist>(読んだ)
2. Microsoft Learn: AppUserModelID <https://learn.microsoft.com/en-us/windows/win32/shell/appids>(読んだ)
3. GNOME Developer: 検索プロバイダ <https://developer.gnome.org/documentation/tutorials/search-provider.html>(読んだ)
4. freedesktop.org: Desktop Entry Specification の Actions <https://specifications.freedesktop.org/desktop-entry/latest/extra-actions.html>(読んだ)
5. KDE Developer: KRunner <https://develop.kde.org/docs/plasma/krunner/>(読んだ・C++ のプラグインの部分だけ)
