export interface EventCopy {
  title: string;
  hint: string;
  details?: string;
  fallbackTitle?: string;
}

export const eventJa = {
  "CONFIG_AUTO_IMPORTED": {
    "title": "プロジェクトファイルを自動読込",
    "hint": ""
  },
  "DEVICE_MISSING": {
    "title": "登録機器が未検出: {name}",
    "hint": "対象機器の電源が入っているか確認。LANケーブルの抜け・緩みを確認して差し直す。",
    "details": "登録済みMAC ({mac}) が一度観測された後、連続ポーリングで未検出",
    "fallbackTitle": "登録機器が未検出"
  },
  "PING_FAIL": {
    "title": "Ping応答なし: {name} ({last_ip})",
    "hint": "機器の電源とLANケーブル接続を確認。動作に問題がないのに出る場合は、機器のICMP応答設定やネットワーク経路を確認。",
    "details": "{last_ip} へのPing (ICMP) が無応答",
    "fallbackTitle": "Ping応答なし"
  },
  "IP_MISMATCH": {
    "title": "IPアドレスが予定IPと不一致: {name} ({last_ip} ≠ 予定 {planned_ip})",
    "hint": "機器本体のIPアドレス設定が予定表どおりか確認。別のポートや別VLANに差し間違えていないか確認。",
    "details": "観測IP {last_ip} と登録済み予定IP {planned_ip} が不一致",
    "fallbackTitle": "IPアドレスが予定IPと不一致"
  },
  "LINK_DOWN": {
    "title": "リンク断が発生",
    "hint": "ケーブルの両端が奥まで刺さっているか確認して差し直す。接続機器の電源が入っているか確認。",
    "details": "ポートのリンクダウンを検知"
  },
  "LINK_UP": {
    "title": "リンクが復帰",
    "hint": "接続機器の音声・映像・制御が正常に戻ったか確認。切断を繰り返す場合は予備ケーブルに交換する。",
    "details": "ポートのリンクアップを検知"
  },
  "LINK_FLAP": {
    "title": "リンクフラップ（瞬断と復帰）を検知 ({count}回)",
    "hint": "コネクタの緩み・ツメ折れ・ケーブルの踏まれを確認。予備ケーブルへの交換と接続機器の電源を確認。",
    "details": "linkDown → linkUp のSNMP Trap、またはifLastChangeの変化からリンクフラップを検知しました。",
    "fallbackTitle": "リンクフラップ（瞬断と復帰）を検知"
  },
  "SWITCH_UNREACHABLE": {
    "title": "スイッチとSNMP通信不可",
    "hint": "スイッチの電源ランプと電源ケーブルを確認。アップリンク（スイッチ間リンク）と管理ネットワークの接続・SNMP設定を確認。",
    "details": "スイッチへのSNMPポーリングが到達不能・無応答"
  },
  "SWITCH_RECOVERED": {
    "title": "スイッチとのSNMP通信が復帰",
    "hint": "このスイッチに接続された機器が正常に動作しているか確認。電源ケーブルやアップリンクの緩み・履歴を確認。",
    "details": "スイッチへのSNMP到達性が復帰"
  },
  "SWITCH_REBOOT": {
    "title": "スイッチが再起動",
    "hint": "電源ケーブルの抜け・瞬停・誤操作がなかったか確認。接続機器が正常に復帰しているか確認。",
    "details": "sysUpTimeの巻き戻りを検知しました。電源瞬断、クラッシュ、手動再起動の可能性があります。"
  },
  "HIGH_UTIL": {
    "title": "ポートの帯域使用率がしきい値を超過",
    "hint": "意図しない映像・音声ストリームが集中していないか確認。帯域配分とQoSを確認。",
    "details": "ポート送受信帯域使用率がしきい値以上"
  },
  "DISCARDS": {
    "title": "パケットドロップ (Discards) が増加 (+{delta})",
    "hint": "音切れや映像の乱れを確認。増加が続く場合はStorm ControlとQoSを確認。",
    "details": "パケットドロップ (Discards) 増加 (+{delta})・輻輳/Storm Control/QoSキュー溢れの疑い",
    "fallbackTitle": "パケットドロップ (Discards) が増加"
  },
  "SPEED_BELOW_EXPECTED": {
    "title": "リンク速度が期待速度を下回っています",
    "hint": "LANケーブルを抜き差しし、改善しなければ予備ケーブルに交換する。中継アダプタ・ケーブル規格・オートネゴシエーション設定を確認。",
    "details": "リンク速度が期待速度未満・芯線断線(4芯落ち)/圧着不良/オートネゴ不整合の疑い"
  },
  "RELINK_SLOWER": {
    "title": "再リンク後にリンク速度が低下",
    "hint": "コネクタの破損や緩みを確認し、予備ケーブルに交換する。接続機器側のポートやオートネゴシエーション設定が変わっていないか確認。",
    "details": "再リンク後にリンク速度が以前より低下・芯線断線/コネクタ不良/オートネゴ不整合の疑い"
  },
  "SPEED_CHANGE": {
    "title": "リンク速度が変化",
    "hint": "接続機器の音声・映像・制御に問題がないか確認。意図しない変化なら期待速度とケーブル状態を確認し、必要なら交換する。",
    "details": "ポートのリンク速度変更を検知"
  },
  "POE_PORT_FAULT": {
    "title": "このポートでPoE障害が発生中",
    "hint": "対象ポートのLANケーブルを抜き、破損・水濡れ・挟まれ・短絡がないか確認。予備ケーブルでも改善しない場合は受電機器とPoEバジェットを確認。",
    "details": "ポートのPoE障害を検知・PoE電力不足/受電機器異常/ケーブル短絡の疑い"
  },
  "POE_FAULT": {
    "title": "PoE障害が発生",
    "hint": "PoE受電機器とLANケーブルを確認。PoEバジェットを確認。",
    "details": "PoE障害を検知・PoEバジェット不足/受電機器異常/ケーブル短絡の疑い"
  },
  "POE_STATE": {
    "title": "PoE給電状態が変化",
    "hint": "接続機器の電源ランプと動作状態を確認。給電が止まっている場合はケーブルを差し直し、PoEポート設定を確認。",
    "details": "ポートのPoE給電状態変更を検知"
  },
  "PHY_ERRORS": {
    "title": "物理エラーが増加 (FCS+{fcs} / Symbol+{symbol} / Align+{align})",
    "hint": "ケーブルを差し直し、可能なら予備ケーブルに交換する。SFP（光リンク）の場合は光端面を清掃し、強い曲げがないか確認。",
    "details": "物理層エラー増加 (FCS+{fcs} / Symbol+{symbol} / Align+{align})・正常時は完全ゼロが基準",
    "fallbackTitle": "物理エラーが増加"
  },
  "MTU_MISMATCH": {
    "title": "MTU不一致の疑いあり（フレーム長超過を検知）",
    "hint": "接続機器と対向スイッチのMTU設定が一致しているか確認。",
    "details": "FrameTooLongsカウンタ増加・接続機器または対向スイッチとのMTU不一致の疑い"
  },
  "DUPLEX": {
    "title": "Half-Duplex動作またはLate Collisionを検知",
    "hint": "LANケーブルを差し直す。改善しない場合は両端のDuplexとオートネゴシエーションを確認。",
    "details": "Half-Duplex動作またはLate Collision増加・片側固定/片側オートのDuplex不整合疑い"
  },
  "COS_DROP": {
    "title": "CoSキュードロップが増加",
    "hint": "音切れや映像の乱れを確認。帯域使用率とQoS優先度を確認。",
    "details": "CoSキュードロップ増加・帯域飽和/マイクロバースト/QoS優先度設定の要確認"
  },
  "PAUSE_FRAMES": {
    "title": "PAUSEフレームが増加",
    "hint": "音声・映像の遅延や途切れを確認。このポートのFlow Control設定を確認。",
    "details": "PAUSEフレーム受信/送信カウンタ増加・Flow Control作動によるストリーム遅延の恐れ"
  },
  "SFP_LOS": {
    "title": "SFPの光信号喪失 (LOS) または送信障害 (TxFault) を検知",
    "hint": "光ファイバの抜け・強い曲げ・断線がないか確認。光端面を清掃して差し直し、直らなければSFPモジュールを交換する。",
    "details": "SFP光信号喪失 (LOS) または送信障害 (TxFault) を検知"
  },
  "SFP_LOW_RX": {
    "title": "SFPの受信光パワーが低下 ({rx_dbm} dBm)",
    "hint": "光ファイバが強く曲がったり踏まれたりしていないか確認。差し直せるタイミングで光端面を清掃して再挿抜し、改善しなければSFPやケーブルを交換する。",
    "details": "SFP受信光パワーがしきい値未満 ({rx_dbm} dBm)",
    "fallbackTitle": "SFPの受信光パワーが低下"
  },
  "SFP_RX_DROP": {
    "title": "SFPの受信光パワーがベースライン（基準値）から低下",
    "hint": "光ファイバに新たな曲げ・引っ張り・踏まれがかかっていないか確認。コネクタの緩みや汚れを確認し、安全なタイミングで清掃・再挿抜する。",
    "details": "SFP受信光パワーが記録済みベースラインからしきい値以上低下"
  },
  "MSRP_RESV_FAILED": {
    "title": "AVB / Milan の MSRP 帯域予約に失敗",
    "hint": "経路上の機器がAVBに対応しているか確認。空き帯域とVLAN設定を確認。",
    "details": "AVB / Milan のMSRP帯域予約失敗を検知・帯域不足/非対応機器/VLAN不一致の疑い"
  },
  "MVR_RX_FAIL": {
    "title": "MVR の IGMP パケット受信失敗が増加",
    "hint": "受信側で音声・映像の途切れを確認。MVRのポートとVLAN設定を確認。",
    "details": "MVR IGMPパケット受信失敗カウンタ増加・ソース/レシーバーポートやMVR VLAN不整合の疑い"
  },
  "CPU_HIGH": {
    "title": "スイッチのCPU使用率がしきい値を超過",
    "hint": "物理ループや異常なトラフィックがないか確認。",
    "details": "スイッチCPU使用率 (5秒平均) がしきい値以上"
  },
  "MEM_LOW": {
    "title": "スイッチの空きメモリが低下",
    "hint": "スイッチや接続機器の応答を確認。空きメモリの推移を確認。",
    "details": "スイッチ空きメモリ率がしきい値以下"
  },
  "TEMP_HIGH": {
    "title": "スイッチの温度が異常",
    "hint": "ラックの吸気・排気口が物や布で塞がれていないか確認。扉を開ける・送風するなど冷却し、周囲温度を下げる。",
    "details": "スイッチ内部温度しきい値超過または温度ステータス異常 (warning/critical/shutdown)"
  },
  "FAN_FAIL": {
    "title": "スイッチのファンに異常あり",
    "hint": "スイッチの冷却ファンが回っているか、異物で止まっていないか確認。温度上昇に注意し、外部送風や予備スイッチを準備する。",
    "details": "スイッチ内蔵ファン異常 (failed / nopower / notpowering / incompatible)"
  },
  "PSU_FAIL": {
    "title": "スイッチのPSU（電源ユニット）に異常あり",
    "hint": "電源ケーブルが奥まで刺さっているか、コンセントに通電しているか確認。冗長PSU構成なら異常側を確認し、予備手配を検討する。",
    "details": "スイッチ電源ユニット (PSU) 異常 (failed / nopower / notpowering / incompatible)"
  },
  "MBUF_ALLOC_FAIL": {
    "title": "Mbuf（スイッチ内部のパケットバッファ）の確保に失敗",
    "hint": "音切れや映像の乱れを確認。帯域使用率とQoS設定を確認。",
    "details": "Mbuf確保失敗カウンタ増加・輻輳/マイクロバースト/QoS設定の要確認"
  },
  "POE_BUDGET": {
    "title": "PoEバジェット（給電容量上限）の使用率が上限に近づいています",
    "hint": "このスイッチにPoE受電機器を追加で接続しない。可能なら一部機器をACアダプタ給電または別スイッチへ移す。",
    "details": "スイッチ全体のPoEバジェット使用率がしきい値以上"
  },
  "UDLD": {
    "title": "UDLD（片方向リンク検知）によりポートが遮断されました",
    "hint": "光ファイバ2芯のうち片方が抜けていないか、折れていないか確認。端面を清掃して差し直すか、予備の光ファイバ・SFPに交換する。",
    "details": "UDLD片方向リンク障害によりポート遮断・光ファイバ片芯断線/Tx・Rx異常の疑い"
  },
  "STP_BLOCKING": {
    "title": "STP Blocking によりポートが遮断中",
    "hint": "ループ接続がないか確認。抜線する前に意図した冗長経路か確認。",
    "details": "リンクアップ中にSTP状態がdiscarding (Blocking)・物理ループまたは冗長経路遮断"
  },
  "STP_TCN_BURST": {
    "title": "STPトポロジー変更 (TCN) が急増中",
    "hint": "リンクフラップやループ接続がないか確認。",
    "details": "STPトポロジー変更通知 (TCN) の急増を検知・物理ループやリンク断続の疑い"
  },
  "DHCP_ROGUE": {
    "title": "不正DHCPサーバーのメッセージが増加",
    "hint": "意図しないルーターやDHCPサーバーがないか確認。",
    "details": "不正DHCPサーバーメッセージ増加・持込ルーター/二重DHCPの疑い"
  },
  "IP_CONFLICT": {
    "title": "IPアドレスが衝突中",
    "hint": "直前に追加・変更した機器を確認。固定IPとDHCP割当の重複を確認。",
    "details": "固定IPとDHCP割当などのIPアドレス衝突を検知"
  },
  "AUDIT_PENDING": {
    "title": "全スイッチの初回設定情報を取得中",
    "hint": "初回のSlow周期が終わるまでお待ちください。続く場合はSNMP応答を確認。",
    "details": "全スイッチの初回Slow周期取得待ちのため横断監査を保留中"
  },
  "DHCP_DOUBLE": {
    "title": "複数のスイッチでDHCPサーバーが稼働中",
    "hint": "DHCPサーバーの設定とVLANごとの割当範囲を確認。",
    "details": "複数スイッチでDHCPサーバー機能が有効（VLAN別構成の判定は未実施）"
  },
  "SNOOP_GLOBAL_OFF": {
    "title": "IGMP Snooping 全体設定がOFF（VLAN {vlans} の設定が効いていない）",
    "hint": "スイッチ全体のIGMP Snooping設定を確認。",
    "details": "VLANではIGMP Snoopingを使う設定だが、スイッチ全体の設定がOFFのため全ポートへマルチキャストが流れる"
  },
  "SNOOP_MISMATCH": {
    "title": "同一VLAN内で IGMP Snooping のON/OFFがスイッチ間で不一致",
    "hint": "同じVLANのIGMP Snooping設定を各スイッチで確認。",
    "details": "同一VLAN内でIGMP Snooping有効スイッチと無効スイッチが混在"
  },
  "QUERIER_ABSENT": {
    "title": "IGMP Snooping 有効VLANに IGMPクエリアが不在",
    "hint": "コアスイッチなどのIGMPクエリア設定をすぐ確認。",
    "details": "IGMP Snooping有効VLANでIGMPクエリア不在（1〜5分後にマルチキャスト途絶の恐れ）"
  },
  "QUERIER_CONFLICT": {
    "title": "同一VLAN内で複数のIGMPクエリアIPを観測",
    "hint": "IGMPクエリの送信元とIGMPクエリアの選出状態を確認。",
    "details": "同一VLAN内で複数のIGMPクエリアIPを観測"
  },
  "IGMP_VERSION_MIX": {
    "title": "同一VLAN内で複数のIGMPバージョンが混在",
    "hint": "AV機器の受信状態と必要なIGMPバージョンを確認。",
    "details": "同一VLAN内で複数のIGMPバージョンを観測"
  },
  "UPLINK_MTU_MISMATCH": {
    "title": "アップリンク両端のMTUが不一致",
    "hint": "",
    "details": "大きなフレームを通す場合は、LLDPで隣接する両端の設定を揃える構成が推奨される場合があります。"
  },
  "QUERIER_CHANGED": {
    "title": "IGMPクエリアのIPが切り替わりました",
    "hint": "マルチキャスト配信の状態とIGMPクエリの送信元を確認。",
    "details": "VLAN内のIGMPクエリアIPが変化・クエリア選出変動や外部機器クエリ送信の疑い"
  },
  "MCAST_FLOOD": {
    "title": "未登録ポートへマルチキャストがフラッディングしている疑いあり",
    "hint": "IGMP SnoopingとIGMPクエリアの状態を確認。",
    "details": "IGMPグループ未登録の過半(2ポート以上)へ20,000 pps超のマルチキャスト送出・フラッディング疑い"
  },
  "DEVICE_NEW": {
    "title": "新しい機器（MACアドレス）を検出",
    "hint": "心当たりのある機器が予定どおりのポートに接続されているか確認。覚えのない機器なら接続先の実物を確認。",
    "details": "新規MACアドレス／機器をFDB上で検出"
  },
  "DEVICE_GONE": {
    "title": "接続していた機器がFDB（MACアドレステーブル）から消えました",
    "hint": "対象機器の電源やLANケーブルが抜かれていないか確認。意図して外した場合は対応不要。",
    "details": "FDBからMACアドレスの観測終了（電源断・抜線・FDBエージング）"
  },
  "PHASE_CHANGED": {
    "title": "運用フェーズが変更されました: {phase}",
    "hint": "現在の運用フェーズ（設営・リハ・本番）と表示が一致しているか確認。誤操作ならヘッダーから元の運用フェーズに戻す。",
    "details": "運用フェーズ変更 ({phase})・フェーズ連動の定期Pingおよびアラート抑止を切替",
    "fallbackTitle": "運用フェーズが変更されました"
  },
  "WORK_STARTED": {
    "title": "作業中を開始: {group}",
    "hint": "作業中は対象グループのアラートを抑止し。"
  },
  "WORK_ENDED": {
    "title": "作業中を終了: {group}",
    "hint": "対象グループの監視を再開しました。"
  },
  "linkDown": {
    "title": "リンク断が発生（SNMP Trap）",
    "hint": "ケーブルの両端が奥まで刺さっているか確認して差し直す。接続機器の電源が入っているか確認。",
    "details": "SNMP Trap (linkDown) によりポートのリンク断を検知"
  },
  "linkUp": {
    "title": "リンクが復帰（SNMP Trap）",
    "hint": "接続機器の音声・映像・制御が正常に戻ったか確認。切断を繰り返す場合は予備ケーブルに交換する。",
    "details": "SNMP Trap (linkUp) によりポートのリンク復帰を検知"
  },
  "coldStart": {
    "title": "スイッチが電源投入から再起動（SNMP Trap）",
    "hint": "電源ケーブルの抜けや瞬停がなかったか確認。接続機器が正常に復帰したか確認。",
    "details": "SNMP Trap (coldStart) 受信・電源投入／コールドブートによる再起動"
  },
  "warmStart": {
    "title": "スイッチが再起動（SNMP Trap）",
    "hint": "再起動や設定反映の履歴を確認。接続機器の動作を確認。",
    "details": "SNMP Trap (warmStart) 受信・ソフトウェア／ウォームリスタートによる再起動"
  },
  "CLOCK_SKEW": {
    "title": "監視PCとの時刻差 {offset_seconds}秒",
    "hint": "スイッチと監視PCの時刻同期を確認。",
    "details": "Slow周期で取得したスイッチの時刻を監視PCと比較しました。",
    "fallbackTitle": "監視PCとの時刻差を検知"
  },
  "PORT_CONFIG_CHANGED": {
    "title": "{port} の {field} が変化",
    "hint": "意図した設定変更か確認。",
    "details": "変更前と変更後の値を比較してください。",
    "fallbackTitle": "ポート設定が変化"
  },
  "PRE_SHOW_CHECKLIST_REMAINDER": {
    "title": "本番開始時に確認が残っていました（{count}項目）",
    "hint": "本番前チェックの残項目を確認。",
    "details": "確認が残った状態で本番を開始したときの記録です。",
    "fallbackTitle": "本番開始時に確認が残っていました"
  },
  "BASELINE_SAVED": {
    "title": "ベースラインを保存: {name}",
    "hint": "",
    "details": "保存した状態を設定差分の比較に使えます。",
    "fallbackTitle": "ベースラインを保存"
  },
  "LAB_FAULT": {
    "title": "ラボで模擬障害を発生させました",
    "hint": "",
    "details": "対象 {target} / シナリオ {scenario}"
  },
  "TERMINAL_OPEN": {
    "title": "スイッチのターミナルを開きました",
    "hint": "",
    "details": "接続方式: {protocol}"
  },
  "MARKER": {
    "title": "マーカー: {note}",
    "hint": "",
    "details": "",
    "fallbackTitle": "マーカー"
  },
  "SWITCH_LOG": {
    "title": "スイッチのログを受信",
    "hint": "",
    "details": ""
  },
  "UNKNOWN_TRAP": {
    "title": "未定義のSNMP Trapを受信",
    "hint": "送信元を確認。",
    "details": "受信した通知のOIDが登録されていません。OID: {trap_oid}"
  },
  "authenticationFailure": {
    "title": "SNMP認証失敗",
    "hint": "SNMP設定を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "lldpRemTablesChange": {
    "title": "LLDP隣接機器の変化",
    "hint": "接続先と配線を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "pethPsePortOnOffNotification": {
    "title": "PoE給電状態の変化",
    "hint": "受電機器の電源を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "pethMainPowerUsageOnNotification": {
    "title": "PoE電力使用量がしきい値超過",
    "hint": "PoEバジェットと接続機器を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "pethMainPowerUsageOffNotification": {
    "title": "PoE電力使用量がしきい値未満に回復",
    "hint": "PoEバジェットの余裕を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "boxsFanStateChange": {
    "title": "ファン状態の変化",
    "hint": "ファンの動作を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "boxsPowSupplyStateChange": {
    "title": "電源ユニット状態の変化",
    "hint": "電源ケーブルとPSUを確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "boxsTemperatureChange": {
    "title": "温度の変化",
    "hint": "温度と通気を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "boxsThermalShutdown": {
    "title": "温度異常によるシャットダウン",
    "hint": "通気と周囲温度を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "boxsTemperatureStateChange": {
    "title": "温度状態の変化",
    "hint": "温度と通気を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "boxsDyingGaspEventChange": {
    "title": "電源喪失（Dying Gasp）",
    "hint": "電源ケーブルとコンセントを確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "broadcastStormStartTrap": {
    "title": "ブロードキャストストーム開始",
    "hint": "ループ接続と送信元を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "broadcastStormEndTrap": {
    "title": "ブロードキャストストーム終了",
    "hint": "スイッチと接続機器の動作を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "linkFailureTrap": {
    "title": "リンク障害",
    "hint": "ケーブルと対向機器を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "fanFailureTrap": {
    "title": "ファン障害",
    "hint": "ファンの動作と温度を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "stpInstanceNewRootTrap": {
    "title": "STPルートブリッジ変更",
    "hint": "STP Root Bridgeの選出結果を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "stpInstanceTopologyChangeTrap": {
    "title": "STPトポロジー変更",
    "hint": "リンクフラップやループ接続を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "powerSupplyStatusChangeTrap": {
    "title": "電源ユニット状態の変化",
    "hint": "電源ケーブルとPSUを確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "failedUserLoginTrap": {
    "title": "スイッチへのログイン認証失敗",
    "hint": "ログイン操作と認証設定を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "daiIntfErrorDisabledTrap": {
    "title": "DAIによるポートErrDisable",
    "hint": "接続機器とDAI設定を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "stpInstanceLoopInconsistentStartTrap": {
    "title": "STPループ不整合検知",
    "hint": "配線とSTP設定を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "stpInstanceLoopInconsistentEndTrap": {
    "title": "STPループ不整合解消",
    "hint": "該当ポートの通信を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "dhcpSnoopingIntfErrorDisabledTrap": {
    "title": "DHCP SnoopingによるポートErrDisable",
    "hint": "DHCP Snooping設定とDHCPメッセージの送信元を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "agentSwitchIpAddressConflictTrap": {
    "title": "IPアドレス重複検知",
    "hint": "固定IPとDHCP割当の重複を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "agentSwitchCpuRisingThresholdTrap": {
    "title": "CPU使用率上昇しきい値超過",
    "hint": "物理ループや異常なトラフィックを確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "agentSwitchCpuFallingThresholdTrap": {
    "title": "CPU使用率が回復",
    "hint": "スイッチの応答を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "agentSwitchCpuFreeMemBelowThresholdTrap": {
    "title": "空きメモリ低下",
    "hint": "スイッチの応答と空きメモリを確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "agentSwitchMbufRisingThresholdTrap": {
    "title": "Mbuf使用率上昇",
    "hint": "帯域使用率とQoS設定を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "agentSwitchMbufFallingThresholdTrap": {
    "title": "Mbuf使用率回復",
    "hint": "音声・映像の受信状態を確認。",
    "details": "SNMP Trapを受信しました。"
  },
  "EEE_ON_AV_PORT": {
    "title": "このポートで EEE（省電力イーサネット / 802.3az）が有効（LPI遷移: +{lpi}）",
    "hint": "",
    "details": "Dante / PTP 等の同期系 AV 用途では EEE 無効が推奨される場合があります。",
    "fallbackTitle": "このポートで EEE（省電力イーサネット / 802.3az）が有効"
  },
  "FLOWCTRL_ON_AV_PORT": {
    "title": "このポートで Flow Control (PAUSE) が有効（設定値: {pause}）",
    "hint": "",
    "details": "NDI 4 を除く AV 用途では Flow Control 無効が推奨される場合があります。",
    "fallbackTitle": "このポートで Flow Control (PAUSE) が有効"
  },
  "NDI4_NEEDS_FLOWCTRL": {
    "title": "ロール ndi4 のポートで Flow Control が無効",
    "hint": "",
    "details": "NDI 4 構成では Flow Control 有効が推奨／要件とされる場合があります。"
  },
  "DSCP_TRUST": {
    "title": "このポートの QoS Trust 設定は {trust}（trustIpDscp 以外）",
    "hint": "",
    "details": "Dante / AES67 / PTP の DSCP 優先制御を使う場合は Trust DSCP が推奨される場合があります。",
    "fallbackTitle": "このポートの QoS Trust 設定は trustIpDscp 以外"
  },
  "QSYS_MTU": {
    "title": "ロール qsys のポートで最大フレーム長が {max_frame}（1522超）",
    "hint": "",
    "details": "Q-SYS 環境では MTU 1500（最大フレーム長 1522）が要件とされる場合があります。",
    "fallbackTitle": "ロール qsys のポートで最大フレーム長が1522を超過"
  },
  "STORM_CTRL_UPLINK": {
    "title": "アップリンクポートで Storm Control（マルチキャスト／ブロードキャスト）が有効",
    "hint": "",
    "details": "高帯域マルチキャストや gigaACE / CobraNet のアップリンクではドロップ要因になり得るため無効が推奨される場合があります。"
  },
  "STP_MSTP": {
    "title": "このスイッチの STP バージョンは MSTP で稼働中",
    "hint": "",
    "details": "ProAV 用途では RSTP が推奨される場合があります。"
  },
  "FW_MISMATCH": {
    "title": "監視中のスイッチ間でファームウェアバージョンが混在（{versions}）",
    "hint": "",
    "details": "同一ネットワーク内のスイッチはファームウェア統一が推奨されています。",
    "fallbackTitle": "監視中のスイッチ間でファームウェアバージョンが混在"
  },
  "STP_PRIORITY_DEFAULT": {
    "title": "監視中の全スイッチで STP 優先度が既定値 32768 のまま",
    "hint": "",
    "details": "意図しない Root Bridge 選出を避けるため、コア=0／予備コア=8192 等の設定が推奨される場合があります。"
  },
  "STP_EDGE_ROOT": {
    "title": "アップリンク1本以下のスイッチが STP Root Bridge に選出済み",
    "hint": "",
    "details": "コアスイッチが STP Root Bridge になる優先度設計が推奨される場合があります。"
  },
  "QUERIER_ON_EDGE": {
    "title": "アップリンク1本以下（非ルート）のスイッチが VLAN {vlan} の IGMP クエリアに選出済み",
    "hint": "",
    "details": "コアスイッチを IGMP クエリアにする配置・IP設計が推奨される場合があります。",
    "fallbackTitle": "アップリンク1本以下の非ルートスイッチがIGMPクエリアに選出済み"
  },
  "DAISY_CHAIN": {
    "title": "STP Root Bridge からのスイッチ接続段数が最大 {depth} 段（3段以上）",
    "hint": "",
    "details": "SMPTE / AES67 / PTPv2 等の同期系ではスター型トポロジーが推奨される場合があります。",
    "fallbackTitle": "STP Root Bridgeからの接続段数が3段以上"
  },
  "VLAN_CROWDED": {
    "title": "VLAN {vlan} 内の非アップリンク接続機器数が {count} 台（200台超）",
    "hint": "",
    "details": "Q-SYS 等では 1 VLAN あたり 200 台以下が推奨される場合があります。",
    "fallbackTitle": "VLAN内の非アップリンク接続機器数が200台を超過"
  },
  "BCAST_STORM_CTRL_UPLINK": {
    "title": "アップリンクでブロードキャストのStorm Controlが有効",
    "hint": "",
    "details": "帯域の大きいブロードキャストではドロップ要因になる場合があります。"
  },
  "MCAST_STORM_CTRL_UPLINK": {
    "title": "アップリンクでマルチキャストのStorm Controlが有効",
    "hint": "",
    "details": "高帯域マルチキャストではドロップ要因になる場合があります。"
  }
} satisfies Record<string, EventCopy>;
