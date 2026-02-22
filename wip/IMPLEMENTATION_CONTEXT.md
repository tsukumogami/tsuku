## Description

The `Spinner` type in `internal/progress` has a data race between the `animate()` goroutine and the `Stop()`/`StopWithMessage()` methods. Both write to `s.output` (a `bytes.Buffer` in tests) concurrently without synchronizing access to the writer.

This causes `TestSpinner_TTY_SetMessage` and `TestSpinner_DoubleStop` (and potentially other TTY spinner tests) to fail under the race detector (`go test -race`).

## Impact

The race detector is only enabled on pushes to `main` (not on PRs), so the bug wasn't caught during PR review. It has now failed **twice** on `main` in separate runs:

- Run [22266075744](https://github.com/tsukumogami/tsuku/actions/runs/22266075744) (commit `378cdb5`) -- `TestSpinner_TTY_SetMessage` and `TestSpinner_DoubleStop`
- Run [22264135059](https://github.com/tsukumogami/tsuku/actions/runs/22264135059) (commit `d79f344`) -- `TestSpinner_DoubleStop` and `TestSpinner_DoubleStopWithMessage`

Neither commit modified the spinner code. The spinner was introduced in PR #1810 (commit `97ef9ea`) and the race has been latent since then.

## Root cause

The `Spinner.mu` mutex protects the `message` field and the `stopped` flag, but does **not** protect writes to `s.output`. Two goroutines write to the same `io.Writer` concurrently:

1. The `animate()` goroutine writes spinner frames via `fmt.Fprint(s.output, line)` at `spinner.go:126`
2. `Stop()` writes the clear-line sequence via `fmt.Fprintf(s.output, ...)` at `spinner.go:81`

After `Stop()` closes the `done` channel, there's a window where `animate()` may still be executing its `fmt.Fprint` call (it already entered the `case <-ticker.C` branch before `done` was closed). Meanwhile, `Stop()` proceeds to write the clear-line escape to the same buffer.

With a `bytes.Buffer` (used in tests), this is a data race on the buffer's internal slice. With `os.Stderr` (production), concurrent writes are technically safe at the kernel level but can produce interleaved output.

## Error output (from run 22266075744)

```
==================
WARNING: DATA RACE
Write at 0x00c00018e740 by goroutine 20:
  bytes.(*Buffer).Write()
      /opt/hostedtoolcache/go/1.25.7/x64/src/bytes/buffer.go:182 +0x3a
  fmt.Fprintf()
      /opt/hostedtoolcache/go/1.25.7/x64/src/fmt/print.go:225 +0xaa
  github.com/tsukumogami/tsuku/internal/progress.(*Spinner).Stop()
      /home/runner/work/tsuku/tsuku/internal/progress/spinner.go:81 +0x1a4
  github.com/tsukumogami/tsuku/internal/progress.TestSpinner_TTY_SetMessage()
      /home/runner/work/tsuku/tsuku/internal/progress/spinner_test.go:63 +0x304

Previous write at 0x00c00018e740 by goroutine 21:
  bytes.(*Buffer).Write()
      /opt/hostedtoolcache/go/1.25.7/x64/src/bytes/buffer.go:182 +0x3a
  fmt.Fprint()
      /opt/hostedtoolcache/go/1.25.7/x64/src/fmt/print.go:263 +0x84
  github.com/tsukumogami/tsuku/internal/progress.(*Spinner).animate()
      /home/runner/work/tsuku/tsuku/internal/progress/spinner.go:126 +0x1b0
  github.com/tsukumogami/tsuku/internal/progress.(*Spinner).Start.gowrap1()
      /home/runner/work/tsuku/tsuku/internal/progress/spinner.go:57 +0x33
==================
--- FAIL: TestSpinner_TTY_SetMessage (0.50s)
    testing.go:1570: race detected during execution of test

--- FAIL: TestSpinner_DoubleStop (0.15s)
    testing.go:1570: race detected during execution of test
FAIL
FAIL	github.com/tsukumogami/tsuku/internal/progress	3.081s
```

## Suggested fix

Hold the mutex around all writes to `s.output`, or wait for the `animate()` goroutine to exit before writing the final clear/message in `Stop()`/`StopWithMessage()`. For example, replace the `done` channel with a signaling mechanism that lets `Stop` wait for `animate` to return:

```go
func (s *Spinner) Stop() {
    s.mu.Lock()
    if s.stopped {
        s.mu.Unlock()
        return
    }
    s.stopped = true
    s.mu.Unlock()

    close(s.done)
    <-s.animateDone // wait for animate() goroutine to finish

    if s.isTTY {
        fmt.Fprintf(s.output, "\r%s\r", strings.Repeat(" ", 80))
    }
}
```

Where `animate()` closes `s.animateDone` when it returns. This ensures no concurrent writes to `s.output`.
