package hml

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

// assert is a minimal assertion helper, keeping the module stdlib-only.
type assert struct {
	t testing.TB
}

func newAssert(t testing.TB) *assert {
	t.Helper()
	return &assert{t: t}
}

// OK fails the test when cond is false and prints the calling source line.
func (a *assert) OK(cond bool) {
	a.t.Helper()
	if cond {
		return
	}
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		a.t.Fatal("assertion failed")
		return
	}
	src := ""
	b, err := os.ReadFile(file)
	if err == nil {
		lines := strings.Split(string(b), "\n")
		if line >= 1 && line <= len(lines) {
			src = strings.TrimSpace(lines[line-1])
		}
	}
	if src == "" {
		src = "assertion failed"
	}
	a.t.Fatal(src)
}
