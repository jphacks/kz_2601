# 00 決定事項（フェーズ0・2026-10-24 確定）

> このファイルがプロジェクトの正典。変更は必ずここを更新してから。

## プロダクト
- iOS専用アプリ。初期画面は音声入力(ChatGPT音声画面風)。「こういうアプリが欲しい」と話すと、アプリ自体がそのアプリに変化する。
- 方式: Server-Driven UI。画面・遷移・コンポーネントは全てアプリに事前ビルド済み。バックエンドが「どの画面/コンポーネントをどう組むか」を **DSL JSON** として保持し、AIがそのJSONを書き換えることでアプリが動的に変わる。
- 初期状態のJSONは「AI会話画面のみ」。

## チーム
- 2人。iOS(Swift)は @ko-tarou がフル担当。@2410as がJSON周り(appconfig)のバックエンド担当。
- Claude Code(メイン＋サブエージェント)が設計・実装を並走。

## リポジトリ構成(モノレポ)
iOS アプリとバックエンドは **同じリポジトリ** に置く。ルート直下で領域ごとにフォルダを分け、CI は変更された領域のジョブだけ走る。

```
kz_2601/
├── ios/        # iOS アプリ本体(Swift / SwiftUI、xcodegen で project.yml から生成、.xcodeproj はコミットしない)
├── backend/    # Go 単一モジュール。cmd/appconfig・cmd/ai が各 Cloud Run サービス、pkg/dsl が共有型
├── docs/       # 00_decisions(正典)・01_goals・02_github_workflow・ios/・backend/・infra/
├── scripts/    # 運用スクリプト(github/ ブランチ保護など)
└── .github/    # workflows(backend-ci / ios-ci / secrets-scan / deploy-cloudrun)・PR/Issue テンプレ・CODEOWNERS
```
- 担当: `ios/` = @ko-tarou、`backend/cmd/appconfig/` = @2410as、それ以外は共同。
- iOS と backend の契約は `docs/dsl/dsl.schema.json`(DSL v1・決定表は `docs/dsl/README.md`)と `docs/backend/openapi-*.yaml`(API)。両側はここから導出し、二重定義しない。

## 技術スタック
| 領域 | 決定 | 備考 |
|---|---|---|
| iOS | Swift / SwiftUI、iOS 17+、xcodegen でプロジェクト生成 | 実機 iPhone 13 で検証。カメラ等はシミュレータ不可 |
| STT | Whisper → Claude の2段 | Whisperは **WhisperKit(オンデバイス)** を第一候補、不足なら OpenAI Whisper API |
| 生成AI | Claude API (claude-sonnet-5-5 を既定、品質不足なら claude-opus-5-5) | 構造化出力でDSL JSONを生成し、サーバー側でスキーマ検証。チューニングは後回し |
| バックエンド | Go 1.24、単一モジュール `backend/` に複数 `cmd/` | 1 cmd = 1 Cloud Run サービス。DSL型は `backend/pkg/dsl` で共有 |
| DB | **Firestore (Native mode)** | DSL JSONはドキュメントそのもの。スキーマ移行不要・無料枠・Cloud SQLの接続設定/VPC/課金を回避。RDBが要る根拠が出たら再検討 |
| 認証 | **Firebase Auth (Sign in with Apple)** | ログインサーバーを自作しない。GoはIDトークン検証のみ |
| インフラ | GCP (Cloud Run / Firestore / Secret Manager)、Terraform なし | gcloud コマンドの手順書で構築 |
| CI | GitHub Actions | Go: lint+test で **5分以内必須**。iOS: build(+unit test)、時間制限なし(超えてもローカル並行実行で代替可)。Draft PRでは発火しない |

## サーバー分割（当面）
| サービス | cmd | 担当 | 役割 |
|---|---|---|---|
| appconfig | `backend/cmd/appconfig` | @2410as | アプリDSL JSONのCRUD(エンティティ毎エンドポイント)、バージョン管理 |
| ai | `backend/cmd/ai` | Claude Code | 音声(or文字)→Claude→DSL JSONパッチ生成、スキーマ検証して appconfig に反映 |
| runner | `backend/cmd/runner`(M4・未作成) | Claude Code | M4 の「自動の動き」(きっかけ → 条件 → 処理)を背景で実行。outbox・定期処理・通知の送信。**本番への追加は人の承認の後**([#336](decisions/336-backend-autogen-approach.md)) |
- 当初2サービス。増やすのは「担当が分かれる」か「スケール特性が違う」時のみ。

## 課金・Pro(2026-11-06 オーナーの決定)
オーナーがチャットで明示的に承認(正典の書き換えも承認)。費用の詳細は [#338](decisions/338-server-feature-cost-model.md)(2026-11-06 改定)、有料の機能の判定は [features/pro.md](backend/features/pro.md)。値段・トークン制の中身はここでは決めない。
- **アプリを作る・直すだけなら無料**(今の 1 日の回数制限つき)。
- **有料の機能を持つアプリを作る・使う時は Tailor It Pro が要る**。有料の機能は 3 つ: ① みんなで共有するデータ(`sharedData`)② サーバーの自動の動き(`serverRules`: 通知・定期処理)③ アプリの中から使う AI(`aiButton` のオンライン版など)。
- **サーバー側で強制する**(402 `pro_required`。実装は #316)。iOS の確認カードだけに頼らない。
- **自動の動きは Pro だけ**(無料の枠は持たない。M4 の #338 の「無料にも小さな枠」をやめた)。
- **共有のアプリの費用は作成者の Pro で持つ**。参加者は無料で使える。
- 将来は使った分だけ払うトークン制(Lovable のような方式)にしたい。検討は #630(中身は未決定)。

## M4 バックエンドの自動構築(2026-11-06 決定)
オーナー承認。詳細は [#336 方式](decisions/336-backend-autogen-approach.md)・[#337 隔離](decisions/337-isolation-multitenancy.md)・[#338 費用](decisions/338-server-feature-cost-model.md)・[#339 悪用対策](decisions/339-abuse-threat-model.md)。単価・法令の確認などの「人が確かめること」は決定に含めない(各文書に未確認として残す)。
- 方式: **A(宣言的なサーバー機能)**。AI がコードを書く B は作らない。C(併用)は A を出した後に「表せない依頼」を測り、3 条件がそろってから検討(#344 は保留)。
- 語彙は 3 種: **データの決まり**(一意・役割・権限・宛先。appconfig がその場で確かめる)/ **絞り込み**(読み取り)/ **自動の動き**(きっかけ → 条件 → 処理)。
- 最初の語彙: きっかけ「作成・更新・時刻」、条件「Expr の部分集合」、処理「通知・レコードを書く・集計」。処理の連鎖は起こさない。外部への送信は作らない。
- 自動の動きは新サービス **runner** で実行(スケール特性が違うため)。きっかけは書き込みと同じトランザクションの outbox(#341 で確定)。
- 失敗の扱い: 冪等・回数の上限つきの再試行・上限超えや停止中は実行しない(後でまとめて実行もしない)・理由を作成者に出す。
- テナント = **appId**(private は (uid, appId))。
- 共有データに効く宣言は**作成者の公開 DSL だけが正**(参加者のコピーの書き換えは効かない)。
- 版の固定: きっかけの時点の公開 DSL の版で動かす。止める変更(取り下げ・削除・非公開化・停止)はすぐ効く。
- 作成者の退会・作成者の Pro が切れた時: そのアプリの自動の動きを止める(ルールとデータは残す)。2026-11-06 オーナーの決定で自動の動きは Pro だけになったため、共有データに限らない。
- runner は専用の SA + appId に縛られた取っ手(コードで範囲を強制)。プッシュの鍵を持つのは runner だけ。#288 の分離までは runner を prod だけで動かす。
- 費用: **Pro だけ**(2026-11-06 オーナーの決定。「無料の小さい枠」はやめた・上の「課金・Pro」)。共有アプリは作成者の Pro の枠で数え、参加者は無料。匿名アカウントは検証済みの Pro でなければ使えない。数える単位は実行・書き込み・通知の宛先・定期のルールの 4 つ。
- 上限: #338 の「上限(案)」の表の Pro の列を出発点に(無料の列は 0 = 使えない) `runner.usage` の実測で直す。環境全体の遮断 `RUNNER_GLOBAL_DAILY`(10,000)。使われていないアプリの定期処理は 30 日で止める。
- 上限に達した時は**止める**(遅らせない・追加課金しない)。利用者の書き込みは必ず保存し、止めたことを作成者に出す。
- 停止スイッチ: 全体 / アプリ / ルールの 3 段 + 通知だけを止めるスイッチ。運営の停止は作成者が解けない。
- 悪用対策の原則: **実行の段の強制が正**、生成の段は同じ規則で早めに断る。
- 通知の本文: URL・電話番号・メールアドレスを入れない・押すとアプリの中だけ。題 30 字・本文 80 字・差し込み 30 字。
- 受け取る人ごとの通知の上限: 1 アプリから 20 通 / 日・合計 100 通 / 日。ブロックの関係にある人がきっかけの通知は届けない。
- 宛先: public のアプリは作成者本人・ref 先のレコードの作者だけ。仲間探しのアプリには最初は自動の動きを付けない。同意は shared は参加時に確認つきで有効・public は無効。
- 静かな時間は入れない(iOS の集中モードに任せる)。
- 新しいアカウントは作成から 24 時間、他人宛ての通知を出せない。
- 通報: 対象に「自動の動き(ルール)」、理由に「なりすまし」を足す。24 時間で 3 人の通報で通知を自動で一時停止(強制停止と非公開化は人)。ミュートの率の閾値は実装の時に初期値を置き運用で直す。
- App Check は他人宛ての自動の動きを出す前に `enforce` にする(人の作業)。

## Git運用
- ブランチ: `main` / `develop` / `feature/*` / `hotfix/*` / `release/*`。エピックブランチ無し。
- `feature/*` → `develop`: PR必須、0 approve でマージ可(マージは人間のみ・botは不可)。
- `develop` → `main`: 実機テスト完了時のみ。
- CI: PRがOpen(非Draft)になった時点で発火。backend CIは5分以内、iOS CIは制限なし。
- PR/Issueテンプレートは `.github/` に用意。
- コミットは Conventional Commits (`feat:`/`fix:`/`docs:`/`chore:`)。
- **publicリポジトリ**。APIキー・GoogleService-Info.plist 等の秘密は絶対にコミットしない(`.gitignore` と secret scanning で二重防御)。

## 2日間の時間割（目安）
- Day1 午前: 設計並列(iOS/backend/要件/GCP手順/GitHub整備) → 昼: DSLスキーマv0をすり合わせ確定
- Day1 午後: iOSレンダラー骨格・音声画面、backend appconfig/ai 骨格、GCP構築、E2E 1本通す
- Day2 午前: デモ3本が通るまで詰める。Day2 午後: 動画・README・磨き

## 判断ログ(ユーザー確定)
| 日時 | 論点 | 決定 |
|---|---|---|
| 10/24 午前 | 生成アプリのユーザーデータ(明細・記録)の置き場 | **バックエンド(Firestore)**。DSL JSONもユーザーデータもユーザーに紐付けて保持。引き継ぎのためログイン機能あり |
| 10/24 午前 | 2回目以降の発話での改変方式 | **JSON Patch(RFC 6902)差分**。aiサービスはPatchを生成し、appconfigが適用・検証・バージョン保存 |
| 10/24 午前 | P0ネイティブ機能 | **カメラ と ローカル通知の両方**。他(位置/センサー/他アプリ連携)はP1以下 |
| 10/24 午前 | デモ3本 | 家計簿 / トレーニング記録 / 散歩メモ |
| 10/24 午前 | Sign in with Apple | 簡単ならやる。詰まったら固定デバイスID(匿名認証)で代替してよい |
| 10/24 午前 | STT | WhisperKit(オンデバイス)で確定。不足時のみ OpenAI Whisper API |
| 10/24 午前 | CI時間 | backend 5分以内必須。iOS は制限なし |
| 10/24 午前 | 秘密管理 | publicリポ。APIキー・plist は .gitignore + gitleaks。ローカルは `~/Desktop/JPHACKS2026/.env`(リポ外) |
| 10/24 昼 | DSL v1 統一(backend 案 × iOS 案) | **構造は backend 案(非再帰2段・閉じた enum・`additionalProperties:false`)、命名は iOS 案(camelCase・props フラット)**。`screens` は配列、`appId/version` は API 封筒、`voiceInput` は Leaf、`datePicker` 追加、式 `{{}}`/state は v1 に無し、写真は data URL(≤800px)、集計は iOS クライアント。正典 `docs/dsl/`(決定表 22 論点は `docs/dsl/README.md`) |
| 10/24 昼 | 画面レイアウト・タブ | **グリッド座標配置**: 各 Screen = 横 A〜F の 6 列 × 縦 1 行=可視高さ÷12(13 行目以降はスクロール)。Screen 直下の部品に省略可能な `area:"A1:C3"`、兄弟の重なりは backend 検証でエラー(id と area を人間可読で列挙)、可変長はセル内スクロール、`area` 無しは最終使用行の次から積む。Container 内は積むだけ(v1)。**タブ(`app.tabs[]`)は v1 に残す**(「タブ無し」却下)。仕様は `docs/dsl/README.md` §3.4 |
| 10/24 昼 | iOSアプリ名 | **メタモン**(表示名)。内部識別子は `Metamon`(ターゲット/スキーム/Bundle ID `jp.kz2601.metamon`)。旧仮名 Morph は廃止。リネームは #5 の実機確認完了後に単独PRで |
| 10/24 夜 | 正式名(#62・#69) | **Tailor It**(表示名・UI 文言・README・台本・ピッチ)。由来は「あなたに合わせて仕立てる(tailor)」。仮名「メタモン」は商標リスク(Q6)もあり廃止。内部識別子(ターゲット `Metamon`・Bundle ID `jp.kz2601.metamon(.widget)`・App Group `group.jp.kz2601.metamon`・`Metamon{DSL,Renderer,Services}`・`METAMON_*`・Firebase 登録・Firestore パス)は変更しない。「変身」「変身一覧」の比喩は残す |
| 10/24 昼 | PRテンプレ | 「重点レビューファイル」節を追加。レビュアーが読む順と見どころを最大5ファイルで明示 |
| 10/24 夜 | マイク FAB の位置(#52) | **ピクチャーインピクチャー風に動かせる**。初期位置は左下(DSL の「追加」が右下 `D11:F12` に来るため)のまま、ドラッグで左右どちらの辺にも吸着・端に寄せると画面外に隠れて取っ手だけ残る。吸着した側の最下段 2 列に部品がある画面ではその上へ自動退避。位置・隠れ状態は端末に保存(アプリ共通)。詳細は `docs/ios/architecture.md` |
