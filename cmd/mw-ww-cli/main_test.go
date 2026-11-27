package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/A-L-L-A-H/mw-ww/internal/sweeper"
)

func TestVersionAndDrivesJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--version", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("--version failed: code=%d err=%s", code, stderr.String())
	}
	var ver map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &ver); err != nil || ver["name"] != "mw-ww-cli" {
		t.Fatalf("unexpected version JSON: %q err=%v", stdout.String(), err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"--drives", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("--drives failed: code=%d err=%s", code, stderr.String())
	}
	var drives []sweeper.DriveInfo
	if err := json.Unmarshal(stdout.Bytes(), &drives); err != nil || len(drives) == 0 {
		t.Fatalf("unexpected drives JSON: %q err=%v", stdout.String(), err)
	}

	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"--empire", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("--empire failed: code=%d err=%s", code, stderr.String())
	}
	var emp sweeper.EmpireSweepSummary
	if err := json.Unmarshal(stdout.Bytes(), &emp); err != nil || len(emp.Groups) != 5 {
		t.Fatalf("unexpected empire JSON: %q err=%v", stdout.String(), err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"--watchdog", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("--watchdog failed: code=%d err=%s", code, stderr.String())
	}
	var alerts []sweeper.TrayWatchdogAlert
	if err := json.Unmarshal(stdout.Bytes(), &alerts); err != nil || len(alerts) == 0 {
		t.Fatalf("unexpected watchdog JSON: %q err=%v", stdout.String(), err)
	}
}

func TestScanCleanSafeAndNtfyWebhook(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "tmp")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sampleFile := filepath.Join(cacheDir, "artifact.bin")
	if err := os.WriteFile(sampleFile, []byte("0123456789abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}

	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		receivedBody = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// 1. Scan JSON
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--scan", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("--scan failed: code=%d err=%s", code, stderr.String())
	}
	var scanRep sweeper.ScanReport
	if err := json.Unmarshal(stdout.Bytes(), &scanRep); err != nil || scanRep.ScannedBytes != 16 {
		t.Fatalf("unexpected scan report: %+v err=%v", scanRep, err)
	}

	// 2. Dry-run clean preserves file and posts to ntfy
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"--clean-safe", "--path", cacheDir, "--dry-run", "--json", "--ntfy-url", srv.URL}, &stdout, &stderr); code != 0 {
		t.Fatalf("--clean-safe dry-run failed: code=%d err=%s", code, stderr.String())
	}
	var dryRes sweeper.CleanResult
	if err := json.Unmarshal(stdout.Bytes(), &dryRes); err != nil || !dryRes.DryRun || dryRes.FreedBytes != 16 {
		t.Fatalf("unexpected dry-run result: %+v err=%v", dryRes, err)
	}
	if _, err := os.Stat(sampleFile); err != nil {
		t.Fatalf("dry-run must preserve file: %v", err)
	}
	if !strings.Contains(receivedBody, "mw-ww clean (dryRun=true)") {
		t.Fatalf("expected ntfy webhook payload, got %q", receivedBody)
	}

	// 3. Apply removes contents
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"--clean-safe", "--path", cacheDir, "--apply", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("--clean-safe --apply failed: code=%d err=%s", code, stderr.String())
	}
	var liveRes sweeper.CleanResult
	if err := json.Unmarshal(stdout.Bytes(), &liveRes); err != nil || liveRes.DryRun || liveRes.DeletedFiles != 1 {
		t.Fatalf("unexpected live result: %+v err=%v", liveRes, err)
	}
	if _, err := os.Stat(sampleFile); !os.IsNotExist(err) {
		t.Fatalf("expected file deleted after --apply, err=%v", err)
	}
}
