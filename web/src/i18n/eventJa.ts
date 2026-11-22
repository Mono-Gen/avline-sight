export const eventJa = {
  "CLOCK_SKEW": {
    "title": "監視PCとの時刻差 {offset_seconds}秒",
    "hint": "スイッチと監視PCの時刻同期を確認してください。"
  },
  "DEVICE_MISSING": {
    "title": "登録機器が見つかりません: {name} ({mac})",
    "hint": "機器の電源・ケーブル・接続先を確認してください。"
  },
  "PING_FAIL": {
    "title": "Ping無応答: {name} ({last_ip})",
    "hint": "電源・経路・ICMP応答設定を確認してください。"
  },
  "IP_MISMATCH": {
    "title": "予定IPと不一致: {name} ({last_ip} → {planned_ip})",
    "hint": "機器のIP設定と予定IPを確認してください。"
  },
  "LINK_DOWN": {
    "title": "リンク断",
    "hint": "ケーブル・コネクタ・接続機器の電源を確認してください。"
  },
  "LINK_UP": {
    "title": "リンク復帰",
    "hint": "接続機器の状態を確認してください。"
  },
  "LINK_FLAP": {
    "title": "一過性のリンク断 ({count}回)",
    "hint": "ケーブル・コネクタ・機器の電源を確認してください。"
  },
  "SWITCH_UNREACHABLE": {
    "title": "スイッチ応答なし",
    "hint": "スイッチの電源・接続・SNMP設定を確認してください。"
  },
  "SWITCH_RECOVERED": {
    "title": "スイッチ応答復帰",
    "hint": "スイッチの状態と障害履歴を確認してください。"
  },
  "SWITCH_REBOOT": {
    "title": "スイッチ再起動",
    "hint": "電源と再起動原因を確認してください。"
  },
  "HIGH_UTIL": {
    "title": "帯域使用率が高い",
    "hint": "輻輳・Storm Control・QoSを確認してください。"
  },
  "DISCARDS": {
    "title": "パケットドロップ +{delta}",
    "hint": "輻輳・Storm Control・QoSを確認してください。"
  },
  "SPEED_BELOW_EXPECTED": {
    "title": "期待速度未満",
    "hint": "ケーブル芯線・コネクタ・オートネゴを確認してください。"
  },
  "RELINK_SLOWER": {
    "title": "再接続後に速度低下",
    "hint": "ケーブル芯線・コネクタ・オートネゴを確認してください。"
  },
  "SPEED_CHANGE": {
    "title": "リンク速度変更",
    "hint": "期待速度とケーブルを確認してください。"
  },
  "POE_PORT_FAULT": {
    "title": "PoEポート障害",
    "hint": "PoE予算と受電機器・ケーブルの短絡を確認してください。"
  },
  "POE_FAULT": {
    "title": "PoE障害",
    "hint": "PoE予算と受電機器・ケーブルの短絡を確認してください。"
  },
  "POE_STATE": {
    "title": "PoE状態変更",
    "hint": "受電機器と電源状態を確認してください。"
  },
  "PHY_ERRORS": {
    "title": "物理エラー増加 FCS+{fcs} Symbol+{symbol} Align+{align}",
    "hint": "ケーブル・コネクタ・SFPの汚れや劣化を確認してください。"
  },
  "MTU_MISMATCH": {
    "title": "MTU不一致の疑い",
    "hint": "接続機器と対向スイッチのMTUを合わせてください。"
  },
  "DUPLEX": {
    "title": "Half-Duplex / Late Collision",
    "hint": "両端のDuplex設定とオートネゴを確認してください。"
  },
  "COS_DROP": {
    "title": "CoSキュードロップ",
    "hint": "帯域飽和・マイクロバースト・QoS優先度を確認してください。"
  },
  "PAUSE_FRAMES": {
    "title": "PAUSEフレーム増加",
    "hint": "Flow Controlを確認してください（NDI4以外のAVでは無効化推奨）。"
  },
  "EEE_ON_AV_PORT": {
    "title": "AVポートでEEE有効",
    "hint": "省電力イーサネットを無効にして同期への影響を確認してください。"
  },
  "SFP_LOS": {
    "title": "SFP光信号喪失 / 送信障害",
    "hint": "光端面清掃・曲げ・断線・モジュールを確認してください。"
  },
  "SFP_LOW_RX": {
    "title": "SFP受信光低下 {rx_dbm} dBm",
    "hint": "光端面清掃・曲げ・断線・モジュールを確認してください。"
  },
  "SFP_RX_DROP": {
    "title": "SFP受信光がベースラインから低下",
    "hint": "光端面清掃・曲げ・断線・モジュールを確認してください。"
  },
  "MSRP_RESV_FAILED": {
    "title": "AVB / Milan帯域予約失敗",
    "hint": "帯域不足・AVB対応・VLAN設定を確認してください。"
  },
  "MVR_RX_FAIL": {
    "title": "MVR IGMP受信失敗",
    "hint": "ソース・レシーバーポートとMVR VLANを確認してください。"
  },
  "CPU_HIGH": {
    "title": "CPU使用率が高い",
    "hint": "スイッチ負荷・冷却・電源を確認してください。"
  },
  "MEM_LOW": {
    "title": "空きメモリ低下",
    "hint": "スイッチ負荷と稼働状態を確認してください。"
  },
  "TEMP_HIGH": {
    "title": "温度異常",
    "hint": "通風・冷却・周囲温度を確認してください。"
  },
  "FAN_FAIL": {
    "title": "ファン異常",
    "hint": "冷却ファンと電源を確認してください。"
  },
  "PSU_FAIL": {
    "title": "電源異常",
    "hint": "電源・ケーブル・電源ユニットを確認してください。"
  },
  "MBUF_ALLOC_FAIL": {
    "title": "Mbuf確保失敗",
    "hint": "輻輳・マイクロバースト・QoSを確認してください。"
  },
  "POE_BUDGET": {
    "title": "PoE予算使用率が高い",
    "hint": "給電容量と機器の消費電力を確認してください。"
  },
  "UDLD": {
    "title": "片方向リンク障害",
    "hint": "光ファイバ両芯とTx/Rxを確認してください。"
  },
  "STP_BLOCKING": {
    "title": "STP Blocking",
    "hint": "物理ループとSTP状態を確認してください。"
  },
  "STP_MSTP": {
    "title": "MSTPが稼働中",
    "hint": "ProAVのRSTP推奨設定を確認してください。"
  },
  "STP_TCN_BURST": {
    "title": "STPトポロジー変更急増",
    "hint": "物理ループとリンク断を確認してください。"
  },
  "DHCP_ROGUE": {
    "title": "不正DHCPサーバーの疑い",
    "hint": "持込ルーター・二重DHCP・固定IP重複を確認してください。"
  },
  "IP_CONFLICT": {
    "title": "IPアドレス衝突",
    "hint": "固定IPとDHCP割当の重複を解消してください。"
  },
  "AUDIT_PENDING": {
    "title": "全スイッチ初回取得待ち",
    "hint": "Slow周期が一巡すると監査結果が表示されます。"
  },
  "FW_MISMATCH": {
    "title": "ファームウェア不一致",
    "hint": "各スイッチのファームウェアを統一してください。"
  },
  "STP_PRIORITY_DEFAULT": {
    "title": "STP優先度が全台既定値",
    "hint": "コア0・予備コア8192の優先度設定を確認してください。"
  },
  "STP_EDGE_ROOT": {
    "title": "エッジがSTP Root Bridge",
    "hint": "コアと予備コアのSTP優先度を確認してください。"
  },
  "DHCP_DOUBLE": {
    "title": "複数DHCPサーバー",
    "hint": "意図しないDHCPサーバーを停止してください。"
  },
  "QUERIER_ON_EDGE": {
    "title": "エッジがIGMPクエリア",
    "hint": "コアへのクエリア配置とIP優先度を確認してください。"
  },
  "SNOOP_MISMATCH": {
    "title": "IGMP Snooping不一致",
    "hint": "VLANごとのSnooping設定を統一してください。"
  },
  "QUERIER_ABSENT": {
    "title": "IGMPクエリア不在",
    "hint": "コアスイッチでIGMPクエリアを有効にしてください。"
  },
  "QUERIER_CONFLICT": {
    "title": "複数クエリアIPを観測",
    "hint": "外部機器のクエリ送信とクエリア選出を確認してください。"
  },
  "IGMP_VERSION_MIX": {
    "title": "IGMPバージョン混在",
    "hint": "IGMPバージョンとAV機器の要件を確認してください。"
  },
  "FLOWCTRL_ON_AV_PORT": {
    "title": "AVポートでFlow Control有効",
    "hint": "NDI4を除きAVポートのFlow Control無効化を検討してください。"
  },
  "NDI4_NEEDS_FLOWCTRL": {
    "title": "NDI4でFlow Control無効",
    "hint": "NDI4ポートのFlow Controlを有効にしてください。"
  },
  "DSCP_TRUST": {
    "title": "AVポートのQoS TrustがDSCP以外",
    "hint": "Dante/AES67/PTP用にTrust DSCPを確認してください。"
  },
  "STORM_CTRL_UPLINK": {
    "title": "アップリンクでStorm Control有効",
    "hint": "高帯域マルチキャストへのドロップ影響を確認してください。"
  },
  "QSYS_MTU": {
    "title": "Q-SYSポートのMTU設定",
    "hint": "Q-SYSのMTU 1500要件を確認してください。"
  },
  "UPLINK_MTU_MISMATCH": {
    "title": "アップリンク両端のMTU不一致",
    "hint": "アップリンク両端のMTUを統一してください。"
  },
  "DAISY_CHAIN": {
    "title": "多段デイジーチェーン",
    "hint": "PTP同期用にはコアからのスター接続を確認してください。"
  },
  "VLAN_CROWDED": {
    "title": "VLANに200台超",
    "hint": "VLAN分割を検討してください。"
  },
  "QUERIER_CHANGED": {
    "title": "IGMPクエリアIP変更",
    "hint": "クエリア選出と外部機器のクエリ送信を確認してください。"
  },
  "MCAST_FLOOD": {
    "title": "マルチキャストフラッディングの疑い",
    "hint": "IGMP Snoopingとクエリアを確認してください。"
  },
  "DEVICE_NEW": {
    "title": "機器を検出",
    "hint": "接続先と機器名を確認してください。"
  },
  "DEVICE_GONE": {
    "title": "機器の観測終了",
    "hint": "接続先・電源・FDBのエージングを確認してください。"
  },
  "PHASE_CHANGED": {
    "title": "運用フェーズ変更: {phase}",
    "hint": "フェーズに応じたPingと抑止設定を確認してください。"
  },
  "WORK_STARTED": {
    "title": "作業中を開始: {group}",
    "hint": "作業中は対象グループのアラートを抑止します。"
  },
  "WORK_ENDED": {
    "title": "作業中を終了: {group}",
    "hint": "対象グループの監視を再開しました。"
  },
  "linkDown": {
    "title": "リンク断",
    "hint": "ケーブル・コネクタ・接続機器の電源を確認してください。"
  },
  "linkUp": {
    "title": "リンク復帰",
    "hint": "接続機器の状態を確認してください。"
  },
  "coldStart": {
    "title": "スイッチ再起動",
    "hint": "電源と再起動原因を確認してください。"
  },
  "warmStart": {
    "title": "スイッチ再起動",
    "hint": "電源と再起動原因を確認してください。"
  }
};
