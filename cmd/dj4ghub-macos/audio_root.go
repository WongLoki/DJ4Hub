package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Only requests adbd's supported root mode; never unlocks or flashes a device.
func ensureAudioRoot(ctx context.Context, uid func() (string, error), request func() error) error {
	value, err := uid()
	if err != nil {
		return fmt.Errorf("无法检查 ADB 权限，请确认设备已授权：%w", err)
	}
	if strings.TrimSpace(value) == "0" {
		return nil
	}
	if err := request(); err != nil {
		return fmt.Errorf("自动 adb root 失败：%w", err)
	}
	for i := 0; i < 12; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		value, err = uid()
		if err == nil && strings.TrimSpace(value) == "0" {
			return nil
		}
	}
	return fmt.Errorf("adb root 后未确认同一设备的 root 权限；请检查固件支持，未上传驱动")
}
