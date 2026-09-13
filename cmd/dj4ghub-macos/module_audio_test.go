package main

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAudioUSBModeWhitelistAndRestore(t *testing.T) {
	start := strings.Index(moduleAudioScript, `case "$original" in`)
	if start < 0 {
		t.Fatal("missing USB mode whitelist")
	}
	end := strings.Index(moduleAudioScript[start:], "esac")
	if end < 0 {
		t.Fatal("missing case end")
	}
	check := "original=$1\n" + moduleAudioScript[start:start+end+len("esac")]
	for _, mode := range []string{"diag,serial,rmnet,ffs", "diag,serial,ecm,ffs", "diag,serial,ecm,ffs,audio", "diag,serial,rmnet,ffs,audio", "diag,serial,ecm,ffs,audio,audio", "diag,serial,rndis,ffs", "", "ecm", "diag,serial,rmnet,ffs\nmalicious"} {
		base := strings.TrimSuffix(mode, ",audio")
		want := base == "diag,serial,rmnet,ffs" || base == "diag,serial,ecm,ffs"
		if supportedAudioFunctions(mode) != want {
			t.Fatalf("Go validation: %q", mode)
		}
		out, err := exec.Command("sh", "-c", check+"\nprintf '%s' \"$audio_functions\"", "test", mode).Output()
		if (err == nil) != want {
			t.Fatalf("shell validation: %q: %v", mode, err)
		}
		if want && string(out) != base+",audio" {
			t.Fatalf("duplicate or missing audio function: %q", out)
		}
	}
	for _, fragment := range []string{`printf '%s' "$audio_functions" > "$base/functions"`, `printf '%s' "$original" > "$base/functions"`, `test "$(cat "$base/functions")" = "$original"`} {
		if !strings.Contains(moduleAudioScript, fragment) {
			t.Fatalf("lost original configuration handling: %s", fragment)
		}
	}
}

func TestAudioDefaultPathsAndOverrides(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DJ4GHUB_MODULE_VOICE_DIR", "")
	t.Setenv("DJ4GHUB_ADB_PATH", "")
	t.Setenv("PATH", "")
	dir, _, err := moduleAudioPaths()
	if err != nil || !strings.HasSuffix(dir, filepath.Join("DJ4Hub", "experimental-audio")) {
		t.Fatalf("default path %q: %v", dir, err)
	}
	t.Setenv("DJ4GHUB_MODULE_VOICE_DIR", "/custom/voice")
	t.Setenv("DJ4GHUB_ADB_PATH", "/custom/adb")
	dir, adb, err := moduleAudioPaths()
	if err != nil || dir != "/custom/voice" || adb != "/custom/adb" {
		t.Fatalf("overrides ignored: %s %s %v", dir, adb, err)
	}
}

func TestAudioImportVerifiedSnapshotAndPreserveExisting(t *testing.T) {
	original := moduleAudioHashes
	defer func() { moduleAudioHashes = original }()
	data := []byte("test fixture, never executed")
	moduleAudioHashes = map[string]string{"fixture.ko": fmt.Sprintf("%x", sha256.Sum256(data))}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "fixture.ko"), data, 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "runtime")
	t.Setenv("DJ4GHUB_MODULE_VOICE_DIR", destination)
	if err := installAudioRuntime(source); err != nil {
		t.Fatal(err)
	}
	if _, err := audioRuntimeFiles(destination); err != nil {
		t.Fatal(err)
	}
	if err := installAudioRuntime(source); err != nil {
		t.Fatalf("valid reinstall: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destination, "fixture.ko"), []byte("user data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := installAudioRuntime(source); err == nil {
		t.Fatal("existing invalid installation overwritten")
	}
	got, err := os.ReadFile(filepath.Join(destination, "fixture.ko"))
	if err != nil || string(got) != "user data" {
		t.Fatal("existing data changed")
	}
}

func TestModuleAudioTarget(t *testing.T) {
	valid := "List of devices attached\n(no serial number) device usb:34603008X transport_id:7\n"
	usb, transport, err := moduleAudioTarget(valid, "34603008X")
	if err != nil || usb != "34603008X" || transport != "7" {
		t.Fatalf("unexpected target %s %s %v", usb, transport, err)
	}
	for _, test := range []struct {
		name     string
		list     string
		expected string
	}{
		{"changed", valid, "other"},
		{"offline", strings.Replace(valid, " device ", " offline ", 1), ""},
		{"multiple", valid + "phone device usb:other transport_id:8\n", ""},
		{"network-only", "127.0.0.1:5555 device transport_id:9\n", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := moduleAudioTarget(test.list, test.expected); err == nil {
				t.Fatal("unsafe target accepted")
			}
		})
	}
}

func TestModuleAudioLocalBoundary(t *testing.T) {
	for _, test := range []struct {
		name    string
		remote  string
		host    string
		origin  string
		header  string
		allowed bool
	}{
		{"local-cli", "127.0.0.1:1234", "localhost:7576", "", "1", true},
		{"same-origin", "[::1]:1234", "localhost:7576", "http://localhost:7576", "1", true},
		{"cross-origin", "127.0.0.1:1234", "localhost:7576", "https://example.com", "1", false},
		{"simple-post", "127.0.0.1:1234", "localhost:7576", "", "", false},
		{"remote", "192.168.1.2:1234", "localhost:7576", "", "1", false},
		{"dns-rebind", "127.0.0.1:1234", "evil.example:7576", "", "1", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://localhost:7576/api/calls/audio/prepare", nil)
			r.RemoteAddr = test.remote
			r.Host = test.host
			r.Header.Set("Origin", test.origin)
			r.Header.Set("X-DJ4Hub-Audio", test.header)
			if allowModuleAudio(r) != test.allowed {
				t.Fatal("incorrect access decision")
			}
		})
	}
}

func TestAudioRuntimeRejectsMissingAndCorruptFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := audioRuntimeFiles(dir); err == nil {
		t.Fatal("missing runtime accepted")
	}
	for name := range moduleAudioHashes {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("untrusted"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := audioRuntimeFiles(dir); err == nil {
		t.Fatal("corrupt runtime accepted")
	}
}

func TestModuleAudioRejectsForeignTokenWithoutDeviceAccess(t *testing.T) {
	a := &app{audioSession: &moduleAudioSession{token: "owner", lastLease: time.Now()}}
	r := httptest.NewRequest(http.MethodPost, "http://localhost:7576/api/calls/audio/stop", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-DJ4Hub-Audio", "1")
	r.Header.Set("X-DJ4Hub-Audio-Token", "other")
	w := httptest.NewRecorder()
	a.moduleAudioStop(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("status %d", w.Code)
	}
}

func TestModuleAudioLeaseExpiresInStatus(t *testing.T) {
	a := &app{audioSession: &moduleAudioSession{token: "owner", lastLease: time.Now().Add(-time.Minute)}}
	r := httptest.NewRequest(http.MethodGet, "http://localhost:7576/api/calls/audio", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	a.moduleAudioStatus(w, r)
	if strings.Contains(w.Body.String(), `"active":true`) || strings.Contains(w.Body.String(), "owner") {
		t.Fatal("stale session or secret exposed")
	}
}
