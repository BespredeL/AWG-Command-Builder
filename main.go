package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"crypto/tls"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	webview2 "github.com/jchv/go-webview2"
)

const listenAddr = "127.0.0.1:18080"
const appURL = "http://" + listenAddr

//go:embed index.html
var indexHTML []byte

//go:embed i18n/languages.json
var embeddedLanguagesJSON []byte

type appState struct {
	mu         sync.RWMutex
	base       string
	authed     bool
	client     *http.Client
	i18nRaw    []byte
	i18nSource string
}

type connectRequest struct {
	Base     string `json:"base"`
	Login    string `json:"login"`
	Password string `json:"password"`
}

type commandRequest struct {
	Command string `json:"command"`
}

type authInfo struct {
	Challenge string
	Realm     string
	Status    int
}

func main() {
	client, err := newRouterClient()
	if err != nil {
		log.Fatalf("init client: %v", err)
	}
	i18nRaw, i18nSource, err := loadI18NConfig()
	if err != nil {
		log.Fatalf("i18n config: %v", err)
	}

	st := &appState{
		client:     client,
		i18nRaw:    i18nRaw,
		i18nSource: i18nSource,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", healthHandler)
	mux.HandleFunc("POST /api/connect", st.connectHandler)
	mux.HandleFunc("GET /api/interfaces", st.interfacesHandler)
	mux.HandleFunc("POST /api/command", st.commandHandler)
	mux.HandleFunc("GET /api/i18n", st.i18nHandler)
	mux.HandleFunc("GET /api/i18n/export-exe", i18nExportExeHandler)
	mux.HandleFunc("GET /api/open-external", openExternalHandler)
	mux.HandleFunc("GET /favicon.ico", faviconHandler)
	mux.HandleFunc("/", staticHandler)

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("listen %s failed: %v", listenAddr, err)
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Serve(ln)
	}()

	if err := waitForServerReady(appURL, 5*time.Second); err != nil {
		_ = srv.Shutdown(context.Background())
		log.Fatalf("backend startup check failed: %v", err)
	}

	log.Printf("AWG Command Builder backend started: %s", appURL)
	log.Printf("I18N source: %s", i18nSource)

	if err := openDesktopWindow(appURL); err != nil {
		_ = srv.Shutdown(context.Background())
		log.Fatalf("desktop window unavailable: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown warning: %v", err)
	}

	if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server stopped unexpectedly: %v", err)
	}
}

func openDesktopWindow(url string) error {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "AWG Command Builder",
			Width:  1280,
			Height: 900,
			Center: true,
		},
	})
	if w == nil {
		return fmt.Errorf("failed to initialize WebView2")
	}
	defer w.Destroy()
	w.SetSize(1280, 900, webview2.HintNone)
	w.Navigate(url)
	w.Run()
	return nil
}

func newRouterClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // local router may use self-signed cert
		},
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Timeout:   20 * time.Second,
		Jar:       jar,
		Transport: transport,
	}, nil
}

func waitForServerReady(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 800 * time.Millisecond}
	healthURL := baseURL + "/api/health"

	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("status=%d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(120 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = errors.New("unknown startup error")
	}
	return fmt.Errorf("health check timeout: %w", lastErr)
}

func staticHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexHTML)
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "awg-command-builder-backend",
	})
}

func faviconHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *appState) i18nHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-I18N-Source", s.i18nSource)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(s.i18nRaw)
}

func i18nExportExeHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="languages.json"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(embeddedLanguagesJSON)
}

func openExternalHandler(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "url is required")
		return
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		writeErr(w, http.StatusBadRequest, "invalid url")
		return
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		writeErr(w, http.StatusBadRequest, "only http/https links are allowed")
		return
	}
	if err := openInDefaultBrowser(u.String()); err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Sprintf("open browser failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func openInDefaultBrowser(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}

func (s *appState) connectHandler(w http.ResponseWriter, r *http.Request) {
	var req connectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	base := normalizeBaseURL(req.Base)
	if base == "" {
		writeErr(w, http.StatusBadRequest, "base is required")
		return
	}
	if strings.TrimSpace(req.Login) == "" {
		writeErr(w, http.StatusBadRequest, "login is required")
		return
	}
	if req.Password == "" {
		writeErr(w, http.StatusBadRequest, "password is required")
		return
	}

	// Reset client on each connect attempt to avoid stale cookies.
	client, err := newRouterClient()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Sprintf("init client failed: %v", err))
		return
	}
	s.mu.Lock()
	s.client = client
	s.mu.Unlock()

	info, err := s.fetchAuthInfo(base)
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("auth challenge failed: %v", err))
		return
	}
	if info.Status == http.StatusOK {
		s.mu.Lock()
		s.base = base
		s.authed = true
		s.mu.Unlock()

		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"base":    base,
			"message": "already authorized",
		})
		return
	}

	attempts := buildAuthAttempts(req.Login, req.Password, info)
	var lastErr error
	var usedMethod string
	for _, a := range attempts {
		err := s.postAuth(base, req.Login, a.hash)
		if err == nil {
			usedMethod = a.name
			lastErr = nil
			break
		}
		lastErr = err
	}
	if lastErr != nil {
		writeErr(w, http.StatusUnauthorized, fmt.Sprintf("auth failed: %v (challenge=%q realm=%q)", lastErr, info.Challenge, info.Realm))
		return
	}

	s.mu.Lock()
	s.base = base
	s.authed = true
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"base":      base,
		"challenge": info.Challenge,
		"realm":     info.Realm,
		"method":    usedMethod,
		"message":   "authorized",
	})
}

func (s *appState) interfacesHandler(w http.ResponseWriter, _ *http.Request) {
	base, ok := s.connection()
	if !ok {
		writeErr(w, http.StatusUnauthorized, "not connected; call /api/connect first")
		return
	}

	respBody, status, err := s.requestRCI(base+"/rci/show/interface", http.MethodGet, nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("interfaces request failed: %v", err))
		return
	}
	if status < 200 || status >= 300 {
		writeErr(w, status, fmt.Sprintf("router returned %d: %s", status, truncate(respBody, 240)))
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBody)
}

func (s *appState) commandHandler(w http.ResponseWriter, r *http.Request) {
	base, ok := s.connection()
	if !ok {
		writeErr(w, http.StatusUnauthorized, "not connected; call /api/connect first")
		return
	}

	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Command) == "" {
		writeErr(w, http.StatusBadRequest, "command is required")
		return
	}

	body := []map[string]any{
		{
			"parse": map[string]any{
				"command": req.Command,
				"execute": true,
			},
		},
	}

	payload, _ := json.Marshal(body)
	respBody, status, err := s.requestRCI(base+"/rci/", http.MethodPost, payload)
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("command request failed: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(respBody)
}

func (s *appState) fetchAuthInfo(base string) (authInfo, error) {
	req, err := http.NewRequest(http.MethodGet, base+"/auth", nil)
	if err != nil {
		return authInfo{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return authInfo{}, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return authInfo{
		Challenge: resp.Header.Get("X-NDM-Challenge"),
		Realm:     resp.Header.Get("X-NDM-Realm"),
		Status:    resp.StatusCode,
	}, nil
}

func (s *appState) postAuth(base, login, hash string) error {
	payload, _ := json.Marshal(map[string]string{
		"login":    login,
		"password": hash,
	})
	respBody, status, err := s.requestRCI(base+"/auth", http.MethodPost, payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("status=%d body=%s", status, truncate(respBody, 240))
	}
	return nil
}

func (s *appState) requestRCI(url, method string, body []byte) ([]byte, int, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, 0, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	return respBody, resp.StatusCode, nil
}

func (s *appState) connection() (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.base, s.authed && s.base != ""
}

func (s *appState) httpClient() *http.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

type authAttempt struct {
	name string
	hash string
}

func buildAuthAttempts(login, password string, info authInfo) []authAttempt {
	attempts := make([]authAttempt, 0, 6)

	// Python-style auth flow:
	// stage1 = md5(login:realm:password)
	// stage2 = sha256(challenge + stage1_hex)
	if info.Realm != "" && info.Challenge != "" {
		stage1 := md5Hex(fmt.Sprintf("%s:%s:%s", login, info.Realm, password))
		stage2 := sha256Hex(info.Challenge + stage1)
		attempts = append(attempts, authAttempt{name: "md5(login:realm:password)->sha256(challenge+md5hex)", hash: stage2})
	}

	// Legacy RCI: md5(md5(password)+challenge) or md5(password) if challenge missing.
	pwdMD5 := md5Hex(password)
	legacy := pwdMD5
	if info.Challenge != "" {
		legacy = md5Hex(pwdMD5 + info.Challenge)
	}
	attempts = append(attempts, authAttempt{name: "legacy-md5", hash: legacy})

	// RFC7616-like variant used by newer firmware.
	if info.Realm != "" {
		base := sha256Hex(fmt.Sprintf("%s:%s:%s", login, info.Realm, password))
		attempts = append(attempts, authAttempt{name: "sha256-login-realm-password", hash: base})
		if info.Challenge != "" {
			attempts = append(attempts, authAttempt{name: "sha256(base+challenge)", hash: sha256Hex(base + info.Challenge)})
		}
	}

	// Extra fallback seen on some custom builds.
	if info.Challenge != "" {
		attempts = append(attempts, authAttempt{name: "sha256(password+challenge)", hash: sha256Hex(password + info.Challenge)})
	}

	return attempts
}

func md5Hex(v string) string {
	sum := md5.Sum([]byte(v))
	return hex.EncodeToString(sum[:])
}

func sha256Hex(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func normalizeBaseURL(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(base), "http://") && !strings.HasPrefix(strings.ToLower(base), "https://") {
		base = "http://" + base
	}
	return strings.TrimSuffix(base, "/")
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"ok":    false,
		"error": msg,
	})
}

func truncate(b []byte, max int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func loadI18NConfig() ([]byte, string, error) {
	exePath, err := os.Executable()
	if err == nil {
		externalPath := filepath.Join(filepath.Dir(exePath), "languages.json")
		if b, readErr := os.ReadFile(externalPath); readErr == nil && len(bytesTrimSpace(b)) > 0 {
			if json.Valid(b) {
				return b, "external:" + externalPath, nil
			}
			log.Printf("languages.json рядом с exe невалиден, используем встроенный")
		}
	}

	if !json.Valid(embeddedLanguagesJSON) {
		return nil, "", fmt.Errorf("embedded i18n JSON is invalid")
	}
	return embeddedLanguagesJSON, "embedded", nil
}

func bytesTrimSpace(b []byte) []byte {
	return bytes.TrimSpace(b)
}
