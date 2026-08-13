package main

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

type usbATESIMChannel struct {
	command func(string, time.Duration) (string, error)
	channel byte
	mu      sync.Mutex
}

func newUSBATESIMChannel(command func(string, time.Duration) (string, error)) *usbATESIMChannel {
	return &usbATESIMChannel{command: command}
}

func (c *usbATESIMChannel) CurrentChannel() byte {
	return c.channel
}

func (c *usbATESIMChannel) Connect() error {
	return nil
}

func (c *usbATESIMChannel) Disconnect() error {
	return nil
}

func (c *usbATESIMChannel) OpenLogicalChannel(aid []byte) (byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	aidHex := strings.ToUpper(hex.EncodeToString(aid))
	resp, err := c.command(fmt.Sprintf(`AT+CCHO="%s"`, aidHex), 8*time.Second)
	if err != nil {
		return 0, fmt.Errorf("打开 eUICC logical channel 失败 (AID=%s): %w", aidHex, err)
	}
	channel, ok := parseUSBCCHO(resp)
	if !ok {
		return 0, fmt.Errorf("解析 CCHO 响应失败: %s", resp)
	}
	c.channel = byte(channel)
	return c.channel, nil
}

func (c *usbATESIMChannel) Transmit(command []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.channel == 0 {
		return nil, fmt.Errorf("eUICC logical channel 尚未打开")
	}
	cmdHex := strings.ToUpper(hex.EncodeToString(command))
	resp, err := c.command(fmt.Sprintf(`AT+CGLA=%d,%d,"%s"`, c.channel, len(cmdHex), cmdHex), 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("APDU 透传失败: %w", err)
	}
	respHex, ok := parseUSBCGLA(resp)
	if !ok {
		return nil, fmt.Errorf("解析 CGLA 响应失败: %s", resp)
	}
	out, err := hex.DecodeString(respHex)
	if err != nil {
		return nil, fmt.Errorf("解析 APDU 响应 hex 失败: %w", err)
	}
	return out, nil
}

func (c *usbATESIMChannel) CloseLogicalChannel(channel byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, err := c.command(fmt.Sprintf("AT+CCHC=%d", channel), 8*time.Second)
	if err != nil {
		return fmt.Errorf("关闭 eUICC logical channel %d 失败: %w", channel, err)
	}
	if c.channel == channel {
		c.channel = 0
	}
	return nil
}

func parseUSBCCHO(resp string) (int, bool) {
	re := regexp.MustCompile(`\+CCHO:\s*(\d+)`)
	match := re.FindStringSubmatch(resp)
	if len(match) != 2 {
		return 0, false
	}
	var channel int
	if _, err := fmt.Sscanf(match[1], "%d", &channel); err != nil {
		return 0, false
	}
	return channel, true
}

func parseUSBCGLA(resp string) (string, bool) {
	re := regexp.MustCompile(`\+CGLA:\s*\d+\s*,\s*"?([0-9A-Fa-f]+)"?`)
	match := re.FindStringSubmatch(resp)
	if len(match) != 2 {
		return "", false
	}
	return strings.ToUpper(match[1]), true
}
