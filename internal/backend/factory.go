package backend

import (
	"fmt"
	"strings"

	"github.com/WongLoki/dj-4g-hub/internal/modem"
	"github.com/WongLoki/dj-4g-hub/pkg/logger"
)

// 后端模式常量
const (
	BackendAT   = "at"
	BackendQMI  = "qmi"
	BackendMBIM = "mbim"
)

// NormalizeBackendMode 标准化后端模式字符串
func NormalizeBackendMode(in string) string {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case "", BackendAT:
		return BackendAT // 默认 AT 模式
	case BackendQMI:
		return BackendQMI
	case BackendMBIM:
		return BackendMBIM
	default:
		return BackendAT
	}
}

// ValidateBackendMode 验证后端模式是否有效
func ValidateBackendMode(in string) error {
	switch NormalizeBackendMode(in) {
	case BackendAT, BackendQMI, BackendMBIM:
		return nil
	default:
		return fmt.Errorf("无效的 device_backend 值: %q (可选: at, qmi, mbim)", in)
	}
}

// NewBackend 根据配置模式创建对应后端实例的工厂方法
// mode: "at" | "qmi"
// controlPath: QMI 控制设备路径（qmi 模式必须）
// m: modem.Manager（at 模式必须）
// source: QMI Core 资源源（qmi 模式必须）
func NewBackend(mode, controlPath string, m *modem.Manager, source QMISource, mbimSource MBIMSource) (DeviceBackend, error) {
	mode = NormalizeBackendMode(mode)

	switch mode {
	case BackendAT:
		if m == nil {
			return nil, fmt.Errorf("AT 模式需要 modem.Manager")
		}
		logger.Info("[backend] 使用 AT 后端模式")
		return NewATBackend(m), nil

	case BackendQMI:
		b, err := NewQMIBackend(controlPath, source)
		if err != nil {
			return nil, fmt.Errorf("QMI 后端初始化失败: %w", err)
		}
		logger.Info("[backend] 使用 QMI 后端模式", "control_path", controlPath)
		return b, nil

	case BackendMBIM:
		if mbimSource == nil {
			return nil, fmt.Errorf("MBIM 模式需要 MBIMSource")
		}
		logger.Info("[backend] 使用 MBIM 后端模式", "control_path", controlPath)
		return NewMBIMBackend(controlPath, mbimSource), nil

	default:
		return nil, fmt.Errorf("不支持的后端模式: %s", mode)
	}
}
