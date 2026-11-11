# Tailor It プライバシーポリシー


施行日: 2026年10月30日
改定日: 2026年11月11日
運営者: 高岡己太朗
連絡先: support@tailor-it.app

Tailor It(以下「本アプリ」)は、話した内容や入力した文から、自分専用のアプリ(記録・一覧・集計などの画面)を作って直せる iOS アプリです。本ポリシーは、本アプリが集める情報、使い道、送り先、保存と削除について説明します。

## 1. 集める情報と使い道

### 1.1 アカウントの情報
- **Sign in with Apple** または匿名でサインインします。認証は Google の **Firebase Authentication** が行います。
- Firebase Authentication には、Firebase が発行するユーザー ID(uid)、サインインの方法、作成日時・最終サインイン日時が保存されます。Apple でサインインした場合は、Apple が提供する場合に限り、Apple のユーザー識別子・名前・メールアドレス(Apple の「メールを非公開」を選んだ場合は中継用のアドレス)も保存されます。匿名の場合、名前・メールアドレスは保存しません。
- 運営者のサーバー(Firestore)が使うのはユーザー ID(uid)で、名前・メールアドレスは保存しません。運営者から、このメールアドレスに連絡することはありません。
- 使い道: 本人の確認、作ったアプリと記録を本人だけが使えるようにするため。

### 1.2 話した内容・入力した文(音声)
- **音声は端末の中で文字にします**(WhisperKit。音声認識のモデルは端末に保存します)。**音声そのもの(録音データ)はサーバーに送りません。**
- 音声認識のモデルが端末に無い時だけ(初回など)、サイズをお知らせして確認した上で、**Hugging Face, Inc.(米国)** から HTTPS でダウンロードします。このとき Hugging Face に届くのは、通常の通信に伴う情報(IP アドレスと、取得するファイルのリクエスト)だけで、音声・話した内容・記録・アカウントの情報は送りません。モデルが端末にあれば Hugging Face には接続しません。Hugging Face での扱いは [Hugging Face のプライバシーポリシー](https://huggingface.co/privacy) によります。
- 音声認識のモデルを準備している間(ダウンロード・読み込みの間)は、iPhone に Apple が用意している**端末の中の音声認識**で、仮に文字にします。この時も端末の中だけで処理し、**声を端末の外(Apple・運営者のサーバーを含みます)に送らず、録音も残しません。**端末の中で認識できない機種・言語では使わず、文字の入力をご案内します。最初に使う時に「音声認識」の許可を求めます。許可しない場合は、モデルの準備ができるまで文字で入力してください。
- 作ったアプリの「AI と話す」部品のマイクも、同じく端末の中で文字にして入力欄に入れるだけで、声は送りません。作ったアプリで録音のファイルを取り込んで文字にする場合(利用できる場合)も、端末の中で文字にし、ファイルそのものは送りません(文字にした結果は、記録に入れた場合に 1.4 のとおり保存します)。
- 文字にした内容(または直接入力した文)は、アプリを作る・直す時と、「AI と話す」部品で送信を押した時に、次の 1.3 のとおり AI に送ります。

### 1.3 第三者の AI(Anthropic)へ送る情報
アプリを作る・直す・相談する時と、作ったアプリの中の AI の部品を使う時に限り、**事前に同意をいただいた上で**、次の情報を本アプリのサーバー(Google Cloud Run・東京リージョン)を経由して **Anthropic, PBC(米国)の AI「Claude」** に送ります。
- 話した内容・入力した文(文字にしたもの)
- AI との会話の履歴(今の相談の分)
- 今のアプリの設計(画面・項目の名前・アプリ名・選択肢など)

**アプリを作る・直す・相談する時に、記録したデータ(数値・メモ・写真・位置など)の中身は AI に送りません。**

作ったアプリの中の AI の部品(利用できる場合に限ります)では、次のとおり送ります。いずれも、押した時・送った時だけ、同じ同意の範囲で送ります。
- **「AI で書く」ボタン**(文を作る・要約する・分類する・書き直す・抜き出す・提案する)
  - 短い答えのボタン: 端末の中の AI(Apple Intelligence)が使える端末では端末の中で処理し、送りません。使えない端末では、Tailor It Pro のオンラインの AI として、そのボタンの材料として設定された最近の記録・入力中の値の文字(最大 4KB)を送ります。
  - 長い答えのボタン(長い文の要約・手紙・計画など): 端末の中の AI は使わず、材料の文字(最大 16KB)を、いつもオンラインの AI に送ります。
  - 同じ画面に出している外部の公開データ(天気・気圧など。1.11)を材料にするボタンでは、表示している行(最大 20 行・2KB)も材料に加えます。
  - AI の答えはフォームの欄に入れるだけで、保存は利用者が押した時です。
- **「AI と話す」部品**: 送るたびに、そのアプリに保存した自分の会話のうち直近の発言(最大 20 件・合わせて 16KB まで)と、そのアプリで決めた AI の役割の説明を、オンラインの AI に送ります。端末の中の AI は使いません。この部品は自分だけの記録として使い、会話はほかの人と共有しません(1.4)。
  - そのアプリで参考にする記録帳が決めてある場合は、その記録帳の**自分の記録**のうち新しいもの(最大 20 件・8KB まで。会話と合わせて 16KB まで)も送ります。写真・録音・位置・ほかの記録への参照の項目と、隠した項目・計算で出す項目は送りません。ほかの人が入れた記録は送りません。
  - そのアプリで決めてある場合は、AI が会話の内容から記録の案を返します。案は画面に出すだけで、「追加」を押した時だけ、端末からそのアプリの記録として保存します(1.4)。案のために新しく送るものはありません。
- **写真を材料にするボタン**: そのフォームの写真 1 枚を、長い辺 1024 ピクセル以下に縮めた画像(JPEG)にして、オンラインの AI に送ります。端末の中の AI は使いません。写真に写っている文字や物(薬の袋・処方箋など、健康に関わる内容を含むことがあります)も AI が読みます。送りたくない写真は選ばないでください。AI は薬の飲み方や量を判断しません。
- 写真は、上の写真を材料にするボタンの場合だけ送ります。**音声・動画そのものは送りません。**
- 共有・公開のアプリでは、オンラインの AI に材料として送る記録は、**あなたが入れた記録だけ**です。ほかの参加者が入れた記録は送りません。
- 共有のアプリに参加した人は、作成者の Tailor It Pro でオンラインの AI を使えることがあります。この場合も、送る前に、使う人(参加者)自身の同意が要ります。参加者が送った文や写真が作成者に届くことはありません(AI の答えを共有のアプリの記録に保存した場合は、1.6 のとおり参加者と共有されます)。

送った情報は、アプリを作る・ボタンの文を作る(要約などを含みます)・会話の返事(記録の案を含みます)を作るためだけに使います。本アプリは Anthropic の API を、Anthropic の商用利用規約のもとで使います。この条件では、API 経由の入力と出力は Anthropic のモデルの学習に使われません。Anthropic は、送った情報を同社の定める期間だけ保存して削除します。ただし、利用ポリシー違反の調査や法令への対応のために、より長く保存することがあります(Anthropic のプライバシーポリシーと商用利用規約に従います)。

同意は、初回起動時の画面、または設定の「プライバシー」から行えます。同意しない場合も、作ったアプリの閲覧・記録は使えます(AI を使う機能だけが使えません)。同意は設定からいつでも取り消せます。取り消し後は送りません。送信済みの情報は取り消せません。

### 1.4 作ったアプリと記録
- 作ったアプリの設計(DSL)と版の履歴、記録したデータ(写真を入れた場合は縮小した画像を含む)は、端末に保存し、接続がある時に **Google Cloud Firestore**(運営者の Google Cloud プロジェクト・東京リージョン)へ送って保存します。
- 使い道: 端末を変えても・再インストールしても使えるようにするため、ホーム画面のウィジェット・通知を表示するため。
- アプリを作る・直す相談の会話は、サーバーでは最大 30 分だけ保持し、端末には履歴として保存します。
- 作ったアプリの「AI と話す」部品の会話(あなたの発言と AI の返事)は、そのアプリの自分だけの記録として、上の記録と同じく保存します。
- 1 日に AI を使える回数を数えるため、ユーザー ID ごとの利用回数(匿名の場合は IP アドレスを変換した値ごとの回数も)を日ごとに保存します。

### 1.5 端末の機能(作ったアプリが使う場合のみ・許可を求めます)
- 本アプリの版によって、使える機能が異なります。次の機能は、その版で利用できる場合に限り、作ったアプリが使う時だけ許可を求めます。
  - マイク(音声入力・録音)、音声認識(1.2 の準備中の仮の聞き取り)、カメラ・写真(写真の項目・QR コードの読み取り)、位置情報(場所の項目・地図・道案内・近くの場所の検索・住所の入力・保存した場所に着いた時の通知)、モーション、ヘルスケア(歩数・体重の表示)、ローカルネットワーク(近くの端末と送り合う)、Apple Music、通知。
- ヘルスケアのデータは端末の中で表示するだけで、サーバー・AI には送りません。広告やマーケティングには使いません。App Store で配布する版にはヘルスケアの機能が無い場合があり、その場合はヘルスケアのデータを一切読み取りません。
- 位置情報・写真は、ユーザーが記録に入れた場合に限り、記録の一部として 1.4 のとおり保存します。写真の項目で、端末の中の Image Playground で作ったイラストも、写真と同じに扱います(イラストは端末の中だけで作り、作るために何かを送ることはありません)。
- 地図の表示・近くの場所の検索・住所の入力・道案内では、端末から Apple に直接、検索の言葉と位置を送ります(2 章)。本アプリのサーバーには送りません。
- Android 版の地図の表示では、端末から OpenFreeMap に直接、地図の画像(タイル)を取りに行きます。この時 OpenFreeMap は、端末の IP アドレスと表示している地図の範囲を受け取ります(2 章)。
- 保存した場所に着いた時の通知は、iPhone の仕組みで端末の中で判定します。この判定のために位置をサーバーに送ることはありません。

### 1.6 共有・公開(利用できる場合)
- 本アプリの版によっては、共有・公開の機能がありません。次は、その機能を利用できる場合の説明です。
- 「共有」を使うと、作ったアプリの設計を共有リンクで他の人が見られるようになります。共有したアプリに参加した人は、表示名(入力した場合)と記録を、そのアプリの参加者と共有します。
- 「探す」に掲載すると、作ったアプリの設計(アプリ名・画面・項目の名前など)が、本アプリの他のユーザーに表示されます。記録したデータは掲載しません。
- 作ったアプリの中に「公開」と表示されるデータ(掲示板・ランキングなど)がある場合、そこに入れた内容はそのアプリを使う他の人も見られます。入れる前に画面でお知らせします。
- 共有・掲載はいつでも取り下げられます。

### 1.7 課金(提供する場合)
- 本アプリの版によっては、アプリ内課金がありません。Tailor It Pro(自動更新サブスクリプション)を提供する場合、支払いは **Apple(App Store)** が処理します。運営者はカード情報などの支払い情報を受け取りません。
- 購入の状態は**本アプリのサーバーで確かめます。**購入・復元の後とアプリの起動時に、Apple が署名した取引の情報を端末からサーバーに送り、サーバーが Apple の署名を確かめた上で、ユーザー ID と一緒に次を保存します。Apple からの更新・返金の知らせ(App Store Server Notifications)と、Apple への問い合わせ(App Store Server API)でも同じ情報を更新します。
  - 商品 ID、取引 ID と最初の取引 ID、有効期限、取り消し(返金)の日時、自動更新の状態、支払いの猶予期間の終わり、本番かテストか(環境)、更新日時
  - 1 つの購読をどのユーザー ID が使っているかの対応(最初の取引 ID ごと)
- 購入には、どのアカウントの購入かを確かめるため、ユーザー ID から作った値(元のユーザー ID には戻せない値)を付けます。
- 使い道: Tailor It Pro が使えるかを判定するため(有料の機能はサーバーで判定します)。

### 1.8 クラッシュ報告
- 本アプリが異常終了(クラッシュ)した時に、原因を調べて直すため、Google の **Firebase Crashlytics** に次の情報を送ります。
  - 落ちた場所(プログラムの位置・スタックトレース)、異常終了の種類、発生日時
  - 端末の機種・OS の版・本アプリの版、発生時の空きメモリ・空き容量・画面の向きなどの端末の状態
  - Crashlytics がインストールごとに作る識別子と、ユーザー ID(uid。1.1 のもの)
- **話した内容・入力した文・記録・作ったアプリの設計(DSL)は送りません。**名前・メールアドレスも送りません。
- 既定はオンです。設定の「プライバシー」の「クラッシュ報告を送る」からいつでもオフにできます。オフにすると、以後は送らず、まだ送っていない報告も端末から消します。
- 使い道: 不具合の原因を調べて直すためだけです。広告・トラッキングには使いません。

### 1.9 集めないもの
- 広告 ID・トラッキング・行動を分析するための SDK(Firebase Analytics 等)は使いません。第三者への販売はしません。
- 本アプリのサーバーでの利用の集計は 1.12 のとおりです。

### 1.10 通知とサーバーの自動の動き(提供する場合)
- 本アプリの版によっては、この機能がありません。次は、その機能を利用できる場合の説明です。
- プッシュ通知を受け取るため、通知を許可した端末について、Apple が発行する端末のトークン(デバイストークン)、インストールごとに作る識別子、配信の環境、本アプリの版、OS の版を、ユーザー ID と一緒にサーバー(Firestore)に保存します。1 人 10 台までで、超えると古いものから消します。トークンはログに書かず、ほかの人には見せません。通知の配信には Apple のプッシュ通知の仕組み(APNs)を使います。
- 同じ端末で別のアカウントにサインインした場合、前のアカウントの通知が届かないよう、前のアカウントに保存したその端末のトークンを消します。サインアウトや通知をやめた時も、その端末のトークンを消します。
- 他の人の操作がきっかけで届く通知は、**アプリごとに受け取る人の同意がある時だけ**送ります。アプリごとの同意の有無を保存し、いつでも取り消せます。共有のアプリでは参加する時に確認した上で有効になり、公開のアプリでは既定で無効です。
- 作成者が Tailor It Pro で設定した「自動の動き」(記録が作られた・更新された時や、決めた時刻に、条件に合えば通知を送る・記録を書く・集計する)は、本アプリのサーバーが実行します。この時、サーバーはそのアプリの記録を読み、条件の判定と通知の文を作るのに使います。この処理で記録を AI に送ることはありません。
- 自動の動きで、天気・気圧・警報と注意報をその日の値として記録に残す場合(利用できる場合)、サーバーが 1.11 と同じ仕組みで取得します。出どころに送るのは、作ったアプリに決めて書いた地域(府県予報区のコード、気圧はその代表地点の座標)だけで、記録の値やユーザーの情報は送りません。
- 共有のアプリに参加すると、作成者が設定した自動の動きによって、通知が届くことがあります。通知の本文には、作成者が決めた文と、記録の値の一部(30 字まで)が入ることがあります。URL・電話番号・メールアドレスは入れません。
- 使いすぎを防ぐため、自動の動きの実行・書き込み・通知の数を、アプリごと・受け取る人ごとに数えて保存します。

### 1.11 外部の公開データ・本の情報(利用できる場合)
- 本アプリの版によっては、この機能がありません。
- 作ったアプリに、公的機関などが公開しているデータ(天気予報・気象の警報と注意報・気圧の予報・祝日・お知らせ・arXiv の分野ごとの新着論文の一覧・国会会議録の発言の一覧)を表示できます。また、提供する場合は、入れた ISBN から本の情報(書名・著者・出版社・出版日)をフォームに入れられます。どれも、本アプリのサーバーが、運営者が決めた出どころだけから取得し、端末へ届けます。端末から出どころへ直接はつながりません。
- 取得する時に出どころへ送るのは、次だけです。**ユーザー ID・あなたの位置・記録・話した内容は送りません。**
  - 天気予報・警報と注意報: 府県予報区のコード
  - 気圧の予報: 府県ごとに運営者が決めた代表地点(県庁などの所在地)の緯度・経度。あなたの位置ではありません
  - 祝日・お知らせ: 取得するファイルの指定
  - arXiv の新着論文: 論文の分野(情報科学の分野など)
  - 国会会議録: 作ったアプリに決めて書いた、探す語(40 字まで)
  - 本の情報: 入れた ISBN
- 出典(天気予報・警報と注意報・祝日・お知らせは、いずれも[公共データ利用規約(第1.0版)](https://www.digital.go.jp/resources/open_data/public_data_license_v1.0)により利用しています)
  - 天気予報: 気象庁ホームページ(https://www.jma.go.jp/)の府県天気予報を加工して作成(天気の種類を言葉と絵に置き換え、表示する日数を絞っています)
  - 祝日: 内閣府ホームページ(https://www8.cao.go.jp/chosei/shukujitsu/gaiyou.html)の「国民の祝日」を加工して作成(これから来る祝日だけを表示しています)
  - お知らせ: デジタル庁ホームページ(https://www.digital.go.jp/)の新着・更新の RSS、消防庁ホームページ(https://www.fdma.go.jp/)の新着情報・災害情報の RSS を加工して作成(題・日時・リンクを件数を絞って表示しています)
  - 警報・注意報: 気象庁ホームページ(https://www.jma.go.jp/)の気象警報・注意報を加工して作成
  - 気圧の予報: MET Norway(ノルウェー気象研究所)のデータ([CC BY 4.0](https://creativecommons.org/licenses/by/4.0/))を加工して作成
  - 新着論文: arXiv.org の論文の情報([CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/))。arXiv による承認や提携はありません
  - 国会会議録: 国立国会図書館 国会会議録検索システム(https://kokkai.ndl.go.jp/)の検索用 API(発言者・会議名・日付とリンクだけを表示しています)
  - 本の情報: [openBD](https://openbd.jp/)(書誌は国立国会図書館の書誌データ・CC BY 4.0 を含みます)

### 1.12 利用の集計
- 本アプリを良くするため、本アプリのサーバーが受けたリクエストから、使われた回数(アプリの一覧を開いた・アプリを開いた・アプリを作った・直した・共有した・掲載した、など)と、その成功・失敗を集計します。端末から集計のために新しく送るものはありません。
- 集計には、ユーザー ID とアプリの ID を変換した値を使います。運営者は、この値を元のユーザー ID と照らし合わせられます。話した内容・会話・アプリの設計・記録の値・IP アドレスは集計に入れません。
- 本アプリの版によっては、「AI で書く」ボタンについて、端末の中の AI が使えるかと、答えを出せなかった回数を、ボタンの種類ごとに 1 日 1 回まとめて送ります。ユーザー ID・入力した文・記録は集計に含めません。設定の「プライバシー」の「AI ボタンの利用状況を送る」でオフにでき、オフにすると、まだ送っていない分も端末から消します。
- 集計は、広告・トラッキングには使いません。

### 1.13 フィードバック(利用できる場合)
- 本アプリの版によっては、この機能がありません。
- アプリ内の「フィードバックを送る」(または端末を振る)から、不具合や要望を送れます。送るのは、書いた本文、種類、ユーザー ID(uid)、送った日時です。
- 次のものは、送る画面で本人がオンにした場合だけ送ります(既定はすべてオフです)。送る前に中身を表示します。
  - 今の画面のスクリーンショット
  - 今のアプリの設計(アプリの名前・画面の文言・項目の名前を含みます)
  - 本アプリの版・OS の版・端末の機種・表示言語
- **記録(入力したデータ)の中身は送りません。** スクリーンショットには画面に表示されている内容が写るため、送る前に確認してください。
- 使い道: 不具合を直し、本アプリを良くするために運営者が読むためだけです。AI には送りません。広告・トラッキングには使いません。

## 2. 第三者への提供
次の委託先に、上記の目的の範囲で情報を預けます。法令に基づく場合を除き、これ以外の第三者には提供しません。
- Google LLC(Firebase Authentication・Firebase Crashlytics・Cloud Run・Firestore・Cloud Logging)
- Anthropic, PBC(AI による生成・相談。米国への移転を含みます)
- Apple Inc.(Sign in with Apple・App Store の課金と取引の確認・プッシュ通知の配信)
- Hugging Face, Inc.(音声認識のモデルの配信。1.2 のダウンロードの通信だけで、音声・話した内容は送りません。米国)
- OpenFreeMap(Android 版の地図のタイルの配信。端末の IP アドレスと表示している地図の範囲だけを受け取ります)

1.2 の端末の中の音声認識では、声を Apple に送りません。1.11 の出どころ(気象庁・内閣府・デジタル庁・消防庁・MET Norway(ノルウェー)・arXiv(米国)・国立国会図書館・openBD)には、1.11 に書いたもの(府県予報区のコード・代表地点の座標・ファイルの指定・論文の分野・探す語・ISBN)のほかは送りません。

地図の表示・場所の検索・住所の取得・道案内(利用できる場合)では、iPhone に Apple が用意している地図の仕組み(MapKit)を使い、**端末から Apple に直接**、検索の言葉と位置(現在地・地図の範囲・選んだ場所の座標)を送ります。本アプリのサーバーは通らず、運営者はこの送信の内容を受け取りません。Apple での扱いは [Apple のプライバシーポリシー](https://www.apple.com/legal/privacy/) によります。

Android 版の地図の表示(利用できる場合)では、[OpenFreeMap](https://openfreemap.org/) の地図の画像(タイル)を使い、**端末から OpenFreeMap に直接**取りに行きます。この時 OpenFreeMap は、端末の IP アドレスと表示している地図の範囲(どの場所のタイルか)を受け取ります。検索の言葉・現在地の座標・記録の中身・ユーザー ID は送りません。本アプリのサーバーは通らず、運営者はこの送信の内容を受け取りません。本アプリは [OpenFreeMap の利用規約](https://openfreemap.org/tos/) に同意して使っており、OpenFreeMap での扱いはこの利用規約によります。

## 3. 保存期間
- 作ったアプリ・記録・アカウントの情報は、アカウントを削除するまで保存します(削除後の扱いは 4 章)。
- 相談の会話は、サーバーで最大 30 分です。
- 購入の記録(1.7)と通知の受け取りの同意(1.10)は、アカウントを削除するまで保存します。
- 通知のための端末のトークン(1.10)は、サインアウト・通知をやめる・アカウントの削除の操作のいずれかまで保存します。
- 1 日の利用回数は、日付ごとの回数(数値)だけで、話した内容などは含みません。
- クラッシュ報告(1.8)は、Firebase Crashlytics の標準の保存期間(受け取ってから 90 日)が過ぎると Google が消去します。
- サーバーの動作記録(ログ。リクエストの日時・経路・結果・IP アドレスなど)は、Google Cloud Logging に最大 30 日保存します。ログには、話した内容・記録の中身を書かないようにしています。リクエストの経路には、問い合わせの値(1.11 の ISBN・探す語など)が含まれることがあります。
- 利用の集計(1.12)は、サーバーの動作記録と同じく最大 30 日保存します。
- フィードバック(1.13)は、受け取ってから 180 日で消去します。
- 障害やデータの破損に備えて、Firestore のバックアップを取っています。直前 7 日間の任意の時点に戻すための記録と、1 日 1 回の複製(14 日間保持)があり、どちらも期限が来ると自動的に消えます。

## 4. アカウントと情報の削除
- 設定の「アカウントを削除」から、アプリ内で削除できます。削除すると、作ったアプリと共有リンクが使えなくなり、端末のデータを消し、Firebase のアカウントを削除し、Sign in with Apple の連携を解除(トークンの失効)します。
- サーバー上のデータは、削除の操作の時点で使えなくし(論理削除)、30 日以内に消去します。送ったフィードバック(1.13)も、このときに消去します。ほかの人と共有しているアプリへの投稿は残ります。
- 通知のための端末のトークンは、削除の操作の時点で消し、通知を止めます。作成したアプリの自動の動きも止まります。購入の記録と通知の受け取りの同意は、30 日以内に消去します。
- Tailor It Pro の購読は、アカウントを削除しても自動では解約されません。iPhone の「設定」から解約してください(利用規約 4 章)。
- 消去したデータも、バックアップには最大 14 日間残り、その後自動的に消えます。バックアップは障害からの復旧にだけ使い、復旧したときは消去済みのデータを改めて消去します。
- 作ったアプリを 1 つずつ削除した場合は、そのアプリのサーバー上のデータをその時点で消去します。

## 5. お子さまの利用
- 本アプリは 13 歳未満の方を対象にしていません。
- 13 歳以上 18 歳未満の方は、保護者の同意を得た上で使ってください。

## 6. 改定
- 本ポリシーを変える場合はアプリ内でお知らせします。AI に送る情報や送り先を変える場合は、改めて同意をいただきます。

## 7. お問い合わせ
- 高岡己太朗
- メール: support@tailor-it.app

---

## English summary

> This summary is for convenience. The Japanese text above prevails.

- **Operator:** Kotaro Takaoka (高岡己太朗) (individual). **Effective:** October 30, 2026. **Revised:** November 11, 2026.
- **Sign-in:** Sign in with Apple or anonymous, via Google Firebase Authentication. Firebase stores your user ID and, if Apple provides them, your name and email (or Apple's private relay address). Our own server (Firestore) uses only the user ID.
- **Voice:** Speech is transcribed **on the device** (WhisperKit). **Audio is never uploaded.** Only when the speech model is not on the device (e.g. first use), and after asking you, it is downloaded over HTTPS from **Hugging Face, Inc. (USA)**. Hugging Face receives only standard connection data (your IP address and the file request), never audio, what you say, your records or account details. See [Hugging Face's privacy policy](https://huggingface.co/privacy). While the model is being prepared, Apple's **on-device** speech recognition transcribes your speech temporarily (with the "Speech Recognition" permission); your voice still never leaves the device and is not recorded. The microphone of the "AI chat" part and importing audio files (where available) also transcribe on the device; the audio is never sent (transcribed text you put in a record is stored as a record).
- **Third-party AI:** Only **after your explicit consent**, the transcribed/typed request, the current conversation and the current app design (screen/field names) are sent through our server (Google Cloud Run, Tokyo) to **Anthropic, PBC (USA, "Claude")** solely to build or edit your app. The contents of your records are not sent when building, editing or chatting about your app. AI parts inside your apps (where available) send data only when you tap or send, within the same consent:
  - **"AI write" buttons** (write, summarize, classify, rewrite, extract or suggest): short answers are processed on the device where on-device AI (Apple Intelligence) is available and nothing is sent; otherwise, as a Tailor It Pro online AI feature, the recent records or form values configured for that button (up to 4KB of text) are sent. Long answers (summaries of long text, letters, plans, etc.) do not use on-device AI; the configured text (up to 16KB) is always sent to the online AI. A button that uses public data shown on the same screen (weather, pressure, etc.) also adds the rows shown (up to 20 rows, 2KB). Answers are only filled into form fields; you save them yourself.
  - **"AI chat" part:** each message sends the latest turns of your own conversation saved in that app (up to 20 turns and 16KB in total) and the AI role description set for the app. On-device AI is not used. This part uses your private records only; the conversation is not shared with others. If the app sets a reference list, it also sends **your own** latest records from that list (up to 20 records and 8KB; 16KB together with the conversation), excluding photo, audio, location and reference fields, hidden fields and calculated fields, and never other people's records. If the app allows it, the AI may suggest a record from the conversation; it is only shown, and is saved as a record from the device only when you tap "Add". Nothing extra is sent for suggestions.
  - **"Photo" AI button:** one photo from the form is downscaled (JPEG, long edge up to 1024 pixels) and sent to the online AI. The AI also reads text and objects in the photo (which may include health-related content such as medicine bags or prescriptions). Do not choose a photo you do not want to send. The AI does not decide how to take a medicine or how much.
  - Photos are sent only by such a button. Audio and video are never sent.
  - In shared or public apps, only records **you** added are sent to the online AI; other participants' records are not.
  - A participant of a shared app may use the online AI under the creator's Tailor It Pro. The participant's own consent is still required, and what the participant sends is not delivered to the creator (AI answers saved to the shared app's records are shared with its participants).
- **Use by Anthropic:** data sent to Anthropic is used only to build your app, write a button's text (including summaries) or reply in a chat (including suggested records). Under Anthropic's commercial terms, API inputs and outputs are not used to train their models. Without consent you can still view and use your apps. You can withdraw consent in Settings at any time.
- **Storage:** App designs, version history and records (including downscaled photos you add) are stored on the device and in Google Cloud Firestore (Tokyo). Conversations for building an app are kept on the server for up to 30 minutes. "AI chat" conversations are saved as your private records of that app. Server logs are kept for up to 30 days (request paths may include lookup values such as an ISBN or search term). We keep Firestore backups to recover from failures: point-in-time recovery data for the last 7 days and daily backups kept for 14 days. Both expire automatically.
- **Device features** (microphone, speech recognition, camera/photos, location, motion, Health, local network, Apple Music, notifications) are used only where available in your build, only when your app uses them, and with your permission. Location is also used for nearby place search, address entry and notifications when you arrive near a saved place; arrival is detected on the device by iOS and your location is not sent to our server for this. Health data stays on the device and is never used for advertising. Illustrations you create on the device with Image Playground in a photo field are treated the same as photos (they are created on the device only; nothing is sent to create them).
- **Maps** (where available): map display, place search, address lookup and route guidance use Apple's MapKit, which sends your search terms and locations (current location, visible map area, chosen place) **directly from the device to Apple**. They do not pass through our server and we do not receive them. [Apple's privacy policy](https://www.apple.com/legal/privacy/) applies. On Android, map tiles are fetched **directly from the device** from [OpenFreeMap](https://openfreemap.org/), which receives your device's IP address and the visible map area (which tiles are requested), never search terms, your location coordinates, records or user ID. They do not pass through our server. We use OpenFreeMap under [its terms of service](https://openfreemap.org/tos/), which govern its handling of this data.
- **Sharing / public data** (where available): shared links, the "Discover" catalog and data marked "public" can be seen by others. You can withdraw sharing at any time.
- **Payments** (if offered) are processed by Apple; we never receive payment details. Our server verifies Apple's signed transactions (and Apple's server notifications) and stores, with your user ID, the product ID, transaction IDs, expiry, revocation/refund time, auto-renew status, grace period, environment and which account a subscription belongs to. Purchases carry a value derived from your user ID that cannot be turned back into it.
- **Notifications and automations** (where available): to deliver push notifications we store your device token, a per-installation ID, environment, app and OS version with your user ID (up to 10 devices), and deliver them through Apple Push Notification service. Notifications triggered by other people are sent only if you agreed per app (on when you join a shared app after confirming; off by default for public apps); you can turn this off any time. Automations set by an app's creator (Tailor It Pro) run on our server on record changes or at set times and may notify participants; notification text may include up to 30 characters of record values but never URLs, phone numbers or email addresses. Records are not sent to AI for this. When an automation saves the day's weather, pressure or warnings to records (where available), our server fetches them as in "Public data" below and sends the source only the region written in your app (a forecast-area code, or the representative point for pressure), never record values or your details.
- **Public data and book info** (where available): our server fetches weather forecasts, weather warnings and national holidays and notices (Japan Meteorological Agency, Cabinet Office, Digital Agency, Fire and Disaster Management Agency; used and modified under the Public Data License (Version 1.0)), pressure forecasts (MET Norway, CC BY 4.0, modified), new papers per category (arXiv; metadata CC0 1.0; not endorsed by or affiliated with arXiv) and Diet records (National Diet Library, Diet Record Search API; speaker, meeting and date with a link only). If offered, it also looks up book details by ISBN from openBD (including National Diet Library bibliographic data, CC BY 4.0). We send them only a forecast-area code, fixed coordinates of a representative point per prefecture chosen by us (not your location), the feed to fetch, a paper category, a search term written in your app's design (up to 40 characters), or the ISBN you entered, never your user ID, location or records. The device never connects to these sources directly.
- **Crash reports:** When the app crashes, Google's Firebase Crashlytics receives the crash location (stack trace), crash time, device model, OS and app version, device state (free memory/disk, orientation), a per-installation identifier and your user ID (uid). **What you say, your records and your app designs are never included.** On by default; turn it off in Settings > Privacy ("Send crash reports"). Google deletes crash reports 90 days after receipt (Crashlytics' standard retention period).
- **No advertising IDs, tracking or analytics SDKs** (such as Firebase Analytics). We do not sell data.
- **Usage statistics:** to improve the app, our server counts usage from the requests it already receives (opening the app list or an app, building, editing, sharing, publishing, etc.) and whether they succeeded. Nothing extra is sent from the device for this. The counts use values derived from your user ID and app IDs, which we can match back to your user ID. What you say, conversations, app designs, record values and IP addresses are not included. They are kept for up to 30 days, like server logs. In some builds, the app also sends, once a day, whether on-device AI is available and how many times "AI write" buttons failed, per button type, without your user ID, text or records; turn it off in Settings > Privacy ("Send AI button usage"), which also deletes unsent counts. Usage statistics are not used for ads or tracking.
- **Feedback** (where available): "Send feedback" (or shaking the device) sends the text you write, its type, your user ID and the time. A screenshot, the current app's design and the app/OS version, device model and language are sent only if you turn them on (all off by default), and are shown before sending. The contents of your records are never sent. Read by the operator only to fix bugs and improve the app; not sent to AI. Erased 180 days after receipt, or within 30 days of account deletion.
- **Deletion:** Delete your account in Settings. Server data becomes unavailable immediately and is erased within 30 days. Device tokens are deleted immediately and your apps' automations stop; purchase records and notification consents are erased within 30 days. Deleting your account does not cancel a Tailor It Pro subscription; cancel it in iOS Settings. Posts in apps shared with others remain. Erased data may remain in backups for up to 14 more days until they expire. Backups are used only for disaster recovery; if we restore one, we erase previously deleted data again.
- **Children:** Not intended for children under 13. Users aged 13 to 17 must have consent from a parent or guardian.
- **Changes:** we will notify you in the app. If we change what is sent to AI or where it is sent, we will ask for your consent again.
- **Contact:** support@tailor-it.app
