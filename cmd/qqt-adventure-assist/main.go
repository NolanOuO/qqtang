// qqt-adventure-assist controls the local-server adventure instant-clear
// assistant through the GM HTTP API. It is a local test/GM aid for your own
// QQTang-Local server: enabling it makes every adventure map entry
// automatically defeat all monsters and the BOSS shortly after the stage
// loads. Do not use it against servers you do not operate.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type assistStatus struct {
	Enabled             bool `json:"enabled"`
	DelayMS             int  `json:"delay_ms"`
	DisableVisualDeaths bool `json:"disable_visual_deaths"`
	Pending             int  `json:"pending"`
}

func main() {
	gmAddress := flag.String("gm", "127.0.0.1:18100", "GM HTTP address (host:port)")
	action := flag.String("action", "status", "status, on, off, or trigger")
	username := flag.String("username", "", "GM username (only needed for remote GM)")
	password := flag.String("password", "", "GM password (prefer --password-stdin)")
	passwordStdin := flag.Bool("password-stdin", false, "read the GM password from stdin")
	timeout := flag.Duration("timeout", 10*time.Second, "HTTP request timeout")
	flag.Parse()

	switch *action {
	case "status", "on", "off", "trigger":
	default:
		fatal(fmt.Sprintf("unknown action %q: use status, on, off, or trigger", *action))
	}

	secret := *password
	if *passwordStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatal(fmt.Sprintf("read password from stdin: %v", err))
		}
		secret = strings.TrimRight(string(data), "\r\n")
	}

	client := &http.Client{Timeout: *timeout}
	base := "http://" + strings.TrimPrefix(strings.TrimPrefix(*gmAddress, "http://"), "https://")

	switch *action {
	case "status":
		var status assistStatus
		call(client, base, "GET", "/gm/api/adventure-assist", nil, *username, secret, &status)
		fmt.Printf("enabled=%t delay_ms=%d disable_visual_deaths=%t pending=%d\n",
			status.Enabled, status.DelayMS, status.DisableVisualDeaths, status.Pending)
	case "on", "off":
		var status assistStatus
		call(client, base, "POST", "/gm/api/adventure-assist",
			map[string]bool{"enabled": *action == "on"}, *username, secret, &status)
		fmt.Printf("enabled=%t delay_ms=%d pending=%d\n", status.Enabled, status.DelayMS, status.Pending)
	case "trigger":
		var result struct {
			Triggered int `json:"triggered"`
		}
		call(client, base, "POST", "/gm/api/adventure-assist/trigger", map[string]bool{}, *username, secret, &result)
		fmt.Printf("triggered=%d\n", result.Triggered)
	}
}

func call(client *http.Client, base, method, path string, body any, username, password string, out any) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			fatal(fmt.Sprintf("encode request: %v", err))
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		fatal(fmt.Sprintf("build request: %v", err))
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if username != "" {
		request.SetBasicAuth(username, password)
	}
	response, err := client.Do(request)
	if err != nil {
		fatal(fmt.Sprintf("GM request failed: %v", err))
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		fatal(fmt.Sprintf("read GM response: %v", err))
	}
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(payload, &failure)
		if failure.Error == "" {
			failure.Error = strings.TrimSpace(string(payload))
		}
		fatal(fmt.Sprintf("GM %s %s: HTTP %d: %s", method, path, response.StatusCode, failure.Error))
	}
	if out != nil {
		if err := json.Unmarshal(payload, out); err != nil {
			fatal(fmt.Sprintf("decode GM response: %v", err))
		}
	}
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, "qqt-adventure-assist: "+message)
	os.Exit(1)
}
