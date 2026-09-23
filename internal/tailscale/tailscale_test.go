package tailscale

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRunner struct {
	status                       []byte
	statusErr, pathErr, startErr error
	checks, starts               int
	args                         []string
	proc                         *fakeProcess
	stdout, stderr               io.Writer
}

func (r *fakeRunner) LookPath() error                            { r.checks++; return r.pathErr }
func (r *fakeRunner) StatusJSON(context.Context) ([]byte, error) { return r.status, r.statusErr }
func (r *fakeRunner) StartServe(_ context.Context, port, target string, stdout, stderr io.Writer) (process, error) {
	r.starts++
	r.args = serveArgs(port, target)
	r.stdout, r.stderr = stdout, stderr
	if r.startErr != nil {
		return nil, r.startErr
	}
	r.proc = newFakeProcess()
	return r.proc, nil
}

type fakeProcess struct {
	done                     chan struct{}
	once                     sync.Once
	mu                       sync.Mutex
	interrupts, kills, waits int
}

func newFakeProcess() *fakeProcess { return &fakeProcess{done: make(chan struct{})} }
func (p *fakeProcess) Wait() error { <-p.done; p.mu.Lock(); p.waits++; p.mu.Unlock(); return nil }
func (p *fakeProcess) Interrupt() error {
	p.mu.Lock()
	p.interrupts++
	p.mu.Unlock()
	p.once.Do(func() { close(p.done) })
	return nil
}
func (p *fakeProcess) Kill() error {
	p.mu.Lock()
	p.kills++
	p.mu.Unlock()
	p.once.Do(func() { close(p.done) })
	return nil
}

func TestActionsAndOutput(t *testing.T) {
	for _, action := range []Action{Default, OnlyCheck, CheckAndStart, OnlyStart} {
		t.Run(string(rune('0'+action)), func(t *testing.T) {
			r := &fakeRunner{status: []byte(`{}`)}
			var out, errOut bytes.Buffer
			s, err := start(context.Background(), Config{LocalAddr: ":50000", HTTPSPort: 443, Action: action, AllowReplaceExisting: action == OnlyStart, Stdout: &out, Stderr: &errOut}, r)
			if err != nil {
				t.Fatal(err)
			}
			if (r.checks == 0) != (action == OnlyStart) {
				t.Fatalf("checks = %d", r.checks)
			}
			if action == OnlyCheck {
				if s != nil || r.starts != 0 {
					t.Fatal("OnlyCheck started process")
				}
				return
			}
			if !reflect.DeepEqual(r.args, []string{"serve", "--yes", "--https", "443", "http://127.0.0.1:50000"}) {
				t.Fatalf("args = %v", r.args)
			}
			_, _ = r.stdout.Write([]byte("Visit https://host.ts.net:443/\n"))
			_, _ = r.stderr.Write([]byte("diagnostic"))
			if url := <-s.URL(); url != "https://host.ts.net:443" {
				t.Fatalf("url = %q", url)
			}
			if !strings.Contains(out.String(), "Visit") || errOut.String() != "diagnostic" {
				t.Fatal("output lost")
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			if err := s.Wait(); err != nil {
				t.Fatal(err)
			}
			p := r.proc
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.waits != 1 || p.interrupts != 1 {
				t.Fatalf("waits=%d interrupts=%d", p.waits, p.interrupts)
			}
		})
	}
}
func TestURLWriterWaitsForDelimiter(t *testing.T) {
	w := newURLWriter()
	_, _ = w.Write([]byte("https://host.ts"))
	select {
	case <-w.ch:
		t.Fatal("reported incomplete URL")
	default:
	}
	_, _ = w.Write([]byte(".net:443/\n"))
	if got := <-w.ch; got != "https://host.ts.net:443" {
		t.Fatalf("URL = %q", got)
	}
}

func TestURLWriterReportsWithoutDelimiterWhileRunning(t *testing.T) {
	w := newURLWriter()
	_, _ = w.Write([]byte("https://host.ts.net:443"))
	select {
	case got := <-w.ch:
		if got != "https://host.ts.net:443" {
			t.Fatalf("URL = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("URL was not reported while process is running")
	}
}

func TestURLWriterSplitURLBeforeQuietPeriod(t *testing.T) {
	w := newURLWriter()
	_, _ = w.Write([]byte("https://host.ts"))
	select {
	case <-w.ch:
		t.Fatal("reported incomplete URL")
	case <-time.After(urlQuietPeriod / 4):
	}
	_, _ = w.Write([]byte(".net:443/"))
	select {
	case got := <-w.ch:
		if got != "https://host.ts.net:443" {
			t.Fatalf("URL = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("split URL was not reported")
	}
}

func TestSessionReportsUndelimitedURLWhileRunning(t *testing.T) {
	r := &fakeRunner{}
	s, err := start(context.Background(), Config{LocalAddr: ":1234", HTTPSPort: 443, Action: OnlyStart, AllowReplaceExisting: true}, r)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, _ = r.stderr.Write([]byte("https://host.ts.net:443"))
	select {
	case got := <-s.URL():
		if got != "https://host.ts.net:443" {
			t.Fatalf("URL = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("URL was not reported before process exit")
	}
	select {
	case <-s.done:
		t.Fatal("process exited before URL was reported")
	default:
	}
}

func TestURLStreamsDoNotInterleave(t *testing.T) {
	r := &fakeRunner{status: []byte(`{}`)}
	s, err := start(context.Background(), Config{LocalAddr: ":1234", HTTPSPort: 443, Action: OnlyStart, AllowReplaceExisting: true}, r)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, _ = r.stdout.Write([]byte("https://host.ts"))
	_, _ = r.stderr.Write([]byte("diagnostic output\n"))
	_, _ = r.stdout.Write([]byte(".net:443/\n"))
	if got := <-s.URL(); got != "https://host.ts.net:443" {
		t.Fatalf("URL = %q", got)
	}
}

func TestStatusHasHTTPSPortAcrossStatusShapes(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		want         bool
	}{
		{"top TCP with wrapper", `{"TCP":{"443":{"HTTPS":true}},"ServeConfig":{"TCP":{}}}`, true},
		{"top Web with wrapper", `{"Web":{"host.ts.net:443":{}},"ServeConfig":{"Web":{}}}`, true},
		{"wrapped TCP", `{"ServeConfig":{"TCP":{"443":{"HTTPS":true}}}}`, true},
		{"wrapped Web", `{"ServeConfig":{"Web":{"host.ts.net:443":{}}}}`, true},
		{"non HTTPS TCP", `{"TCP":{"443":{"HTTPS":false}},"ServeConfig":{"TCP":{"8443":{"HTTPS":true}}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusHasHTTPSPort([]byte(tc.status), "443"); got != tc.want {
				t.Fatalf("statusHasHTTPSPort = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidationAndPreflight(t *testing.T) {
	for _, cfg := range []Config{{LocalAddr: ":0", HTTPSPort: 443}, {LocalAddr: "127.0.0.1:65536", HTTPSPort: 443}, {LocalAddr: "bad", HTTPSPort: 443}, {LocalAddr: ":50000", HTTPSPort: 0}, {LocalAddr: ":50000", HTTPSPort: 65536}, {LocalAddr: ":50000", HTTPSPort: 443, Action: 99}} {
		r := &fakeRunner{}
		if _, err := start(context.Background(), cfg, r); err == nil || r.starts != 0 {
			t.Errorf("config %+v: error %v starts %d", cfg, err, r.starts)
		}
	}
	r := &fakeRunner{status: []byte(`{"TCP":{"2020":{"HTTPS":true},"50000":{"HTTPS":false}},"Web":{"host.ts.net:8443":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:50000"}}}}}`)}
	if _, err := start(context.Background(), Config{HTTPSPort: 443, Action: OnlyCheck}, r); err != nil {
		t.Fatal(err)
	}
	for _, port := range []int{2020, 8443} {
		if _, err := start(context.Background(), Config{LocalAddr: ":50000", HTTPSPort: port, Action: OnlyCheck}, r); err == nil {
			t.Errorf("port %d not detected", port)
		}
	}
	if _, err := start(context.Background(), Config{LocalAddr: ":50000", HTTPSPort: 50000, Action: OnlyCheck}, r); err != nil {
		t.Fatal(err)
	}
	r.pathErr = errors.New("missing")
	if _, err := start(context.Background(), Config{LocalAddr: ":50000", HTTPSPort: 443, Action: OnlyCheck}, r); err == nil {
		t.Fatal("missing binary")
	}
	r.pathErr = nil
	r.statusErr = errors.New("exit 1")
	r.status = []byte("no serve config")
	if _, err := start(context.Background(), Config{LocalAddr: ":50000", HTTPSPort: 443, Action: OnlyCheck}, r); err != nil {
		t.Fatal(err)
	}
	r.status = []byte("unexpected error")
	if _, err := start(context.Background(), Config{LocalAddr: ":50000", HTTPSPort: 443, Action: OnlyCheck}, r); err == nil {
		t.Fatal("status failure")
	}
	r.statusErr = nil
	if _, err := start(context.Background(), Config{HTTPSPort: 443, Action: OnlyCheck}, r); err == nil {
		t.Fatal("invalid status JSON accepted")
	}
}
func TestOnlyStartRequiresReplacementOptIn(t *testing.T) {
	r := &fakeRunner{status: []byte(`{}`)}
	if _, err := start(context.Background(), Config{LocalAddr: ":1234", HTTPSPort: 443, Action: OnlyStart}, r); err == nil {
		t.Fatal("OnlyStart must reject the default replacement policy")
	}
	if r.checks != 0 || r.starts != 0 {
		t.Fatal("rejected action ran a Tailscale command")
	}
}

func TestOnlyStartSkipsPreflightAndCancelReaps(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &fakeRunner{pathErr: errors.New("missing")}
	s, err := start(ctx, Config{LocalAddr: ":1234", HTTPSPort: 443, Action: OnlyStart, AllowReplaceExisting: true}, r)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	done := make(chan error, 1)
	go func() { done <- s.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not reap")
	}
	if _, ok := <-s.URL(); ok {
		t.Fatal("expected closed URL channel")
	}
	if r.checks != 0 {
		t.Fatal("OnlyStart checked status")
	}
}

type stubbornProcess struct {
	done   chan struct{}
	once   sync.Once
	killed bool
}

func (p *stubbornProcess) Wait() error      { <-p.done; return nil }
func (p *stubbornProcess) Interrupt() error { return nil }
func (p *stubbornProcess) Kill() error {
	p.killed = true
	p.once.Do(func() { close(p.done) })
	return nil
}

func TestCloseKillsUnresponsiveProcess(t *testing.T) {
	p := &stubbornProcess{done: make(chan struct{})}
	s := &Session{proc: p, url: newURLWriter(), done: make(chan struct{})}
	go func() { _ = p.Wait(); s.url.close(); close(s.done) }()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if !p.killed {
		t.Fatal("process not killed after timeout")
	}
	if _, ok := <-s.URL(); ok {
		t.Fatal("URL not closed")
	}
}

func TestExitAndStartFailure(t *testing.T) {
	r := &fakeRunner{startErr: io.EOF}
	if _, err := start(context.Background(), Config{LocalAddr: ":1234", HTTPSPort: 443, Action: OnlyStart, AllowReplaceExisting: true}, r); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	r.startErr = nil
	s, err := start(context.Background(), Config{LocalAddr: ":1234", HTTPSPort: 443, Action: OnlyStart, AllowReplaceExisting: true}, r)
	if err != nil {
		t.Fatal(err)
	}
	r.proc.once.Do(func() { close(r.proc.done) })
	if err := s.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-s.URL(); ok {
		t.Fatal("URL not closed")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
