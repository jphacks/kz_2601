# デスクトップ版を Windows の Narrator で確かめる

#2321(親 #2068)。Windows で、デスクトップ版(`desktop/`)の主な画面を Narrator で読めるかを人が確かめる時の手順と表。確かめた結果を比べられるように、何をどう見るかを先に決めておく。

- 書いた元: `origin/epic/god`(7d96d09c2)の `desktop/app/src/main/kotlin/jp/kz2601/metamon/desktop/` の読み上げの指定(`semantics`・`contentDescription`・`liveRegion`・`heading`・`paneTitle`)。
- **推測** と書いた所は、コードから「こう読まれるはず」と考えたこと。実機で確かめたら結果の欄に書く。
- Linux(Orca)は対象外。読み上げに対応するかは [#2066](https://github.com/jphacks/kz_2601/issues/2066) の決定待ち(Compose Desktop が Linux の読み上げに対応していないため。[desktop/README.md](../../desktop/README.md) の「読み上げ」)。

## 目次

1. [前提](#1-前提)
2. [起動](#2-起動)
3. [Narrator の操作](#3-narrator-の操作)
4. [確認の表](#4-確認の表)
5. [今の作りで名前を付けている所・付けていない所](#5-今の作りで名前を付けている所付けていない所)
6. [結果の書き方](#6-結果の書き方)

## 1. 前提

- Windows 10 / 11 の実機か VM。
- Java Access Bridge を有効にする。Compose Desktop は Java Access Bridge を通して Narrator に読ませる。配った形(msi)の実行環境には `jdk.accessibility` を入れてある([desktop/app/build.gradle.kts](../../desktop/app/build.gradle.kts) の `modules("jdk.accessibility")`)。
  - 手元の JDK で動かす時: その JDK の `bin\jabswitch.exe -enable` を 1 回実行し、サインアウト・サインインし直す(**推測**: 有効化は次のサインインから効く)。
  - msi で入れた時: 同梱の実行環境に `jabswitch.exe` が無ければ、手元の JDK の `jabswitch -enable` で同じ設定になる(**推測**: 設定は利用者ごとの 1 か所)。
- Narrator で読めない時は NVDA でも試す(#2068 のやること)。

## 2. 起動

`desktop/` で:

```
.\gradlew.bat :desktop:run                      # 同梱の例 shopping
.\gradlew.bat :desktop:run --args=kakeibo       # stat・グラフのある例
```

- msi で入れたなら、スタートメニューの Tailor It。msi は `.\gradlew.bat :desktop:packageMsi` で作る(`desktop-ci.yml` と同じ)。
- 英語の読み上げも見る時は `set TAILORIT_LANG=en` の後に起動する。

## 3. Narrator の操作

| したいこと | キー |
|---|---|
| Narrator を始める / 止める | Ctrl + Win + Enter |
| 次 / 前の項目 | Narrator キー(Caps Lock か Insert)+ → / ← |
| 今の項目を読む | Narrator キー + Tab |
| 見出しで移る | Narrator キー + H(スキャンモードの時は H) |
| スキャンモードの切り替え | Narrator キー + Space |
| 読み上げを止める | Ctrl |
| アプリの中で次の部品へ | Tab / Shift + Tab(アプリのフォーカスの順で動く) |

## 4. 確認の表

期待は「何が読まれるべきか」。結果は空欄のまま渡す(人が「読めた / 読めない / 違う文」と、読まれた文を書く)。

### 4.1 名前が読まれる

| 画面 | 何を | 期待 | 関係する部品・ファイル | 結果 |
|---|---|---|---|---|
| 一覧 | 画面の見出し | 画面の題(無ければアプリ名)が「見出し」として読まれる | `DesktopApp.kt` の見出しの `Text`(`semantics { heading() }`) | |
| 一覧 | 行 | 行の文字が読まれる | `DataLeaves.kt` の list・table の行 | |
| 一覧 | 「表をコピー」などのボタン | ボタンの文字と「ボタン」 | Material の `TextButton` など(既定) | |
| 一覧 | stat・グラフ・ゲージ・進み具合 | 「ラベル 値」(グラフは「題 グラフ: 項目 値、…」)が 1 つで読まれる | `A11y.kt` の `describe`・`leafSemantics`(`DesktopApp.kt` の `Node`) | |
| 一覧 | image・avatar・label 付きの icon | label(無ければ「画像」など) | `A11y.kt`・`IconLeaf.kt` | |
| フォーム | 文字の入力 | 項目名(単位があれば付く) | `InputLeaves.kt` の `OutlinedTextField` の `label` | |
| フォーム | toggle・slider・stepper | 項目名と部品 | `InputLeaves.kt`。項目名は横の `Text` で、部品に結び付けていない(5 章) | |
| フォーム | picker | 選択肢の文字 | `InputLeaves.kt` の `FilterChip` | |
| フォーム | 保存ボタン | ボタンの文字 | `DesktopApp.kt`・`Actions.kt` | |
| 詳細 | 写真のプレビュー | 「写真のプレビュー: 項目名」がダイアログの名前として読まれる | `PhotoPreview.kt`(`paneTitle`・`A11y.previewTitle`) | |
| 詳細 | 別のウィンドウの詳細 | ウィンドウの題が読まれる | `Main.kt` の `Window`・`Windows.kt` | |
| ダイアログ | sheet・fullScreen | 画面の題(無ければアプリ名)がダイアログの名前として読まれる | `DesktopApp.kt`(`paneTitle`・`isTraversalGroup`) | |
| ダイアログ | 暗幕 | 「閉じる」ボタン | `DesktopApp.kt`(`contentDescription = closeLabel`) | |
| ダイアログ | 削除の確認 | 確認の文・「削除」「キャンセル」 | `DesktopApp.kt` の `AlertDialog`(`confirmMessage`) | |
| コマンドパレット | 入力欄 | 案内の文(placeholder) | `CommandPalette.kt` の `OutlinedTextField` | |
| コマンドパレット | 候補 | 種類・名前・ショートカットが読まれる | `CommandPalette.kt` の `LazyColumn` の行 | |
| ショートカットの一覧(F1) | 中身 | キーと説明 | `CommandPalette.kt` の `ShortcutHelp`(`AlertDialog`) | |
| トレイのメニュー | 各項目 | 「開く」「すぐ記録」「終了」など項目の文字 | `Tray.kt`(OS のメニュー。Compose の外) | |
| トレイのメニュー | アイコン | ツールチップ(アプリ名・タイマー) | `Tray.kt` の `TrayMenu.tooltip` | |

### 4.2 状態が読まれる

| 画面 | 何を | 期待 | 関係する部品・ファイル | 結果 |
|---|---|---|---|---|
| 一覧 | タブ | 「タブ」と「選択中」 | `DesktopApp.kt` の `selectableGroup`・`selectable(role = Role.Tab)` | |
| 一覧 | 選んだ行 | 「選択中」 | `DataLeaves.kt`(`semantics { selected = isSelected }`) | |
| 一覧 | チェックの付いた行 | チェックの on / off | `DataLeaves.kt` の list のチェック | |
| フォーム | toggle | on / off | `InputLeaves.kt` の `Switch` | |
| フォーム | slider | 今の値 | `InputLeaves.kt` の `Slider` | |
| フォーム | picker | 選んだ選択肢が「選択中」 | `FilterChip(selected = …)` | |
| フォーム | 入力の誤り | 誤りがあること | `OutlinedTextField(isError = …)` | |
| コマンドパレット | 矢印で選んだ候補 | 選んでいることが読まれる | `CommandPalette.kt`。色だけで、読み上げの状態は付けていない(5 章) | |
| トレイのメニュー | 「ログイン時に開く」「ウィンドウを閉じてもトレイに残す」 | チェックの on / off | `Tray.kt` の `CheckboxItem` | |

### 4.3 知らせが読まれる

| 画面 | 何を | 期待 | 関係する部品・ファイル | 結果 |
|---|---|---|---|---|
| 起動時 | 読み込み中 | 読み込み中の文が 1 回読まれる | `LoadingSkeleton.kt`(`liveRegion = Polite`) | |
| フォーム | 保存の失敗などのエラー | 出た時に読まれる | `DesktopApp.kt` の `session.message`(`liveRegion`) | |
| 一覧 | 取り込み・書き出しの知らせ | 出た時に読まれる | `DesktopApp.kt` の `io.notice`(`liveRegion`) | |
| 一覧 | 送り待ちの件数 | 件数が変わった時に読まれる | `DesktopApp.kt` の `desktop_sync_pending`(`liveRegion`) | |
| どこでも | 通知(scheduleNotification・タイマーの終わり) | Windows の通知として読まれる | `Tray.kt` の `sendNotification`・`Actions.kt`(OS の通知。Compose の外) | |

## 5. 今の作りで名前を付けている所・付けていない所

### 付けている

| 所 | 付け方 | ファイル |
|---|---|---|
| 画面の見出し・写真のプレビューの見出し | `heading()` | `DesktopApp.kt`・`PhotoPreview.kt` |
| sheet・fullScreen・写真のプレビューのダイアログ | `paneTitle`・`isTraversalGroup` | `DesktopApp.kt`・`PhotoPreview.kt` |
| モーダルの間の下の画面 | 読み上げから外す(`clearAndSetSemantics`) | `Keyboard.kt` の `inert` |
| 暗幕の「閉じる」 | `contentDescription` | `DesktopApp.kt`・`PhotoPreview.kt` |
| タブ | `Role.Tab`・`selectable` | `DesktopApp.kt` |
| list・table の選んだ行 | `selected` | `DataLeaves.kt` |
| 知らせ(エラー・入出力・送り待ち・読み込み中) | `liveRegion = Polite` | `DesktopApp.kt`・`LoadingSkeleton.kt` |
| 絵で描く部品(stat・progressBar・gauge・chart・rating の表示・image・avatar・label 付きの icon) | 1 つにまとめて `contentDescription` | `A11y.kt` |
| 写真のサムネイル | `contentDescription = label` | `io/DesktopIo.kt` |

### 付けていない(確かめる時に特に見る)

| 所 | 今の作り | 起きそうなこと |
|---|---|---|
| toggle・slider・stepper の項目名 | 項目名は横の `Text`。部品に結び付けていない | 部品だけ読まれて、何の項目か分からない(**推測**) |
| stepper の「−」「+」 | 文字だけのボタン | 「マイナス」「プラス」とだけ読まれる(**推測**) |
| コマンドパレットの選んだ候補 | 背景の色だけ | 矢印で動かしても、どれを選んでいるか読まれない(**推測**) |
| picker の見出し | `Caption` の `Text` で、選択肢のまとまりに結び付けていない | 選択肢だけ読まれる(**推測**) |
| 手書き(drawingField)・ゲームの盤(Canvas) | 絵だけ | 何も読まれない(**推測**) |
| icon の label 無し・openUrlButton のアイコン | `contentDescription = null`(飾り) | 読まれない(意図どおり。ボタンの文字は読まれる) |

## 6. 結果の書き方

- 4 章の表を #2068 に写し、結果の欄に「読めた / 読めない / 違う文」と、読まれた文をそのまま書く。
- 読めない所は、画面・操作・読まれた文を書く。直す Issue はエージェントが分ける(#2068 のやること)。
- 利用者の情報(記録の中身・メールアドレス)は書かない。同梱の例(shopping・kakeibo)で確かめる。
