package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyADBPassword(t *testing.T) {
	// Public vendor-issued test pair; never use a live device challenge in fixtures.
	got, err := legacyADBPassword("35787336")
	if err != nil || got != "zoCLUARzj8zsJIO" {
		t.Fatal("legacy protocol vector mismatch")
	}
	for _, s := range []string{"", "1234567", "123456789", "abcdefgh", "1234567\n"} {
		if _, err := legacyADBPassword(s); err == nil {
			t.Fatal("accepted malformed challenge")
		}
	}
}

func TestAudioUSBLocation(t *testing.T) {
	device := "+-o Baiwang\n  | \"idVendor\" = 11427\n  | \"idProduct\" = 16390\n  | \"locationID\" = 17825792\n"
	got, err := parseAudioUSBLocation(device)
	if err != nil || got != "17825792X" {
		t.Fatal(got, err)
	}
	for _, raw := range []string{"", device + device, strings.Replace(device, "17825792", "0", 1)} {
		if _, e := parseAudioUSBLocation(raw); e == nil {
			t.Fatal("unsafe location accepted")
		}
	}
}

func TestParseAudioUSB(t *testing.T) {
	raw := `+QCFG: "usbcfg",0x2CA3,0x4006,1,1,1,1,1,0,0` + "\r\nOK"
	c, err := parseAudioUSB(raw)
	if err != nil || c[7] != "0" {
		t.Fatal(c, err)
	}
	for _, s := range []string{strings.Replace(raw, "0x2CA3", "0x2c7c", 1), strings.Replace(raw, "1,1,1,1,1", "1,1,0,1,1", 1), strings.Replace(raw, "0,0\r", "0,0,1\r", 1)} {
		if _, e := parseAudioUSB(s); e == nil {
			t.Fatal("accepted unsafe USB config")
		}
	}
}

func TestInitializeAudioADB(t *testing.T) {
	for _, scenario := range []string{"success", "enabled", "busy", "identity", "backup", "auth", "unknown"} {
		t.Run(scenario, func(t *testing.T) {
			const id = "123456789012345"
			config := `+QCFG: "usbcfg",0x2CA3,0x4006,1,1,1,1,1,0,0`
			if scenario == "enabled" {
				config = strings.Replace(config, "1,0,0", "1,1,0", 1)
			}
			writes := []string{}
			auth := false
			at := func(cmd string) (string, error) {
				switch cmd {
				case `AT+QCFG="usbcfg"`:
					return config + "\r\nOK", nil
				case "AT+GSN":
					if scenario == "identity" {
						return "987654321098765\r\nOK", nil
					}
					return id + "\r\nOK", nil
				case "AT+CVERSION":
					return "VERSION: QDC507GLEFM21\r\nJan 29 2024\r\nOK", nil
				case "AT+CLCC":
					if scenario == "busy" {
						return `+CLCC: 1,1,0,0,0,"",128`, nil
					}
					return `+CLCC: 1,1,0,1,0,"",128`, nil
				case "AT+QADBKEY?":
					if scenario == "unknown" {
						return "+QADBKEY: unknown", nil
					}
					return "+QADBKEY: 35787336\r\nOK", nil
				}
				if strings.HasPrefix(cmd, "AT+QADBKEY=") {
					auth = true
					if scenario == "auth" {
						return "", errors.New("refused")
					}
					return "OK", nil
				}
				writes = append(writes, cmd)
				if strings.HasPrefix(cmd, `AT+QCFG="usbcfg",`) {
					config = strings.Replace(cmd, "AT+QCFG=", "+QCFG: ", 1)
				}
				return "OK", nil
			}
			dir := t.TempDir()
			if scenario == "backup" {
				dir = filepath.Join(dir, "file")
				if err := os.WriteFile(dir, []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			changed, err := initializeAudioADB(context.Background(), at, dir, id)
			if scenario == "success" {
				if err != nil || !changed || len(writes) != 2 || writes[0] != `AT+QCFG="usbcfg",0x2CA3,0x4006,1,1,1,1,1,1,0` || writes[1] != "AT+CFUN=1,1" {
					t.Fatal(changed, err, writes)
				}
				files, _ := os.ReadDir(dir)
				if len(files) != 1 {
					t.Fatal("backup missing")
				}
				info, _ := files[0].Info()
				if info.Mode().Perm() != 0600 {
					t.Fatal("backup permissions")
				}
				content, _ := os.ReadFile(filepath.Join(dir, files[0].Name()))
				if strings.Contains(string(content), "zoCLU") {
					t.Fatal("password leaked")
				}
			} else if scenario == "enabled" {
				if err != nil || changed || auth || len(writes) > 0 {
					t.Fatal("not idempotent")
				}
			} else if err == nil || len(writes) > 0 {
				t.Fatal("unsafe write", scenario, err, writes)
			}
		})
	}
}
