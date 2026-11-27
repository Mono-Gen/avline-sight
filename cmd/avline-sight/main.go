// AVLine-Sight — NETGEAR AV Line（M4250）専用監視ソフト。
// 単一実行ファイルで起動し、実行ファイルと同階層の ./data/ にDB等を保存する（完全ポータブル）。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/api"
	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/poller"
	"github.com/Mono-Gen/avline-sight/internal/sim"
	"github.com/Mono-Gen/avline-sight/internal/store"
	"github.com/Mono-Gen/avline-sight/internal/trap"
	"github.com/Mono-Gen/avline-sight/web"
)

// version はビルド時に -ldflags "-X main.version=..." で上書きする。
var version = "0.4.0"

func main() {
	// ダブルクリック起動（黒い画面なし）では false。PowerShell等から起動したときはその画面へ出力する
	hasConsole := attachConsole()
	if logPath, err := run(hasConsole); err != nil {
		text := startupErrorText(err, logPath)
		if hasConsole {
			fmt.Fprintln(os.Stderr, text)
		} else {
			// 二重起動は失敗ではなく案内なので、エラーではなく情報のアイコンで出す
			showStartupMessage(text, !errors.Is(err, errAlreadyRunning))
		}
		os.Exit(1)
	}
}

func exeDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

// run は監視を動かし、終了まで戻らない。logPath はエラー表示で案内するログファイルの場所（開けなかったときは空）。
func run(hasConsole bool) (logPath string, err error) {
	var (
		dataDir  = flag.String("data", filepath.Join(exeDir(), "data"), "データ保存先ディレクトリ")
		config   = configFlag(flag.CommandLine)
		listen   = flag.String("listen", "127.0.0.1:8080", "WebUI待受アドレス（LAN公開は 0.0.0.0:8080。閲覧専用ロック併用推奨）")
		trapAddr = flag.String("trap", "", "SNMP Trap待受アドレス（既定 0.0.0.0:162、失敗時は設定画面で変更）")
		lab      = flag.Int("lab", 0, "仮想M4250ラボを N 台起動して監視する（実機なし検証用、1〜20）")
		replay   = flag.String("snmp-dump", "", "実機のSNMPダンプ（snmp_<IP>.txt）のフォルダを読み、その内容で応答する仮想スイッチを監視する（実機なし検証用）")
		labOff   = flag.Int("lab-port-offset", 0, "ラボのSNMP/Telnet/SSHポートをまとめてずらす（ラボを2つ同時に動かす検証用）")
		openUI   = flag.Bool("open", false, "起動後に既定ブラウザでWebUIを開く")
		fast     = flag.Duration("fast", poller.DefaultIntervals.Fast, "Fast周期")
		medium   = flag.Duration("medium", poller.DefaultIntervals.Medium, "Medium周期")
		slow     = flag.Duration("slow", poller.DefaultIntervals.Slow, "Slow周期")
		debug    = flag.Bool("debug", false, "デバッグログ")
	)
	flag.Usage = func() { printUsage(flag.CommandLine) }
	flag.Parse()
	var missingConfigPath bool
	flag.Visit(func(option *flag.Flag) {
		if option.Name == "config" && *config == "" {
			missingConfigPath = true
		}
	})
	if missingConfigPath {
		return "", fmt.Errorf("--config: 設定ファイルのパスを指定してください")
	}
	lvl := slog.LevelInfo
	if *debug {
		lvl = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))
	if *labOff < 0 || *labOff > 10000 {
		return "", fmt.Errorf("--lab-port-offset は 0〜10000 で指定してください")
	}
	api.LabSNMPBase += *labOff
	api.LabCLIBase += *labOff
	api.LabSSHBase += *labOff
	if *lab > 0 && *replay != "" {
		return "", fmt.Errorf("--lab と --snmp-dump は同時に使えません（同じポートを使うため）")
	}
	if *lab < 0 || *lab > 20 {
		return "", fmt.Errorf("--lab は 0〜20 で指定してください")
	}
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		return "", fmt.Errorf("データディレクトリを作成できません %s: %w", *dataDir, err)
	}
	// 同じデータフォルダの二重起動を止める（2つが同じDBへ書くと記録が壊れうる）
	release, err := acquireSingleInstance(*dataDir)
	if err != nil {
		return "", err
	}
	defer release()
	// 黒い画面が無い起動ではログの行き先がファイルだけになるため、常にファイルへも書く
	if lf, err := openRotatingFile(filepath.Join(*dataDir, "avline-sight.log"), logFileLimit); err != nil {
		slog.Warn("ログファイルを開けません。画面への出力だけにします", "err", err)
	} else {
		defer lf.Close()
		logPath = lf.path
		var out io.Writer = lf
		if hasConsole {
			out = io.MultiWriter(lf, os.Stderr)
		}
		slog.SetDefault(slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: lvl})))
	}
	dbName := "avline-sight.db"
	if *lab > 0 {
		dbName = "avline-sight-lab.db" // ラボのデータで本番DBを汚さない
	} else if *replay != "" {
		dbName = "avline-sight-replay.db" // 再生のデータも本番・ラボと分ける
	}
	st, err := store.Open(filepath.Join(*dataDir, dbName), false)
	if err != nil {
		return logPath, err
	}
	defer st.Close()
	configCtx, cancelConfig := context.WithTimeout(context.Background(), 20*time.Second)
	configWarning, err := loadStartupConfig(configCtx, st, exeDir(), *config)
	cancelConfig()
	if err != nil {
		return logPath, err
	}
	if configWarning != "" {
		slog.Warn("設定ファイルを自動で読み込めません", "file", defaultConfigName, "err", configWarning)
	}
	pid, err := st.EnsureDefaultProfile()
	if err != nil {
		return logPath, fmt.Errorf("プロファイル初期化に失敗: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var simLab *sim.Lab
	ta := *trapAddr
	if ta == "" {
		var saved string
		if st.DB.QueryRow(`SELECT v FROM meta WHERE k='trap_addr'`).Scan(&saved) == nil && saved != "" {
			ta = saved
		} else if *lab > 0 || *replay != "" {
			ta = "127.0.0.1:1162" // ラボは管理者権限不要のポートを使う
		} else {
			ta = "0.0.0.0:162"
		}
	}

	hub := api.NewHub()
	eng := engine.New(st, pid, hub)
	if *lab > 0 {
		simLab = sim.NewLab(*lab, time.Now().UnixNano())
		trapTarget := ta
		if h, p, err := net.SplitHostPort(ta); err == nil && (h == "0.0.0.0" || h == "") {
			trapTarget = net.JoinHostPort("127.0.0.1", p)
		}
		if err := simLab.Start(ctx, sim.Config{BaseSNMPPort: api.LabSNMPBase, BaseCLIPort: api.LabCLIBase, BaseSSHPort: api.LabSSHBase, TrapTarget: trapTarget}); err != nil {
			return logPath, err
		}
		if err := registerLab(st, pid, simLab); err != nil {
			return logPath, err
		}
		eng.Reload()
		slog.Info("仮想M4250ラボを起動しました", "台数", *lab, "SNMP", fmt.Sprintf("127.0.0.1:%d〜", api.LabSNMPBase))
	}
	if *replay != "" {
		rp, err := sim.LoadReplay(*replay)
		if err != nil {
			return logPath, err
		}
		if err := rp.Start(ctx, api.LabSNMPBase); err != nil {
			return logPath, err
		}
		if err := registerReplay(st, pid, rp); err != nil {
			return logPath, err
		}
		eng.Reload()
		slog.Info("SNMPダンプの再生を始めました", "台数", len(rp.Agents), "SNMP", fmt.Sprintf("127.0.0.1:%d〜", api.LabSNMPBase))
	}

	iv := poller.Intervals{Fast: *fast, Medium: *medium, Slow: *slow, Timeout: poller.DefaultIntervals.Timeout, Retries: poller.DefaultIntervals.Retries}
	pm := poller.NewManager(iv, eng, eng.Alias).WithPhase(eng.Phase)
	eng.SetFDBRequester(pm.RequestFDB)
	defer pm.StopAll()

	sws, _ := st.Switches(pid)
	tr := trap.New(eng.OnEvent, func(ip string, labPort int) (int64, string, bool) {
		list, _ := st.Switches(eng.ProfileID())
		return resolveTrapSource(list, ip, labPort, simLab != nil)
	})
	tr.OnRegistered(pm.TriggerTrap)
	defer tr.Stop()

	ln, addr, err := listenWithFallback(*listen)
	if err != nil {
		return logPath, err
	}
	// Serve に渡す前に戻る経路（トレイを出せない等）でもポートを開いたままにしない（二重に閉じても害はない）
	defer ln.Close()
	srv := &api.Server{Version: version, DataDir: *dataDir, Store: st, Engine: eng, Poller: pm, Trap: tr, Hub: hub, Lab: simLab,
		Static: web.Dist(), WebAddr: addr, Ctx: ctx, ConfigStartupError: configWarning}
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	pm.Sync(ctx, sws)
	tr.Start(ta)
	engineDone := make(chan struct{})
	go func() { defer close(engineDone); eng.Run(ctx, store.DefaultRetention) }()
	// 定期Pingを含むエンジンの終了を待ってからDBを閉じる。
	defer func() { stop(); <-engineDone }()
	go srv.RunTicker(ctx)
	// Serve は Shutdown を呼んだ直後に戻るため、処理中の要求が終わるのを待ってから DB・ログを閉じる
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		hs.Shutdown(sctx)
	}()
	url := "http://" + addr
	if h, p, _ := net.SplitHostPort(addr); h == "0.0.0.0" || h == "::" {
		url = "http://127.0.0.1:" + p
	}
	slog.Info("AVLine-Sight を起動しました", "version", version, "url", url, "data", *dataDir)
	// トレイの「終了」は Ctrl+C と同じく ctx を止め、サーバー・エンジンを順に閉じる
	closeTray, err := startTray(url, stop)
	if err != nil {
		if !hasConsole {
			// 黒い画面もトレイも無いと止める手段が無くなるため、起動を取りやめる
			stop()
			<-shutdownDone
			return logPath, fmt.Errorf("タスクトレイにアイコンを出せません: %w", err)
		}
		slog.Warn("タスクトレイにアイコンを出せません", "err", err)
		closeTray = func() {}
	}
	defer closeTray()
	if hasConsole {
		fmt.Println("AVLine-Sight:", url, "（Ctrl+Cで終了）")
	}
	// ダブルクリック起動では他に見える物が無いので、画面を必ず開く
	if *openUI || !hasConsole {
		openBrowser(url)
	}
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		stop()
		<-shutdownDone
		return logPath, err
	}
	<-shutdownDone
	return logPath, nil
}

func resolveTrapSource(switches []model.SwitchConfig, ip string, labPort int, lab bool) (int64, string, bool) {
	for _, sw := range switches {
		if sw.IP != ip {
			continue
		}
		// ラボのポート番号はローカルホスト上の複数の仮想スイッチを区別するためだけに使う。
		if lab && ip == "127.0.0.1" && labPort > 0 && sw.SNMPPort != labPort {
			continue
		}
		return sw.ID, sw.Name, true
	}
	return 0, "", false
}

// listenWithFallback は指定ポートが使用中なら +1〜+20 を試す（仕様8.1）。
func listenWithFallback(addr string) (net.Listener, string, error) {
	host, portS, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, "", fmt.Errorf("待受アドレスが不正です %q: %w", addr, err)
	}
	port, _ := strconv.Atoi(portS)
	var last error
	for i := 0; i <= 20; i++ {
		a := net.JoinHostPort(host, strconv.Itoa(port+i))
		ln, err := net.Listen("tcp", a)
		if err == nil {
			if i > 0 {
				slog.Warn("指定ポートが使用中のため別ポートで待受します", "requested", addr, "actual", a)
			}
			return ln, a, nil
		}
		last = err
	}
	return nil, "", fmt.Errorf("WebUIの待受ポートを確保できません: %w", last)
}

func registerLab(st *store.Store, pid int64, lab *sim.Lab) error {
	existing, err := st.Switches(pid)
	if err != nil {
		return err
	}
	have := map[int]bool{}
	for _, c := range existing {
		if c.IP == "127.0.0.1" {
			have[c.SNMPPort] = true
		}
	}
	for _, s := range lab.Switches {
		port := api.LabSNMPBase + s.Index
		if have[port] {
			continue
		}
		tag := "edge"
		if s.Index == 0 {
			tag = "core"
		}
		c := model.SwitchConfig{ProfileID: pid, Name: s.Name, GroupTag: tag, IP: "127.0.0.1", SNMPPort: port, Community: sim.Community, Enabled: true}
		if err := st.SaveSwitch(&c); err != nil {
			return err
		}
	}
	return nil
}

// registerReplay は再生エージェントを監視対象に登録する（同じポートが登録済みなら飛ばす）。
func registerReplay(st *store.Store, pid int64, rp *sim.Replay) error {
	existing, err := st.Switches(pid)
	if err != nil {
		return err
	}
	have := map[int]bool{}
	for _, c := range existing {
		if c.IP == "127.0.0.1" {
			have[c.SNMPPort] = true
		}
	}
	for _, a := range rp.Agents {
		port := api.LabSNMPBase + a.Index
		if have[port] {
			continue
		}
		c := model.SwitchConfig{ProfileID: pid, Name: "Replay-" + a.SourceIP, IP: "127.0.0.1", SNMPPort: port, Community: sim.Community, Enabled: true}
		if err := st.SaveSwitch(&c); err != nil {
			return err
		}
	}
	return nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		slog.Warn("ブラウザを開けませんでした", "err", err)
	}
}
