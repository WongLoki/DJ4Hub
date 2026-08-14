package config

import (
	"sync"

	"github.com/WongLoki/DJ4Hub/pkg/logger"
)

var (
	globalConfig *Config
	configMu     sync.RWMutex
	configPath   string
)

// InitGlobalManager 初始化全局配置管理器，将首次从文件加载到内存
func InitGlobalManager(path string) error {
	configPath = path
	return ReloadFromFile()
}

// ReloadFromFile 从磁盘重新加载最新配置到内存，通常在任何更新配置的动作后主动调用
func ReloadFromFile() error {
	if configPath == "" {
		return nil
	}
	cfg, err := Load(configPath)
	if err != nil {
		return err
	}
	configMu.Lock()
	globalConfig = cfg
	configMu.Unlock()
	logger.Info("配置文件已从磁盘热加载到内存", "path", configPath)
	return nil
}

// GetConfig 获取当前处于内存中的全局配置。为保障一致性，不可在外部直接修改返回值。
func GetConfig() *Config {
	configMu.RLock()
	defer configMu.RUnlock()
	if globalConfig == nil {
		return &Config{}
	}
	return globalConfig
}

// GetConfigPath 返回当前全局配置文件路径，供需要直接读写配置文件的场景使用
// （如设备恢复后回写发现到的物理路径）。
func GetConfigPath() string {
	configMu.RLock()
	defer configMu.RUnlock()
	return configPath
}

// ListDevices 快捷获取内存中的设备列表，替代原 ListDevicesFromFile 造成的高频 IO
func ListDevices() []DeviceConfig {
	return GetConfig().Devices
}

// GetDeviceByID 快捷获取内存中指定 ID 的设备
func GetDeviceByID(id string) (*DeviceConfig, error) {
	devices := ListDevices()
	for i := range devices {
		if devices[i].ID == id {
			return &devices[i], nil
		}
	}
	return nil, nil // not found
}
