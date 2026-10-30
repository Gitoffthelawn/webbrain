package core

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	sdkconfig "github.com/router-for-me/CLIProxyAPI/v8/sdk/config"

	"github.com/esokullu/webbrain-systray/internal/paths"
)

// Bonsai 2's chat template accepts these efforts while thinking is on and
// raises on any other.
var bonsaiTemplateEfforts = []string{"low", "medium", "xhigh"}

func bonsaiOptions(baseURL string, apiKeys ...string) Options {
	return Options{Host: "127.0.0.1", APIKeys: apiKeys, Local: []LocalUpstream{{
		Name:    "bonsai",
		BaseURL: baseURL,
		Models:  []LocalModel{{Upstream: "bonsai-2-27b", Alias: "bonsai-2-27b", Vision: true, ContextLength: 65536}},
	}}}
}

// The proxy sends a local model only the levels declared for it, and passes
// WebBrain's reasoning_effort "none" through as "none" only when the model
// allows zero thinking (otherwise it sends "low", which Bonsai 2 runs close
// to xhigh).
func TestWriteConfigDeclaresBonsaiThinkingControls(t *testing.T) {
	t.Setenv("WBC_HOME", t.TempDir())
	if err := writeConfig(8327, bonsaiOptions("http://127.0.0.1:18081/v1", "internal"), "secret", false); err != nil {
		t.Fatal(err)
	}
	cfg, err := sdkconfig.LoadConfig(paths.ProxyConfig())
	if err != nil {
		t.Fatalf("generated proxy config does not load: %v", err)
	}
	if len(cfg.OpenAICompatibility) != 1 || len(cfg.OpenAICompatibility[0].Models) != 1 {
		t.Fatalf("local upstream missing from generated config: %+v", cfg.OpenAICompatibility)
	}
	thinking := cfg.OpenAICompatibility[0].Models[0].Thinking
	if thinking == nil || !thinking.ZeroAllowed {
		t.Fatalf("local model cannot turn thinking off: %+v", thinking)
	}
	if got, want := strings.Join(thinking.Levels, ","), strings.Join(bonsaiTemplateEfforts, ","); got != want {
		t.Fatalf("declared levels %s, want the efforts Bonsai 2's template accepts: %s", got, want)
	}
}

// Whatever effort a client asks for, the local server receives "none", no
// effort, or one Bonsai 2's template accepts, and the request succeeds.
func TestProxySendsBonsaiOnlyEffortsItsTemplateAccepts(t *testing.T) {
	if testing.Short() {
		t.Skip("starts the embedded proxy")
	}
	t.Setenv("WBC_HOME", t.TempDir())
	var mu sync.Mutex
	var received []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		effort, ok := body["reasoning_effort"].(string)
		if !ok {
			effort = "(none sent)"
		}
		mu.Lock()
		received = append(received, effort)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"bonsai-2-27b",`+
			`"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],`+
			`"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer upstream.Close()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	opts := bonsaiOptions(upstream.URL+"/v1", "test-key")
	opts.PreferredPort = port
	p := New()
	if err := p.Start(opts); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	client := &http.Client{Timeout: 10 * time.Second}
	post := func(path, body string) (int, string) {
		req, _ := http.NewRequest(http.MethodPost, p.BaseURL()+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(out)
	}
	// The proxy registers configured models as it starts.
	deadline := time.Now().Add(15 * time.Second)
	for {
		req, _ := http.NewRequest(http.MethodGet, p.BaseURL()+"/v1/models", nil)
		req.Header.Set("Authorization", "Bearer test-key")
		resp, err := client.Do(req)
		if err == nil {
			list, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if strings.Contains(string(list), `"bonsai-2-27b"`) {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("bonsai-2-27b never appeared in /v1/models")
		}
		time.Sleep(100 * time.Millisecond)
	}

	const chat = `{"model":"bonsai-2-27b","messages":[{"role":"user","content":"Plan."}]`
	for _, tc := range []struct {
		name, path, body, want string // want "" accepts any template effort
	}{
		{"none", "/v1/chat/completions", chat + `,"reasoning_effort":"none"}`, "none"},
		{"no effort", "/v1/chat/completions", chat + `}`, "(none sent)"},
		{"low", "/v1/chat/completions", chat + `,"reasoning_effort":"low"}`, "low"},
		{"medium", "/v1/chat/completions", chat + `,"reasoning_effort":"medium"}`, "medium"},
		{"xhigh", "/v1/chat/completions", chat + `,"reasoning_effort":"xhigh"}`, "xhigh"},
		{"minimal", "/v1/chat/completions", chat + `,"reasoning_effort":"minimal"}`, ""},
		{"high", "/v1/chat/completions", chat + `,"reasoning_effort":"high"}`, ""},
		{"max", "/v1/chat/completions", chat + `,"reasoning_effort":"max"}`, ""},
		{"claude budget", "/v1/messages", `{"model":"bonsai-2-27b","max_tokens":64,"messages":[{"role":"user","content":"Plan."}],` +
			`"thinking":{"type":"enabled","budget_tokens":24000}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mu.Lock()
			received = nil
			mu.Unlock()
			status, out := post(tc.path, tc.body)
			if status != http.StatusOK {
				t.Fatalf("status %d: %s", status, out)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(received) != 1 {
				t.Fatalf("local server got %d requests, want 1", len(received))
			}
			got := received[0]
			if tc.want != "" {
				if got != tc.want {
					t.Fatalf("local server got reasoning_effort %q, want %q", got, tc.want)
				}
				return
			}
			for _, effort := range bonsaiTemplateEfforts {
				if got == effort {
					return
				}
			}
			t.Fatalf("local server got reasoning_effort %q, which Bonsai 2's template rejects", got)
		})
	}
}

func TestHealthProbeRequiresExpectedResponse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"error response", http.StatusNotFound, "wrong", false},
		{"another service", http.StatusOK, "wrong", false},
		{"expected service", http.StatusOK, "my-instance", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			if got := healthReady(&http.Client{Timeout: time.Second}, server.URL, "my-instance"); got != tc.want {
				t.Fatalf("healthReady = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPickPortMaximum(t *testing.T) {
	const host = "127.0.0.1"
	const port = 65535
	if !portFree(host, port) {
		t.Skip("port 65535 is unavailable")
	}
	got, err := PickPort(host, port, false)
	if err != nil {
		t.Fatalf("PickPort(%q, %d, false): %v", host, port, err)
	}
	if got != port {
		t.Fatalf("PickPort returned %d, want %d", got, port)
	}
}

func TestTimedOutStopKeepsProxyStateUntilExit(t *testing.T) {
	done := make(chan struct{})
	p := &Proxy{port: 8327, done: done, cancel: func() {}}
	p.stop(20 * time.Millisecond)
	if !p.Running() || p.Port() != 8327 {
		t.Fatalf("timed-out stop lost live proxy state: running=%v port=%d", p.Running(), p.Port())
	}
	close(done)
	if p.Running() || p.Port() != 0 {
		t.Fatalf("exited proxy still appears live: running=%v port=%d", p.Running(), p.Port())
	}
}

func TestRestartWaitsForPreviousProxyToExit(t *testing.T) {
	done := make(chan struct{})
	p := &Proxy{port: 8327, done: done, cancel: func() {}}
	stopped := make(chan struct{})
	go func() {
		p.StopForRestart()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("restart proceeded while old proxy was still alive")
	case <-time.After(25 * time.Millisecond):
	}
	close(done)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("restart did not proceed after old proxy exited")
	}
	if p.Port() != 0 {
		t.Fatal("stopped proxy retained its port")
	}
}
