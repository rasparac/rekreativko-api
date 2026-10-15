// Command open-browsers opens one Chrome window per user of a seed YAML file
// (see scripts/seed-users), already logged in to the web app, tiled on the
// screen - for watching live updates from several users at once.
//
// Each user gets their own Chrome profile (--user-data-dir): incognito windows
// share one session, so they can't hold different users. The profiles are
// kept, so re-running reuses the open windows and just logs them in again
// (e.g. after the database was reset).
//
// Login is the real one: the user logs in through the gateway, and the tokens
// are put into the web app's localStorage over the DevTools protocol, where
// the app looks for a session on load.
//
//	go run ./scripts/open-browsers -file scripts/seed-users/users.example.yaml
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/websocket"
	"gopkg.in/yaml.v3"
)

// The keys the web app keeps its session under
// (mobile-app/shared/session/tokenStorage.ts).
const (
	accessTokenKey  = "rekreativko_access_token"
	refreshTokenKey = "rekreativko_refresh_token"
)

type options struct {
	file        string
	webURL      string
	gatewayURL  string
	chrome      string
	profilesDir string
	basePort    int
	columns     int
	width       int
	height      int
	only        string
}

func main() {
	home, _ := os.UserHomeDir()

	opts := options{}
	flag.StringVar(&opts.file, "file", "scripts/seed-users/users.example.yaml", "seed YAML file with the users")
	flag.StringVar(&opts.webURL, "web", "http://localhost:8888", "web app URL")
	flag.StringVar(&opts.gatewayURL, "gateway", "http://localhost:8080", "gateway base URL")
	flag.StringVar(&opts.chrome, "chrome", "google-chrome", "Chrome binary")
	flag.StringVar(&opts.profilesDir, "profiles", filepath.Join(home, ".cache", "rekreativko-browsers"), "directory for the per-user Chrome profiles")
	flag.IntVar(&opts.basePort, "port", 9300, "DevTools port of the first window; the next ones count up")
	flag.IntVar(&opts.columns, "columns", 3, "windows per row")
	flag.IntVar(&opts.width, "width", 1146, "window width")
	flag.IntVar(&opts.height, "height", 1200, "window height")
	flag.StringVar(&opts.only, "only", "", "comma-separated emails to open, instead of every user in the file")
	flag.Parse()

	if err := run(context.Background(), opts); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

type user struct {
	Email    string `yaml:"email"`
	Password string `yaml:"password"`

	// index is the user's place in the file; it picks the window's DevTools
	// port and tile, so a user keeps the same window with or without -only.
	index int
}

func run(ctx context.Context, opts options) error {
	users, err := loadUsers(opts.file, opts.only)
	if err != nil {
		return err
	}

	web, err := url.Parse(opts.webURL)
	if err != nil {
		return fmt.Errorf("parse -web: %w", err)
	}
	origin := web.Scheme + "://" + web.Host

	failed := 0
	for _, u := range users {
		w := window{
			user:    u,
			port:    opts.basePort + u.index,
			profile: filepath.Join(opts.profilesDir, u.Email),
			x:       (u.index % opts.columns) * opts.width,
			y:       (u.index / opts.columns) * opts.height,
		}

		if err := w.open(ctx, opts, origin); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "x %s: %v\n", u.Email, err)
			continue
		}
		fmt.Printf("+ %s (DevTools port %d)\n", u.Email, w.port)
	}

	if failed > 0 {
		return fmt.Errorf("%d of %d windows failed", failed, len(users))
	}
	return nil
}

// loadUsers reads just the emails and passwords from a seed file.
func loadUsers(path, only string) ([]user, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var file struct {
		Defaults struct {
			Password string `yaml:"password"`
		} `yaml:"defaults"`
		Users []user `yaml:"users"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	wanted := map[string]struct{}{}
	for e := range strings.SplitSeq(only, ",") {
		if e = strings.TrimSpace(strings.ToLower(e)); e != "" {
			wanted[e] = struct{}{}
		}
	}

	var users []user
	for i, u := range file.Users {
		u.index = i
		if u.Password == "" {
			u.Password = file.Defaults.Password
		}
		if _, ok := wanted[strings.ToLower(u.Email)]; len(wanted) > 0 && !ok {
			continue
		}
		users = append(users, u)
	}

	if len(users) == 0 {
		return nil, errors.New("no users to open")
	}
	return users, nil
}

type window struct {
	user    user
	port    int
	profile string
	x, y    int
}

func (w window) open(ctx context.Context, opts options, origin string) error {
	tokens, err := login(ctx, opts.gatewayURL, w.user)
	if err != nil {
		return fmt.Errorf("login (run task seed:users first?): %w", err)
	}

	// A window still open from an earlier run is reused: Chrome would hand a
	// second launch of the same profile to it and ignore the new flags anyway.
	if !w.devToolsUp() {
		if err := w.launch(opts); err != nil {
			return err
		}
	}

	page, err := w.pageTarget(ctx, origin)
	if err != nil {
		return err
	}

	return setSession(ctx, page, origin, tokens)
}

func (w window) launch(opts options) error {
	if err := os.MkdirAll(w.profile, 0o755); err != nil {
		return err
	}

	cmd := exec.Command(opts.chrome,
		"--user-data-dir="+w.profile,
		fmt.Sprintf("--remote-debugging-port=%d", w.port),
		"--remote-allow-origins=http://localhost",
		"--no-first-run",
		"--no-default-browser-check",
		fmt.Sprintf("--window-position=%d,%d", w.x, w.y),
		fmt.Sprintf("--window-size=%d,%d", opts.width, opts.height),
		"--new-window",
		opts.webURL,
	)
	// its own session, so the windows outlive this command and its terminal
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start chrome: %w", err)
	}
	return cmd.Process.Release()
}

func (w window) devToolsUp() bool {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", w.port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// pageTarget returns the DevTools websocket URL of the window's tab showing
// the web app, or of its first tab if none does yet.
func (w window) pageTarget(ctx context.Context, origin string) (string, error) {
	deadline := time.Now().Add(20 * time.Second)
	for {
		targets, err := w.targets()
		if err == nil {
			var first string
			for _, t := range targets {
				if t.Type != "page" {
					continue
				}
				if strings.HasPrefix(t.URL, origin) {
					return t.WebSocketDebuggerURL, nil
				}
				if first == "" {
					first = t.WebSocketDebuggerURL
				}
			}
			if first != "" {
				return first, nil
			}
		}

		if time.Now().After(deadline) {
			return "", fmt.Errorf("chrome's DevTools on port %d did not come up", w.port)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

type target struct {
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func (w window) targets() ([]target, error) {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", w.port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var targets []target
	return targets, json.NewDecoder(resp.Body).Decode(&targets)
}

type tokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func login(ctx context.Context, gatewayURL string, u user) (tokenPair, error) {
	body, _ := json.Marshal(map[string]string{"email": u.Email, "password": u.Password})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gatewayURL+"/identity/api/v1/login", bytes.NewReader(body))
	if err != nil {
		return tokenPair{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return tokenPair{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return tokenPair{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var env struct {
		Data tokenPair `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return tokenPair{}, fmt.Errorf("decode response: %w", err)
	}
	return env.Data, nil
}

// setSession writes the tokens into the web app's localStorage and reloads the
// page, so the app starts logged in. localStorage belongs to an origin, so it
// first waits for the tab to be on the web app.
func setSession(ctx context.Context, wsURL, origin string, tokens tokenPair) error {
	conn, err := websocket.Dial(wsURL, "", "http://localhost")
	if err != nil {
		return fmt.Errorf("connect to devtools: %w", err)
	}
	defer conn.Close()

	cdp := &devtools{conn: conn}

	if _, err := cdp.call("Page.navigate", map[string]any{"url": origin}); err != nil {
		return err
	}

	script := fmt.Sprintf(`(() => {
		if (location.origin !== %q || document.readyState === "loading") return "wait";
		localStorage.setItem(%q, %q);
		localStorage.setItem(%q, %q);
		location.reload();
		return "ok";
	})()`, origin, accessTokenKey, tokens.AccessToken, refreshTokenKey, tokens.RefreshToken)

	deadline := time.Now().Add(20 * time.Second)
	for {
		res, err := cdp.call("Runtime.evaluate", map[string]any{"expression": script, "returnByValue": true})
		if err != nil {
			return err
		}

		var out struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(res, &out); err == nil && out.Result.Value == "ok" {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("the web app at %s did not load - is it running?", origin)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// devtools is a minimal Chrome DevTools Protocol client: one call at a time,
// skipping the events that arrive in between.
type devtools struct {
	conn   *websocket.Conn
	nextID int
}

func (d *devtools) call(method string, params any) (json.RawMessage, error) {
	d.nextID++
	id := d.nextID

	if err := websocket.JSON.Send(d.conn, map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}

	for {
		var msg struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := websocket.JSON.Receive(d.conn, &msg); err != nil {
			return nil, fmt.Errorf("%s: %w", method, err)
		}
		if msg.ID != id {
			continue
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, msg.Error.Message)
		}
		return msg.Result, nil
	}
}
