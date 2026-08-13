// Package backend 定义 DJ 4G Hub 的设备后端抽象层。
// AT 和 QMI 是平等的两种后端实现，通过配置开关 device_backend 选择。
package backend

// DeviceBackend 顶层聚合接口 — 所有后端模式（at/qmi/auto）均实现此接口
type DeviceBackend interface {
	DeviceInfoProvider
	SMSProvider
	OperatingModeController
	SIMAuthProvider

	// Mode 返回当前后端模式标识: "at" | "qmi"
	Mode() string

	// Close 释放后端持有的资源（QMI service 连接等）
	Close() error
}
