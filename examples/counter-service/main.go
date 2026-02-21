// counter-service is a reference Nile neb.
//
// It appends every event to $NILE_STATE_DIR/activity.log so you can
// observe the system with: tail -f /var/lib/nile/<name>/state/activity.log
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      uint64          `json:"id"`
}

type messageParams struct {
	Offset uint64 `json:"offset"`
	Data   string `json:"data"`
}

type retainParams struct {
	Snapshot string `json:"snapshot"`
}

type response struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
	ID      uint64      `json:"id"`
}

type statusResult struct {
	Status string `json:"status"`
}

var logFile *os.File

func main() {
	stateDir := os.Getenv("NILE_STATE_DIR")
	if stateDir == "" {
		stateDir = "."
	}

	var err error
	logFile, err = os.OpenFile(
		filepath.Join(stateDir, "activity.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "counter: open log: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			logf("ERROR parse: %v", err)
			continue
		}

		switch req.Method {
		case "init":
			logf("INIT")
			respond(req.ID, statusResult{Status: "ok"})

		case "message":
			var p messageParams
			json.Unmarshal(req.Params, &p)
			logf("MSG offset=%d data=%s", p.Offset, p.Data)
			respond(req.ID, statusResult{Status: "ok"})

		case "retain":
			var p retainParams
			json.Unmarshal(req.Params, &p)
			logf("RETAIN snapshot=%s", p.Snapshot)
			respond(req.ID, statusResult{Status: "ok"})

		case "shutdown":
			logf("SHUTDOWN")
			respond(req.ID, statusResult{Status: "ok"})
			os.Exit(0)

		default:
			logf("UNKNOWN method=%s", req.Method)
			data, _ := json.Marshal(response{
				JSONRPC: "2.0",
				Error:   map[string]interface{}{"code": -32601, "message": "unknown method"},
				ID:      req.ID,
			})
			fmt.Println(string(data))
		}
	}
}

func respond(id uint64, result interface{}) {
	data, _ := json.Marshal(response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	})
	fmt.Println(string(data))
}

func logf(format string, args ...interface{}) {
	line := fmt.Sprintf("%s %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	logFile.WriteString(line)
}
