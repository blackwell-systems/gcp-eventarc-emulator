package logger

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// captureLog redirects the standard logger output to a buffer for the duration
// of fn and returns what was written.
func captureLog(fn func()) string {
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)
	// Remove timestamp flags so we only see the message prefix.
	origFlags := log.Flags()
	log.SetFlags(0)
	defer log.SetFlags(origFlags)
	fn()
	return buf.String()
}

func TestNew_DefaultsToInfo(t *testing.T) {
	l := New("")

	// info should appear
	out := captureLog(func() {
		l.Info("hello info")
	})
	if !strings.Contains(out, "[INFO]") {
		t.Errorf("expected [INFO] in output, got: %q", out)
	}

	// debug should NOT appear at info level
	out = captureLog(func() {
		l.Debug("hello debug")
	})
	if strings.Contains(out, "[DEBUG]") {
		t.Errorf("expected no [DEBUG] output at info level, got: %q", out)
	}
}

func TestLogger_Debug(t *testing.T) {
	l := New("debug")

	out := captureLog(func() {
		l.Debug("this is debug output")
	})
	if !strings.Contains(out, "[DEBUG]") {
		t.Errorf("expected [DEBUG] prefix, got: %q", out)
	}
	if !strings.Contains(out, "this is debug output") {
		t.Errorf("expected message in output, got: %q", out)
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	l := New("error")

	debugOut := captureLog(func() { l.Debug("debug msg") })
	infoOut := captureLog(func() { l.Info("info msg") })
	warnOut := captureLog(func() { l.Warn("warn msg") })
	errorOut := captureLog(func() { l.Error("error msg") })

	if strings.Contains(debugOut, "[DEBUG]") {
		t.Errorf("error-level logger should suppress debug, got: %q", debugOut)
	}
	if strings.Contains(infoOut, "[INFO]") {
		t.Errorf("error-level logger should suppress info, got: %q", infoOut)
	}
	if strings.Contains(warnOut, "[WARN]") {
		t.Errorf("error-level logger should suppress warn, got: %q", warnOut)
	}
	if !strings.Contains(errorOut, "[ERROR]") {
		t.Errorf("error-level logger should emit error, got: %q", errorOut)
	}
}

func TestLogger_IsDebug(t *testing.T) {
	if !New("debug").IsDebug() {
		t.Error("expected IsDebug() == true for debug logger")
	}
	if New("info").IsDebug() {
		t.Error("expected IsDebug() == false for info logger")
	}
	if New("warn").IsDebug() {
		t.Error("expected IsDebug() == false for warn logger")
	}
	if New("error").IsDebug() {
		t.Error("expected IsDebug() == false for error logger")
	}
	// empty string defaults to info
	if New("").IsDebug() {
		t.Error("expected IsDebug() == false for default (info) logger")
	}
}
