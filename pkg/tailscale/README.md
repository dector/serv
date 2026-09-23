# pkg/tailscale

`Start(ctx, Config)` manages **only** a foreground `tailscale serve` process. It does not open a local listener, run an HTTP server, or install signal handlers. Your application must start its backend before calling `Start` and stop it after `Session.Close`/`Wait`.

```go
cfg := tailscale.Config{LocalAddr: "127.0.0.1:50000", HTTPSPort: 443}
s, err := tailscale.Start(ctx, cfg)
if err != nil { return err }
defer s.Close()
// s.URL() delivers the first HTTPS URL (or closes on process exit).
// s.Wait() waits for the command to exit.
return s.Wait()
```

`LocalAddr` is a host:port (and may be omitted for `OnlyCheck`); `:50000` becomes `127.0.0.1:50000`. Ports must be in 1–65535, so `:0` is rejected: bind first, then pass the actual port. `Stdout` and `Stderr` are optional output writers; URL discovery works even when both are nil. `Session.URL()` yields at most one URL and closes when the process exits. `Wait()` can be called repeatedly; `Close()` interrupts, waits up to three seconds, then kills and reaps the process. Cancelling the context also closes the session. A normal signal-induced exit may appear as an error from `Wait()`; applications that initiated shutdown can ignore that result.

Actions: `Default` (zero) is `CheckAndStart`. `OnlyCheck` checks the binary and `tailscale serve status --json`, returning `(nil, nil)` if free. `CheckAndStart` checks and then starts; `OnlyStart` skips preflight and requires `AllowReplaceExisting: true` (false by default). Without that explicit opt-in, `OnlyStart` returns an error without running Tailscale. The check detects configured HTTPS ports in both `Web` and `TCP` status entries, not proxy backend ports. **The check is not atomic.** `--yes` means “update without interactive prompts,” not “replace only if requested”; even `CheckAndStart` can race with another configurator between check and start and replace its configuration. `AllowReplaceExisting: false` does not guarantee that replacement is impossible. Do not rely on either action as a lock. This package never invokes `serve reset` or `serve off`.

## Choosing a backend port

For a stable high-port choice using [nettw](https://pkg.go.dev/github.com/dector/nettw), bind the selected address and keep its listener alive for the lifetime of the session. The availability check/selection is not a reservation, so `net.Listen` can still fail; handle that error (or retry selection).

```go
// Imports: context, net, github.com/dector/nettw,
// github.com/dector/serv/pkg/tailscale. Assume ctx is your app's context.
rootPath := "/path/to/project" // same seed gives a stable port choice
port, err := nettw.ParsePortOrPickAnother("random",
    nettw.WithIgnoreInvalidPort(true), nettw.WithSeed(rootPath),
    nettw.WithPortRange(49152, 65535))
if err != nil { return err }
ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port.Str))
if err != nil { return err }
defer ln.Close()
// Start your HTTP server on ln and wait until it is ready before calling Start.
s, err := tailscale.Start(ctx, tailscale.Config{
    LocalAddr: ln.Addr().String(), HTTPSPort: 443,
})
if err != nil { return err }
defer s.Close()
return s.Wait()
```

Alternatively, let the OS safely reserve an available local port with `ln, err := net.Listen("tcp", "127.0.0.1:0")`, then use `ln.Addr().String()` as `LocalAddr`. This is not stable across runs, but avoids the selection-to-bind race. In either case, start the server on `ln` before exposing it.
