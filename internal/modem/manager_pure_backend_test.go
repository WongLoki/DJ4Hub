package modem

import (
	"testing"

	"github.com/WongLoki/DJ4Hub/internal/config"
)

func TestPureControlPlaneBackendMBIM(t *testing.T) {
	if !pureQMIBackendConfig(config.DeviceConfig{DeviceBackend: "mbim", ControlDevice: "/dev/cdc-wdm2"}) {
		t.Fatal("mbim 应视为控制面后端，跳过 AT 启动")
	}
	if !pureQMIBackendConfig(config.DeviceConfig{DeviceBackend: "mbim"}) {
		t.Fatal("mbim 无 AT 口也应跳过 AT 启动")
	}
	if !pureQMIBackendConfig(config.DeviceConfig{DeviceBackend: "qmi"}) {
		t.Fatal("qmi 仍应成立")
	}
	if pureQMIBackendConfig(config.DeviceConfig{DeviceBackend: "at", ATPort: "/dev/ttyUSB0"}) {
		t.Fatal("at 不应跳过 AT 启动")
	}
}
