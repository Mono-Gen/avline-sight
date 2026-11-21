// Package sim は実機なし検証のための仮想M4250（SNMPv2cエージェント＋Trap送信＋障害注入＋モックCLI）。
package sim

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

// DeviceKind は仮想ポートに接続された機器の種類。
type DeviceKind string

const (
	DevNone     DeviceKind = ""
	DevDante    DeviceKind = "dante"
	DevNDI      DeviceKind = "ndi"
	DevCamera   DeviceKind = "ptz-camera"
	DevSDVoE    DeviceKind = "sdvoe"
	DevLighting DeviceKind = "lighting"
	DevControl  DeviceKind = "control"
	DevUplink   DeviceKind = "uplink"
	DevHub      DeviceKind = "hub"
)

// Device は接続機器の模擬情報。
type Device struct {
	Kind    DeviceKind
	Name    string
	MAC     [6]byte
	IP      [4]byte
	VLAN    int
	LLDP    bool
	PoEmW   float64 // 0なら非PoE
	RxBps   float64 // スイッチ受信方向（機器→スイッチ）
	TxBps   float64
	McastTx float64 // 機器から送出するマルチキャストpps
	Groups  []string
}

// Port は仮想ポートの状態。カウンタはfloat64で積算し、読み出し時にuint64化する。
type Port struct {
	IfIndex    int
	Name       string
	SFP        bool
	Up         bool
	SpeedMbps  int
	Dev        *Device
	PeerSwitch int // アップリンク先スイッチ番号（-1:なし）
	PeerPort   int

	InOct, OutOct, InMc, OutMc, InBc, OutBc float64
	InErr, OutErr, InDisc, OutDisc          float64
	FCS, Align, Symbol, TooLong, LateCol    float64
	PauseIn, PauseOut, LPITx, LPIRx         float64
	CosDrop                                 [8]float64
	PoeDenied, PoeOverload, PoeShort        float64
	PoeFault                                int
	RxDBm, TxDBm                            float64
	UdldOper                                int
	EEE                                     bool
	PauseMode                               int
	MaxFrame                                int
	Blocking                                bool

	// 障害注入で設定するレート（/秒）
	FCSRate, TooLongRate, PauseRate, LPIRate, DiscRate float64
	McastFloodPps                                    float64
}

// Switch は1台の仮想M4250。
type Switch struct {
	mu sync.Mutex

	Index    int
	Name     string
	Model    string
	FW       string
	Serial   string
	BaseMAC  [6]byte
	MgmtIP   [4]byte
	Ports    []*Port
	VLANs    map[int]string
	bootTime time.Time
	last     time.Time
	rng      *rand.Rand

	CPU5s          float64
	MemFreePct     float64
	MbufTotal      int
	MbufUsed       float64
	MbufAllocFails float64
	TempC          float64
	FanState       int
	PsuState       int
	PoeBudgetW     float64

	Querier       map[int]*QuerierState // VLAN別
	DhcpServerOn  bool
	DhcpInvalid   float64
	DhcpConflicts []string
	StpVersion    int
	StpPriority   int
	TopoChanges   float64
	Logs          []LogEntry
	logSeq        int
	EEEGlobal     bool
	Cable         CableResult

	trapHook func(name string, vbs []extraVB)
}

// QuerierState はVLANのIGMPクエリア状態。
type QuerierState struct {
	Snooping   bool
	AdminOn    bool
	Oper       int    // 0=disabled 1=querier 2=non-querier
	Address    string // 自身のクエリアIP
	LastAddr   string
	LastVer    int
	OperVer    int
}

// LogEntry はスイッチ内部メモリログ1行。
type LogEntry struct {
	Index int
	Text  string
}

// CableResult はTDRケーブル診断結果。
type CableResult struct {
	IfIndex, Status, MinLen, MaxLen, FailLoc int
}

// ModelSpec はモデル毎のポート構成。
type ModelSpec struct {
	Name   string
	Copper int
	SFP    int
	PoEW   float64
}

// Models はラボで使う代表的なM4250モデル（ポート数は製品カタログ準拠の概数）。
var Models = []ModelSpec{
	{"M4250-26G4F-PoE++", 26, 4, 1440},
	{"M4250-10G2XF-PoE++", 10, 2, 720},
	{"M4250-12M2XF", 12, 2, 0},
	{"M4250-40G8XF-PoE++", 40, 8, 2880},
}

func (s *Switch) uptimeTicks() uint32 {
	return uint32(time.Since(s.bootTime).Milliseconds() / 10)
}

// advance は前回からの経過時間ぶん各カウンタを進める（呼び出し側でロック済み）。
func (s *Switch) advance(now time.Time) {
	dt := now.Sub(s.last).Seconds()
	if dt <= 0 {
		return
	}
	if dt > 60 {
		dt = 60
	}
	s.last = now
	t := float64(now.UnixMilli()) / 1000
	for _, p := range s.Ports {
		if !p.Up {
			continue
		}
		var rx, tx, mcIn, mcOut float64
		if p.Dev != nil {
			j := 1 + 0.08*math.Sin(t/7+float64(p.IfIndex)) + 0.04*(s.rng.Float64()-0.5)
			rx, tx = p.Dev.RxBps*j, p.Dev.TxBps*j
			mcIn = p.Dev.McastTx * j
			mcOut = float64(len(p.Dev.Groups)) * 900 * j
		}
		mcOut += p.McastFloodPps
		capBps := float64(p.SpeedMbps) * 1e6
		if tx > capBps*0.98 {
			p.OutDisc += (tx - capBps*0.98) / 8 / 1200 * dt
			tx = capBps * 0.98
		}
		p.InOct += rx / 8 * dt
		p.OutOct += (tx + p.McastFloodPps*1200*8) / 8 * dt
		p.InMc += mcIn * dt
		p.OutMc += mcOut * dt
		p.InBc += 3 * dt
		p.OutBc += 8 * dt
		p.FCS += p.FCSRate * dt
		p.InErr += p.FCSRate * dt
		p.Symbol += p.FCSRate * 0.3 * dt
		p.TooLong += p.TooLongRate * dt
		p.PauseIn += p.PauseRate * dt
		p.PauseOut += p.PauseRate * 0.5 * dt
		p.InDisc += p.DiscRate * dt
		p.CosDrop[5] += p.DiscRate * 0.5 * dt
		if p.EEE {
			p.LPITx += p.LPIRate * dt
			p.LPIRx += p.LPIRate * dt
		}
	}
	// 筐体ヘルスはゆらぎ付きで目標値へ漸近させる
	s.MbufUsed += (float64(s.MbufTotal)*0.18 - s.MbufUsed) * 0.1
	s.TopoChanges += 0
}

// poeTotalW は現在のPoE総消費電力[W]。
func (s *Switch) poeTotalW() float64 {
	var w float64
	for _, p := range s.Ports {
		if p.Up && p.Dev != nil && p.Dev.PoEmW > 0 && p.PoeFault == 0 {
			w += p.Dev.PoEmW / 1000
		}
	}
	return w
}

func (s *Switch) addLog(format string, a ...any) {
	s.logSeq++
	ts := time.Now().UTC().Format("Jan 02 15:04:05")
	s.Logs = append(s.Logs, LogEntry{s.logSeq, fmt.Sprintf("<%d> %s %s %s", 13, ts, s.Name, fmt.Sprintf(format, a...))})
	if len(s.Logs) > 200 { // 実機同様に上限付きリングバッファ
		s.Logs = s.Logs[len(s.Logs)-200:]
	}
}
