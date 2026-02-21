// nile is the runtime for Nile copts. It manages the WAL, spawns the neb
// process, delivers messages over stdio JSON-RPC, and handles retention.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/gluck/nile/pkg/lifecycle"
	nileotel "github.com/gluck/nile/pkg/otel"
	"github.com/gluck/nile/pkg/sandbox"
	"github.com/gluck/nile/pkg/transport"
	"github.com/gluck/nile/pkg/wal"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		cmdRun(os.Args[2:])
	case "install":
		cmdInstall(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: nile <command> [options]

Commands:
  run <name>       Run a copt (spawn neb, process messages)
  install <name>   Generate and enable a systemd user unit
  status <name>    Show copt status

Run options:
  --binary <path>           Path to neb binary
  --data-dir <path>         Data directory (default: /var/lib/nile/<name>)
  --max-messages <n>        Retention: max consumed messages (default: 10000)
  --max-bytes <n>           Retention: max log bytes (default: 10485760)
  --segment-size <n>        Bytes per WAL segment (default: 1048576)
  --message-timeout <secs>  Neb response timeout in seconds (default: 60)
  --max-retries <n>         Retries before dead-letter (default: 3)
  --max-depth <n>           Max unprocessed messages (Phase 2: HTTP 429)
`)
}

type runConfig struct {
	name           string
	binary         string
	dataDir        string
	maxMessages    int
	maxBytes       int64
	segmentSize    int64
	messageTimeout int
	maxRetries     int
	maxDepth       int
}

func parseRunArgs(args []string) runConfig {
	cfg := runConfig{
		maxMessages:    10000,
		maxBytes:       10 * 1024 * 1024,
		segmentSize:    1024 * 1024,
		messageTimeout: 60,
		maxRetries:     3,
	}

	requireValue := func(flag string, i int) {
		if i >= len(args) {
			fmt.Fprintf(os.Stderr, "error: %s requires a value\n", flag)
			os.Exit(1)
		}
	}
	parseInt := func(flag, val string) int {
		n, err := strconv.Atoi(val)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: invalid integer %q\n", flag, val)
			os.Exit(1)
		}
		return n
	}
	parseInt64 := func(flag, val string) int64 {
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: invalid integer %q\n", flag, val)
			os.Exit(1)
		}
		return n
	}

	i := 0
	for i < len(args) {
		flag := args[i]
		switch flag {
		case "--binary":
			i++
			requireValue(flag, i)
			cfg.binary = args[i]
		case "--data-dir":
			i++
			requireValue(flag, i)
			cfg.dataDir = args[i]
		case "--max-messages":
			i++
			requireValue(flag, i)
			cfg.maxMessages = parseInt(flag, args[i])
		case "--max-bytes":
			i++
			requireValue(flag, i)
			cfg.maxBytes = parseInt64(flag, args[i])
		case "--segment-size":
			i++
			requireValue(flag, i)
			cfg.segmentSize = parseInt64(flag, args[i])
		case "--message-timeout":
			i++
			requireValue(flag, i)
			cfg.messageTimeout = parseInt(flag, args[i])
		case "--max-retries":
			i++
			requireValue(flag, i)
			cfg.maxRetries = parseInt(flag, args[i])
		case "--max-depth":
			i++
			requireValue(flag, i)
			cfg.maxDepth = parseInt(flag, args[i])
		default:
			if cfg.name == "" && !strings.HasPrefix(args[i], "-") {
				cfg.name = args[i]
			} else {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
				os.Exit(1)
			}
		}
		i++
	}

	if cfg.name == "" {
		fmt.Fprintln(os.Stderr, "error: copt name required")
		os.Exit(1)
	}
	if cfg.binary == "" {
		fmt.Fprintln(os.Stderr, "error: --binary required")
		os.Exit(1)
	}
	if cfg.dataDir == "" {
		cfg.dataDir = filepath.Join("/var/lib/nile", cfg.name)
	}
	return cfg
}

func cmdRun(args []string) {
	cfg := parseRunArgs(args)

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil)).With("copt", cfg.name)

	// Resolve binary to absolute path
	binary, err := filepath.Abs(cfg.binary)
	if err != nil {
		logger.Error("resolve binary path", "error", err)
		os.Exit(1)
	}

	// Ensure data directories exist
	stateDir := filepath.Join(cfg.dataDir, "state")
	retainDir := filepath.Join(cfg.dataDir, "retain")
	deadDir := filepath.Join(cfg.dataDir, "dead")
	runDir := filepath.Join(cfg.dataDir, "run")
	for _, d := range []string{stateDir, retainDir, deadDir, runDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			logger.Error("create directory", "path", d, "error", err)
			os.Exit(1)
		}
	}

	// Write PID file
	pidFile := filepath.Join(runDir, "nile.pid")
	os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove(pidFile)

	// Setup OTel
	otelProviders, err := nileotel.Setup(nileotel.Config{
		CoptName: cfg.name,
		DataDir:  cfg.dataDir,
	})
	if err != nil {
		logger.Error("setup otel", "error", err)
		os.Exit(1)
	}
	defer otelProviders.Shutdown(context.Background())

	metrics := nileotel.NewMetrics()

	// Open WAL
	wlog, err := wal.Open(cfg.dataDir, wal.Options{
		MaxMessages: cfg.maxMessages,
		MaxBytes:    cfg.maxBytes,
		SegmentSize: cfg.segmentSize,
		MaxDepth:    cfg.maxDepth,
	})
	if err != nil {
		logger.Error("open WAL", "error", err)
		os.Exit(1)
	}
	defer wlog.Close()

	// Spawn neb process
	nebCmd, err := sandbox.Command(sandbox.Config{
		NebBinary: binary,
		StateDir:  stateDir,
		RetainDir: retainDir,
	})
	if err != nil {
		logger.Error("create neb command", "error", err)
		os.Exit(1)
	}

	nebStdin, err := nebCmd.StdinPipe()
	if err != nil {
		logger.Error("stdin pipe", "error", err)
		os.Exit(1)
	}
	nebStdout, err := nebCmd.StdoutPipe()
	if err != nil {
		logger.Error("stdout pipe", "error", err)
		os.Exit(1)
	}
	nebCmd.Stderr = os.Stderr

	if err := nebCmd.Start(); err != nil {
		logger.Error("start neb", "error", err)
		os.Exit(1)
	}
	logger.Info("started neb process", "pid", nebCmd.Process.Pid)

	// Create transport
	tr := transport.NewStdio(nebStdin, nebStdout)

	// Create lifecycle manager
	mgr := lifecycle.New(lifecycle.Config{
		Name:           cfg.name,
		DataDir:        cfg.dataDir,
		Store:          wlog,
		Transport:      tr,
		Logger:         logger,
		Metrics:        metrics,
		MessageTimeout: time.Duration(cfg.messageTimeout) * time.Second,
		MaxRetries:     cfg.maxRetries,
	})

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig.String())
		mgr.Stop()
	}()

	// Run the lifecycle (blocks until stopped)
	if err := mgr.Start(); err != nil {
		logger.Error("lifecycle error", "error", err)
	}

	// Cleanup
	tr.Close()
	nebStdin.Close()
	if err := nebCmd.Wait(); err != nil {
		logger.Info("neb exited", "error", err)
	}

	logger.Info("copt stopped")
}

const systemdUnitTemplate = `[Unit]
Description=Nile copt: {{.Name}}
After=network.target

[Service]
Type=simple
ExecStart={{.NileBinary}} run {{.Name}} --binary {{.Binary}} --data-dir {{.DataDir}}{{if ne .MaxMessages 0}} --max-messages {{.MaxMessages}}{{end}}{{if ne .MaxBytes 0}} --max-bytes {{.MaxBytes}}{{end}}{{if ne .MaxRetries 0}} --max-retries {{.MaxRetries}}{{end}}{{if ne .MessageTimeout 0}} --message-timeout {{.MessageTimeout}}{{end}}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`

type unitData struct {
	Name           string
	NileBinary     string
	Binary         string
	DataDir        string
	MaxMessages    int
	MaxBytes       int64
	MaxRetries     int
	MessageTimeout int
}

func cmdInstall(args []string) {
	cfg := parseRunArgs(args)

	binary, err := filepath.Abs(cfg.binary)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: resolve binary path: %v\n", err)
		os.Exit(1)
	}

	nileBinary, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: resolve nile binary: %v\n", err)
		os.Exit(1)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: get home dir: %v\n", err)
		os.Exit(1)
	}
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: create unit dir: %v\n", err)
		os.Exit(1)
	}

	unitName := fmt.Sprintf("nile-%s.service", cfg.name)
	unitPath := filepath.Join(unitDir, unitName)

	tmpl, err := template.New("unit").Parse(systemdUnitTemplate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: parse template: %v\n", err)
		os.Exit(1)
	}

	f, err := os.Create(unitPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: create unit file: %v\n", err)
		os.Exit(1)
	}

	data := unitData{
		Name:           cfg.name,
		NileBinary:     nileBinary,
		Binary:         binary,
		DataDir:        cfg.dataDir,
		MaxMessages:    cfg.maxMessages,
		MaxBytes:       cfg.maxBytes,
		MaxRetries:     cfg.maxRetries,
		MessageTimeout: cfg.messageTimeout,
	}

	if err := tmpl.Execute(f, data); err != nil {
		f.Close()
		fmt.Fprintf(os.Stderr, "error: write unit file: %v\n", err)
		os.Exit(1)
	}
	f.Close()

	fmt.Printf("Created: %s\n", unitPath)
	fmt.Println()
	fmt.Println("To enable and start:")
	fmt.Printf("  systemctl --user daemon-reload\n")
	fmt.Printf("  systemctl --user enable --now %s\n", unitName)
}

func cmdStatus(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: copt name required")
		os.Exit(1)
	}
	name := args[0]
	dataDir := filepath.Join("/var/lib/nile", name)

	pidFile := filepath.Join(dataDir, "run", "nile.pid")
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Printf("copt %s: not running (no pid file)\n", name)
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		fmt.Printf("copt %s: not running (invalid pid file)\n", name)
		return
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf("copt %s: not running\n", name)
		return
	}

	err = process.Signal(syscall.Signal(0))
	if err != nil {
		fmt.Printf("copt %s: not running (pid %d stale)\n", name, pid)
		return
	}

	fmt.Printf("copt %s: running (pid %d)\n", name, pid)

	wlog, err := wal.Open(dataDir, wal.DefaultOptions())
	if err == nil {
		fmt.Printf("  next index: %d\n", wlog.NextIndex())
		fmt.Printf("  depth: %d\n", wlog.Depth())
		fmt.Printf("  total bytes: %d\n", wlog.TotalBytes())
		wlog.Close()
	}

	// Show dead letter count
	deadLetters, _ := wlog.ReadDeadLetters()
	if len(deadLetters) > 0 {
		fmt.Printf("  dead letters: %d\n", len(deadLetters))
	}
}
