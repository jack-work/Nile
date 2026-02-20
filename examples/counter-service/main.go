// counter-service is a Nile neb that counts messages and persists the count.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      uint64          `json:"id"`
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

func main() {
	stateDir := os.Getenv("NILE_STATE_DIR")
	if stateDir == "" {
		stateDir = "."
	}
	countFile := filepath.Join(stateDir, "count")

	// Load existing count
	count := loadCount(countFile)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			fmt.Fprintf(os.Stderr, "counter: parse error: %v\n", err)
			continue
		}

		switch req.Method {
		case "init":
			fmt.Fprintf(os.Stderr, "counter: initialized, current count: %d\n", count)
			respond(req.ID, statusResult{Status: "ok"})

		case "message":
			count++
			saveCount(countFile, count)
			fmt.Fprintf(os.Stderr, "counter: count = %d\n", count)
			respond(req.ID, statusResult{Status: "ok"})

		case "retain":
			fmt.Fprintf(os.Stderr, "counter: retain, count = %d\n", count)
			respond(req.ID, statusResult{Status: "ok"})

		case "shutdown":
			fmt.Fprintf(os.Stderr, "counter: shutting down, final count = %d\n", count)
			respond(req.ID, statusResult{Status: "ok"})
			os.Exit(0)

		default:
			resp := response{
				JSONRPC: "2.0",
				Error:   map[string]interface{}{"code": -32601, "message": "unknown method"},
				ID:      req.ID,
			}
			data, _ := json.Marshal(resp)
			fmt.Println(string(data))
		}
	}
}

func respond(id uint64, result interface{}) {
	resp := response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func loadCount(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return n
}

func saveCount(path string, count int) {
	os.WriteFile(path, []byte(strconv.Itoa(count)), 0644)
}
