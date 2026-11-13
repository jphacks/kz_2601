# ai-eval の expect のキー(#371)

生成のケース(`backend/internal/ai/eval/*cases.json`)の `expect` に書けるキーの一覧。回し方は [README.md](README.md)。

## 目次

1. [読み方](#1-読み方)
2. [キーの表](#2-キーの表)
3. [表とコードのずれを見つける](#3-表とコードのずれを見つける)
4. [キーを足す時](#4-キーを足す時)

## 1. 読み方

- キーと型の正は `backend/cmd/ai-eval/expect.go` の `expectKeys`(と、各ファイルの `init()` で足す `expectKeys["…"]`)。知らないキー・型の違いは `-check` と実行の前に止まる。
- 型の `string[]` は 1 つ以上の文字列の配列。
- bool のキーは、たいてい true = 「ある」・false = 「無い」を確かめる。`matchSafe`・`noOmission`・`noShared`・`ruleSim` は true の時しか判定しない(false は何も確かめない)。
- 「判定する関数」が `runCase` のものは `main.go` の `runCase` の中で直接見ている。
- 決まった組: `keep` / `growTo` は `keepEntity` が要る。`optHas` / `optNot` / `fields` / `refPick` は `enumField` が要る(`validateExpect`)。
- 値の形が決まっているキー: `notification`(`any` か `HH:MM`)・`scope`・`routeDest`・`routeMode`・`export`・`qrSource`・`aiGenerate`・`aiTask`・`aiLength` は、違う値を `-check` が止める。
- 例のケースは、そのキーを使う最初のケース(ファイル名の順)。対話のケース(`*chat_cases.json`)の `expect` は別物で、`question` / `ready` / `answer` / `proposal` のどれか(`chat.go`)。

## 2. キーの表

名前順(大文字と小文字は区別しない)。

| キー | 型 | 意味 | 判定する関数 | 例のケース |
|---|---|---|---|---|
| `activityButtons` | bool | startActivity と endActivity の両方の action がある | `checkLiveActivity` | `live_activity_cases.json` の `la_driving`(`true`) |
| `addMinutes` | bool | 式のどこかで op `addMinutes` を使う(到着 − 移動の分 = 出発) | `checkAddMinutes` | `add_minutes_cases.json` の `addmin_depart_transit`(`true`) |
| `addressed` | bool | `addressedTo` を持つ entity がある(宛先つきのメッセージ) | `checkAddressed` | `addressed_cases.json` の `ad_create_dm`(`true`) |
| `addressedGroup` | bool | 宛先つきの entity の宛先が type `members` | `checkAddressed` | `addressed_cases.json` の `ad_create_group`(`true`) |
| `addressedUnread` | bool | 宛先つきの entity に、一覧が出す mark(`item.mark`)がある(未読) | `checkAddressed` | `addressed_cases.json` の `ad_create_dm`(`true`) |
| `addToCalendar` | bool | button / toolbar / onItemTap のどれかが `addToCalendar` を実行する | `checkCalendarAdd` | `ai_outputs_cases.json` の `aot_poster_event`(`true`) |
| `aiChatInput` | bool | aiChat が自分の記録を読む(`inputEntity`) | `checkAIChatInput` | `ai_conversation_cases.json` の `ac_tutor`(`false`) |
| `aiChatSaveTo` | bool | aiChat が会話から記録を提案する(`saveTo`) | `checkAIChatSaveTo` | `ai_conversation_cases.json` の `ac_counselor`(`false`) |
| `aiChatSpeak` | bool | aiChat が `speak: true`(AI の返事を読み上げる) | `checkAIChatSpeak` | `ai_conversation_cases.json` の `ac_english_read_aloud`(`true`) |
| `aiGenerate` | string | generate の aiButton の形が `line` / `list` / `fields`、または `any` | `checkAIGenerate` | `ai_generate_cases.json` の `ag_batsu`(`"list"`) |
| `aiGenerateEntity` | bool | generate の aiButton のどれかが記録を読む | `checkAIGenerate` | `ai_generate_cases.json` の `ag_fridge`(`true`) |
| `aiInputFeeds` | bool | generate の aiButton が同じ画面の feed の行を読む(`inputFeeds`) | `checkAIInputFeeds` | `ai_input_feed_cases.json` の `aif_weather_pressure_health`(`true`) |
| `aiInputPhoto` | bool | aiButton のどれかが写真の field を送る(`inputPhoto`)。false = どれも送らない | `checkAIGenerate` | `ai_photo_input_cases.json` の `ap_prescription_read`(`true`) |
| `aiLength` | string | aiButton のどれかの length(`short` / `long`。未設定は `short`) | `checkAIGenerate` | `ai_long_cases.json` の `al_thanks_letter`(`"long"`) |
| `aiOutputs` | bool | `fields` の generate の aiButton がフォームの field を埋める(`outputs`) | `checkAIOutputs` | `ai_outputs_cases.json` の `aot_poster_event`(`true`) |
| `aiSaveTo` | bool | generate の aiButton が list の項目を記録として保存する(`saveTo`) | `checkAISaveTo` | `ai_outputs_intent_cases.json` の `aox_packing_list_not_fields`(`true`) |
| `aiTask` | string | aiButton のどれかの task(`summarize` / `classify` / … / `generate`) | `checkAIGenerate` | `ai_conversation_cases.json` の `ac_diary_comment_not_chat`(`"suggest"`) |
| `alarm` | bool | 通知(`app.notifications` か scheduleNotification の button)のどれかが `alarm: true` | `checkAlarm` | `alarm_cases.json` の `al_create_wake`(`true`) |
| `alarmRecord` | bool | alarm の通知のどれかが記録(entity + field)から時刻を取る | `checkAlarm` | `alarm_cases.json` の `al_create_wake`(`true`) |
| `audio` | bool | フォームに audio の field と結んだ audioRecorder がある | `checkAudio` | `audio_cases.json` の `a_meeting`(`true`) |
| `audioInList` | bool | 一覧が audio の field を出す(行から再生できる) | `checkAudio` | `audio_cases.json` の `a_meeting`(`true`) |
| `authorShown` | bool | list / table / timeline のどれかが `$author` を出す | `checkExpect` | `cases.json` の `chat_group`(`true`) |
| `calc` | bool | 何かの計算がある(computed・stat.value・settlement・progressBar の target のどれか) | `checkExpect` | `cases.json` の `logic_warikan`(`true`) |
| `calendar` | bool | calendar の部品がある | `checkExpect` | `calendar_cases.json` の `cal_plain`(`true`) |
| `calendarAlert` | bool | addToCalendar のどれかが `calendarEvent.alertMinutes` を持つ | `checkCalendarAdd` | `calendar_cases.json` の `cal_deadline`(`true`) |
| `chart` | bool | chart の部品がある | `checkExpect` | `cases.json` の `kakeibo_a`(`true`) |
| `computed` | bool | `computed` の field がある | `checkExpect` | `adaptive_amount_intent_cases.json` の `aa_study_last_week`(`true`) |
| `countWhere` | bool | computed の field が、この記録の子のうち値で絞ったものだけを数える(agg count + `{"ref":"id"}` + 値の比較) | `checkCountWhere` | `schedule_poll_intent_cases.json` の `sp_poll_count`(`true`) |
| `dashcamAuto` | bool | dashcam のどれかが `autoStart` を持つ(手で押すボタンでなく条件で始まる) | `checkDashcam` | `dashcam_cases.json` の `dash_stand`(`true`) |
| `deletable` | string[] | 並べた entity ごとに、swipeToDelete の list か deleteRecord の button / toolbar がある | `checkNav` | `cases.json` の `warikan_members_split`(`["members"]`) |
| `editMembers` | bool | 共有の entity のどれかがメンバーも編集できる | `checkExpect` | `jphacks_products_cases.json` の `jp_priority_matrix`(`true`) |
| `enumField` | string | `entity.field` の形で、選択肢の field を決める(`optHas` / `optNot` / `fields` / `refPick` の前提) | `checkEnum` | `cases.json` の `enum_add`(`"income.kind"`) |
| `export` | string | button / toolbar が export を実行する。形式は `csv` / `pdf` / `both` | `checkExport` | `export_cases.json` の `x_kakeibo_csv`(`"csv"`) |
| `feed` | string | feed のどれかがこの提供元(`weather` / `warnings` / `pressure` / `holidays` / `rss`)を読む。`none` = feed が無い | `checkFeed` | `ai_input_feed_cases.json` の `aif_weather_outfit`(`"weather"`) |
| `feedRegion` | string | weather / warnings / pressure の feed がこの予報区のコードを使う | `checkFeed` | `ai_input_feed_cases.json` の `aif_weather_pressure_health`(`"170000"`) |
| `fields` | number | `enumField` の entity の field の数がちょうどこの数 | `checkEnum` | `cases.json` の `enum_add`(`4`) |
| `flashcardsUnlearned` | bool | flashcards が覚えた / まだを toggle に保存し、filter で覚えたカードを外す | `checkFlashcards` | `flashcards_cases.json` の `fc_vocab`(`true`) |
| `followButton` | bool | follow / unfollow の action がある | `checkFollows` | `follow_cases.json` の `fo_sns`(`true`) |
| `followEntity` | bool | 手作りのフォローの entity がある。false = 関係を appconfig に任せる | `checkFollows` | `follow_cases.json` の `fo_sns`(`false`) |
| `followFilter` | bool | 部品がフォローした人だけを出す(`{"myFollows": true}`) | `checkFollows` | `follow_cases.json` の `fo_sns`(`true`) |
| `grid` | bool | gridTable がある | `checkExpect` | `cases.json` の `grid_timetable`(`true`) |
| `groupedTable` | bool | table が groupBy と集計の列を 1 つ以上持つ | `checkGroupBy` | `groupby_cases.json` の `g_ranking`(`true`) |
| `growTo` | number | `keepEntity` の field の数がこの数に届く。届かない時は summary が上限(`上限` / `limit`)に触れていれば通す | `checkKeep` | `cases.json` の `field_cap_add2`(`21`) |
| `has` | string[] | 並べた部品の type がどこかの画面にある | `checkNav` | `add_minutes_cases.json` の `addmin_depart_transit`(`["openUrlButton"]`) |
| `health` | string | この metric(`steps` / `weight`)の healthStat がある | `checkExpect` | `cases.json` の `health_steps`(`"steps"`) |
| `healthField` | bool | health が `weight` の field がある | `checkExpect` | `cases.json` の `health_weight`(`true`) |
| `jitter` | bool | `app.notifications` のどれかが `jitterMinutes` を持つ | `checkNotifyNear` | `notify_near_cases.json` の `nn_create_station`(`false`) |
| `keep` | string[] | 編集の後も `keepEntity` に残っているべき field の id | `checkKeep` | `cases.json` の `field_cap_add2`(`["date", "height", "weight", "bmi", "…`) |
| `keepEntity` | string | `keep` / `growTo` を見る entity の id(その 2 つの前提) | `checkKeep` | `cases.json` の `field_cap_add2`(`"checkup"`) |
| `lacks` | string[] | 並べた部品の type がどの画面にも無い | `checkNav` | `ai_conversation_cases.json` の `ac_counselor`(`["aiButton"]`) |
| `liveTimer` | bool | timerDisplay のどれかが `liveActivity: true` | `checkLiveActivity` | `live_activity_cases.json` の `la_ramen`(`true`) |
| `locationAddress` | bool | locationButton のどれかが `addressField` を持つ(緯度経度から住所) | `checkPlaceSearch` | `place_search_cases.json` の `ps_here_address`(`true`) |
| `lookupBook` | bool | button / toolbar が lookupBook を実行し、書名を field に入れる | `checkBookLookup` | `book_cases.json` の `book_lending_isbn`(`true`) |
| `mark` | bool | list の項目のどれかが `item.mark`(いいね・保存の切り替え)を持つ | `checkMarks` | `jphacks_products2_cases.json` の `jp2_class_assignments`(`true`) |
| `markedByMe` | bool | 部品のどれかが `$marks.<id>` を `{"me": true}` で絞る | `checkMarks` | `jphacks_products2_cases.json` の `jp2_class_assignments`(`true`) |
| `markEntity` | bool | いいね・保存を手作りの子の entity で作っている。false = 作っていない | `checkMarks` | `m0disc_cases.json` の `d_sns_post`(`false`) |
| `masterTappable` | string[] | 並べた entity ごとに、行のタップでフォームを開ける(`a\|b` はどちらかの id) | `checkMasterTappable` | `jphacks_products2_cases.json` の `jp2_keep_in_touch`(`["friend\|friends"]`) |
| `matchMatched` | bool | match が `matched` の entity がある | `checkMatching` | `matching_cases.json` の `mt_hobby`(`true`) |
| `matchProfile` | bool | match が `profile` の entity がある | `checkMatching` | `jphacks_coverage_cases.json` の `cov_conference_meet`(`true`) |
| `matchSafe` | bool | profile に連絡先・年齢・性別・誕生日・学校・住所・本名に見える field が無い(`dsl.ProfileSensitiveField` と同じ語) | `checkMatching` | `jphacks_coverage_cases.json` の `cov_conference_meet`(`true`) |
| `matchUI` | bool | profile の list が `item.interest` を持ち、matchList が届いた人かつながった人を出す | `checkMatching` | `matching_cases.json` の `mt_hobby`(`true`) |
| `meFilter` | bool | 部品のどれかが `$author` で自分の分だけに絞る | `checkExpect` | `m0disc_cases.json` の `d_edit_mine_only`(`true`) |
| `musicAutoAccept` | bool | musicPlayer のどれかが `autoAccept` を持つ | `checkMusic` | `music_cases.json` の `mu_drive_auto`(`true`) |
| `musicRequests` | bool | musicPlayer のどれかが `allowRequests` を持つ(友だちが曲をリクエストできる) | `checkMusic` | `jphacks_coverage_cases.json` の `cov_car_dj`(`true`) |
| `nameInput` | bool | 共有の entity に、人の名前を手で打つ field がある | `checkExpect` | `cases.json` の `chat_group`(`false`) |
| `nav` | string[] | `from>to` ごとに、`from` の button / toolbar / 行のタップが `to` を開く | `checkNav` | `cases.json` の `nav_edit_link`(`["stats>expense_form"]`) |
| `nextOccurrence` | bool | 式のどこかで op `nextOccurrence` を使う | `checkRepeat` | `repeat_cases.json` の `nr_create_subs`(`true`) |
| `noOmission` | bool | サーバーが「省いた」と書き足した部分(`Meta.Omissions`)が無い | `runCase` | `clarify_cases.json` の `cl_gen_water_remind`(`true`) |
| `noShared` | bool | shared / public の entity が無い | `checkExpect` | `ai_conversation_cases.json` の `ac_counselor`(`true`) |
| `notification` | string | この時刻(`HH:MM`)の通知がある。`any` = 時刻を問わない | `checkExpect` | `alarm_cases.json` の `al_create_medicine`(`"any"`) |
| `notifyNear` | bool | `app.notifications` のどれかが location の field に `near` を持つ | `checkNotifyNear` | `notify_near_cases.json` の `nn_create_station`(`true`) |
| `notifyNearRepeat` | string | near の通知の repeat(`once` / `daily`) | `checkNotifyNear` | `notify_near_cases.json` の `nn_create_station`(`"once"`) |
| `notifyWhen` | bool | `app.notifications` のどれかが `when` を持つ(条件の通知) | `checkNotifyWhen` | `demo_script_cases.json` の `ds_c1_total_over`(`true`) |
| `notifyWhenOp` | string | その `when` の一番外の op(`lt` / `gt` / `gte` …) | `checkNotifyWhen` | `demo_script_cases.json` の `ds_c1_total_over`(`"gt"`) |
| `oneAdd` | bool | toolbar の ＋ と画面のボタンが同じフォームを重ねて開かない | `checkExpect` | `cases.json` の `kakeibo_a`(`true`) |
| `optHas` | string[] | `enumField` の選択肢にどれも入っている | `checkEnum` | `cases.json` の `enum_add`(`["副業", "ボーナス"]`) |
| `optNot` | string[] | `enumField` の選択肢にどれも入っていない | `checkEnum` | `cases.json` の `enum_remove`(`["ボーナス", "その他"]`) |
| `passby` | bool | nearbyShare のどれかが mode `passby` で field を明示している | `checkNearby` | `nearby_cases.json` の `n_passby_profile`(`true`) |
| `passbyDistance` | bool | passby の nearbyShare が `distance: true` | `checkNearby` | `nearby_cases.json` の `n_distance_friends`(`true`) |
| `patch` | bool | 編集が全体の書き直しでなく JSON Patch で保存された | `checkNav` | `ai_generate_cases.json` の `ag_batsu_five`(`true`) |
| `photo` | bool | photoField か image がある | `checkExpect` | `ai_photo_input_cases.json` の `ap_prescription_read`(`true`) |
| `photoAnalyze` | string[] | photoField の `analyze` に並べた kind がすべてある(レシートなら `amount`) | `checkPhotoAI` | `ai_photo_input_cases.json` の `ap_receipt_amount`(`["amount"]`) |
| `placeSearch` | string | 名前や場所を埋める placeSearch の探し方(`category` / `query` / `free` / `any`) | `checkPlaceSearch` | `place_search_cases.json` の `ps_cowork`(`"query"`) |
| `prefillRef` | bool | navigate のどれかが子のフォームを `prefillRef` で開く(親 → 子) | `checkExpect` | `jphacks_products_cases.json` の `jp_notebook_lists`(`true`) |
| `qrSource` | string | qrCode の値の出どころ(`text` / `field` / `value` / `record` / `any`) | `checkQR` | `qr_cases.json` の `q_member_card`(`"record"`) |
| `ref` | bool | ref か refs の field がある | `checkExpect` | `cases.json` の `logic_warikan`(`true`) |
| `refPick` | bool | `enumField` がマスターの entity への ref になり、そのフォームの画面と初めの選択肢がある | `checkEnum` | `cases.json` の `enum_user`(`true`) |
| `refs` | bool | type `refs`(複数選ぶ)の field がある | `checkNav` | `cases.json` の `warikan_members_split`(`true`) |
| `repeat` | string | 記録から作る `app.notifications` のどれかの repeat(`monthly` / `yearly`) | `checkRepeat` | `repeat_cases.json` の `nr_create_subs`(`"monthly"`) |
| `repeatOffset` | number | その通知の `offsetMinutes`(`-1440` = 前の日) | `checkRepeat` | `repeat_cases.json` の `nr_create_subs`(`-1440`) |
| `retention` | number | entity のどれかの `retentionDays` がこの数。`0` = どの entity にも無い | `checkRetention` | `retention_cases.json` の `rt_create_diary_week`(`7`) |
| `retentionOnRef` | bool | ref / refs が指す entity(マスター)が `retentionDays` を持つ | `checkRetention` | `retention_cases.json` の `rt_create_diary_week`(`false`) |
| `routeDest` | string | routeGuide の行き先の取り方(`entity` / `record` / `address` / `any`) | `checkRoute` | `route_cases.json` の `r_drive`(`"any"`) |
| `routeMode` | string | routeGuide の始めの手段(`automobile` / `walking`。空は `automobile`) | `checkRoute` | `jphacks_coverage_cases.json` の `cov_shelter_guide`(`"walking"`) |
| `rowWindow` | bool | computed の field が、自分の行の日付から見た期間で別の entity を集計する(例 `addDays(start,-7)`) | `checkRowWindow` | `adaptive_amount_intent_cases.json` の `aa_study_last_week`(`true`) |
| `ruleAction` | string | rule のどれかの action(`notify` / `createRecord` / `updateRecord` / `increment`) | `checkServerRules` | `jphacks_coverage_cases.json` の `cov_family_quiz`(`"notify"`) |
| `ruleAgg` | bool | rule のどれかが agg で記録を読む | `checkServerRules` | `jphacks_coverage_cases.json` の `cov_family_quiz`(`true`) |
| `ruleEach` | bool | rule のどれかが `on.each` を持つ | `checkServerRules` | `jphacks_coverage_cases.json` の `cov_study_rank`(`true`) |
| `ruleLookup` | string | rule のどれかの lookup の op(`exists` / `notExists` / `overlaps` / `notOverlaps`) | `checkServerRules` | `server_rules_cases.json` の `sr_roster_lookup`(`"notExists"`) |
| `rulePick` | string | rule のどれかが 1 件を選ぶ順(`random` / `oldest` / `newest` / `fewest` / `most`) | `checkServerRules` | `pick_fewest_intent_cases.json` の `pfx_family_chores`(`"fewest"`) |
| `ruleSim` | bool | すべての rule が作った記録で模擬の実行(`engine.PlanRule`)をエラー無しで通る | `checkServerRules` | `pick_fewest_intent_cases.json` の `pfx_family_chores`(`true`) |
| `ruleSimilar` | bool | rule のどれかが一番似た記録を探す(`similar`) | `checkServerRules` | `server_rules_cases.json` の `sr_similar_profiles`(`true`) |
| `ruleTo` | string | notify のどれかの宛先(`owner` / `author` / `authorOf` / `members` / `recipients`) | `checkServerRules` | `addressed_cases.json` の `ad_create_notify`(`"recipients"`) |
| `ruleTrigger` | string | rule のどれかのきっかけ(`created` / `updated` / `daily` / `weekly` / `monthly`) | `checkServerRules` | `jphacks_coverage_cases.json` の `cov_family_quiz`(`"daily"`) |
| `ruleWith` | bool | notify のどれかが `with` を持つ | `checkServerRules` | `server_rules_cases.json` の `sr_weekly_summary`(`true`) |
| `scope` | string | entity のどれかがこの scope(`public` / `shared` / `private`) | `checkExpect` | `cases.json` の `chat_group`(`"public"`) |
| `scopeNot` | string | どの entity もこの scope でない(「家族だけ」を public にしない) | `checkExpect` | `m0disc_cases.json` の `d_circle_attend`(`"public"`) |
| `sensor` | string[] | 並べたセンサーがすべて、どれかの sensorValue の sensor になっている | `checkSensor` | `sensor_cases.json` の `s_level`(`["tilt"]`) |
| `sensorDecline` | bool | sensorValue が無く、summary が「iPhone では測れない」と正直に言う | `checkSensor` | `sensor_cases.json` の `s_temperature`(`true`) |
| `sensorSave` | bool | sensorValue のどれかが entity に保存する | `checkSensor` | `sensor_cases.json` の `s_pressure_log`(`true`) |
| `sensorThreshold` | bool | sensorValue のどれかが threshold を持つ | `checkSensor` | `sensor_cases.json` の `s_tilt_alert`(`true`) |
| `serverRules` | bool | `app.serverRules` が空でない(`-server-rules` を付けて回す) | `checkServerRules` | `jphacks_coverage_cases.json` の `cov_family_quiz`(`true`) |
| `setting` | bool | 式のどこかが `{"setting": ...}` を読む | `checkSingle` | `m0disc_cases.json` の `d_period`(`true`) |
| `settlement` | bool | settlement の部品がある | `checkExpect` | `cases.json` の `logic_warikan`(`true`) |
| `shared` | bool | shared か public の entity がある(どちらでもよい) | `checkExpect` | `follow_cases.json` の `fo_sns`(`true`) |
| `single` | bool | entity のどれかが `single: true`(設定のような 1 件だけの記録) | `checkSingle` | `m0disc_cases.json` の `d_period`(`true`) |
| `statValue` | bool | `value` を持つ stat がある | `checkExpect` | `cases.json` の `logic_budget`(`true`) |
| `substitution` | bool | summary が代わりにした・できないと言う(`代替` / `できません` / `replaced` など) | `checkExpect` | `bracket_cases.json` の `br_tournament`(`false`) |
| `summaryHas` | string[] | summary に並べた文字がすべて入っている | `checkExpect` | `m0disc_cases.json` の `d_circle_attend`(`["招待した人だけが見られます"]`) |
| `summaryNot` | string[] | summary に並べた文字がどれも入っていない | `checkExpect` | `matching_cases.json` の `mt_not_dating`(`["出会い系サイト規制法"]`) |
| `tabHas` | string | 1 つ目以外のタブの画面にこの type の部品がある | `checkNav` | `cases.json` の `nav_edit_to_tab`(`"settlement"`) |
| `tabOrder` | string | タブの画面の並びが `a,b` で始まる | `checkNav` | `cases.json` の `nav_edit_reorder`(`"stats,home"`) |
| `tabs` | number | タブの数がちょうどこの数 | `checkNav` | `cases.json` の `nav_edit_add_tab`(`3`) |
| `tabsMin` | number | タブの数がこの数以上 | `checkNav` | `cases.json` の `nav_study`(`3`) |
| `textHighlight` | string | text の highlight の出どころ(`words` = 決まった語 / `field` = 開いた記録の field / `any`) | `checkTextHighlight` | `jphacks_coverage_cases.json` の `cov_error_query`(`"field"`) |
| `unique` | bool | entity のどれかが空でない `unique`(field の id か `$author`)を持つ | `checkUnique` | `jphacks_products_cases.json` の `jp_slot_booking`(`true`) |
| `unreachable` | number | どのタブ・最初の画面からも行けない画面の数(`dsl.Unreachable`) | `checkNav` | `add_minutes_cases.json` の `addmin_depart_transit`(`0`) |

## 3. 表とコードのずれを見つける

`backend/` の外(repo の一番上)で打つ。何も出なければ一致している(`<` はコードだけ・`>` は表だけにあるキー)。

```sh
diff <(grep -hoE '^\s*"[A-Za-z]+": +expect[A-Za-z]+|expectKeys\["[A-Za-z]+"\] =' backend/cmd/ai-eval/*.go | grep -oE '[A-Za-z]+"' | tr -d '"' | sort -fu) <(grep -oE '^\| `[A-Za-z]+`' docs/backend/ai-eval/expect-keys.md | tr -d '|` ' | sort -fu)
```

## 4. キーを足す時

1. 判定を `backend/cmd/ai-eval/<機能>.go` に書き、`init()` で `expectKeys["<キー>"] = expect…` を足す(最近のキーはこの形。`expect.go` の大きな表に足すより並行の PR とぶつかりにくい)。
2. `runCase` から判定の関数を呼ぶ。
3. この表に 1 行足す(名前順の位置)。
4. `go run ./cmd/ai-eval -check-all internal/ai/eval` で、ケースの形が通ることを確かめる。
