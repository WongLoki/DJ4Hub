//go:build darwin && cgo

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const activationWaitTimeout = 40 * time.Second

type networkService struct {
	name     string
	port     string
	device   string
	disabled bool
}

type hardwarePort struct {
	name   string
	device string
}

func activateDJINetwork(output io.Writer) (string, error) {
	if output == nil {
		output = io.Discard
	}
	present, _ := moduleUSBState()
	if !present {
		return "", errors.New("未检测到大疆第一代 4G 模块（USB 2ca3:4006）")
	}
	fmt.Fprintln(output, "已检测到大疆第一代 4G 模块（USB 2ca3:4006）")

	cleanupResult := removeOrphanedModemServices(output)
	if device, address := activeModemLink(); device != "" {
		message := fmt.Sprintf("4G 网卡已可用：%s，IP %s", device, address)
		if cleanupResult != "" {
			message += "；" + cleanupResult
		}
		return message, nil
	}

	channel, err := openModemChannel()
	if err != nil {
		return "", fmt.Errorf("打开模块控制通道失败: %w", err)
	}
	defer channel.Close()

	reply, err := channel.Send(`AT+QCFG="usbnet"`, 3*time.Second)
	if err != nil {
		return "", fmt.Errorf("读取 USB 上网模式失败: %w", err)
	}
	mode := usbnetMode(reply)
	if mode != "1" {
		fmt.Fprintf(output, "当前 usbnet=%s，正在切换到 ECM 模式…\n", modeLabel(mode))
		reply, err = channel.Send(`AT+QCFG="usbnet",1`, 5*time.Second)
		if err != nil || !responseOK(reply) {
			if err != nil {
				return "", fmt.Errorf("写入 ECM 模式失败: %w", err)
			}
			return "", fmt.Errorf("模块拒绝 ECM 模式设置: %q", reply)
		}
	} else {
		fmt.Fprintln(output, "模块已保存 ECM 上网模式。")
	}

	fmt.Fprintln(output, "正在重启模块，让 macOS 重新识别网卡…")
	reply, err = channel.Send("AT+CFUN=1,1", 4*time.Second)
	if err != nil && !disconnectDuringRestart(err) {
		return "", fmt.Errorf("重启模块失败: %w", err)
	}
	if err == nil && !responseOK(reply) {
		return "", fmt.Errorf("模块拒绝重启指令: %q", reply)
	}
	channel.Close()

	deadline := time.Now().Add(activationWaitTimeout)
	for time.Now().Before(deadline) {
		if device, address := activeModemLink(); device != "" {
			removeOrphanedModemServices(output)
			return fmt.Sprintf("连接完成：%s，IP %s", device, address), nil
		}
		time.Sleep(time.Second)
	}
	return "", fmt.Errorf("模块已经重启，但 %s 内没有获得 ECM 网卡地址", activationWaitTimeout)
}

func usbnetMode(reply string) string {
	compact := strings.ReplaceAll(reply, " ", "")
	needle := `+qcfg:"usbnet",`
	start := strings.Index(strings.ToLower(compact), needle)
	if start < 0 {
		return ""
	}
	tail := compact[start+len(needle):]
	for index, char := range tail {
		if char < '0' || char > '9' {
			return tail[:index]
		}
	}
	return tail
}

func modeLabel(mode string) string {
	if mode == "" {
		return "未知"
	}
	return mode
}

func disconnectDuringRestart(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToUpper(err.Error())
	return strings.Contains(message, "NO_DEVICE") || strings.Contains(message, "DISCONNECT")
}

func activeModemLink() (string, string) {
	output, err := exec.Command("networksetup", "-listallhardwareports").Output()
	if err != nil {
		return "", ""
	}
	for _, port := range parseHardwarePorts(string(output)) {
		if !modemIdentity(port.name) || port.device == "" {
			continue
		}
		addressBytes, err := exec.Command("ipconfig", "getifaddr", port.device).Output()
		address := strings.TrimSpace(string(addressBytes))
		if err != nil || !strings.HasPrefix(address, "192.168.225.") {
			continue
		}
		state, err := exec.Command("ifconfig", port.device).Output()
		if err == nil && strings.Contains(string(state), "status: active") {
			return port.device, address
		}
	}
	return "", ""
}

func removeOrphanedModemServices(output io.Writer) string {
	serviceBytes, serviceErr := exec.Command("networksetup", "-listnetworkserviceorder").Output()
	portBytes, portErr := exec.Command("networksetup", "-listallhardwareports").Output()
	if serviceErr != nil || portErr != nil {
		fmt.Fprintln(output, "警告：无法读取 macOS 网络服务配置。")
		return ""
	}
	present := make(map[string]bool)
	for _, port := range parseHardwarePorts(string(portBytes)) {
		if port.device != "" {
			present[port.device] = true
		}
	}
	var candidates []networkService
	for _, service := range parseNetworkServices(string(serviceBytes)) {
		if !service.disabled && modemIdentity(service.name+" "+service.port) && service.device != "" && !present[service.device] {
			candidates = append(candidates, service)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].name < candidates[j].name })

	var completed []string
	for _, service := range candidates {
		_, removeErr := exec.Command("networksetup", "-removenetworkservice", service.name).CombinedOutput()
		if removeErr == nil {
			fmt.Fprintf(output, "已移除残留网络服务：%s（%s）\n", service.name, service.device)
			completed = append(completed, "已清理 "+service.name)
			continue
		}
		if !service.disabled {
			if _, disableErr := exec.Command("networksetup", "-setnetworkserviceenabled", service.name, "off").CombinedOutput(); disableErr == nil {
				fmt.Fprintf(output, "已禁用无法移除的残留服务：%s（%s）\n", service.name, service.device)
				completed = append(completed, "已禁用 "+service.name)
				continue
			}
		}
		fmt.Fprintf(output, "警告：无法处理残留网络服务 %s。\n", service.name)
	}
	return strings.Join(completed, "、")
}

func parseHardwarePorts(value string) []hardwarePort {
	var result []hardwarePort
	var current *hardwarePort
	scanner := bufio.NewScanner(strings.NewReader(value))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "Hardware Port:"):
			result = append(result, hardwarePort{name: strings.TrimSpace(strings.TrimPrefix(line, "Hardware Port:"))})
			current = &result[len(result)-1]
		case current != nil && strings.HasPrefix(line, "Device:"):
			current.device = strings.TrimSpace(strings.TrimPrefix(line, "Device:"))
		}
	}
	return result
}

func parseNetworkServices(value string) []networkService {
	var result []networkService
	var current *networkService
	scanner := bufio.NewScanner(strings.NewReader(value))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "(") && !strings.HasPrefix(line, "(Hardware Port:") {
			closeIndex := strings.Index(line, ")")
			if closeIndex > 0 {
				prefix := line[1:closeIndex]
				name := strings.TrimSpace(line[closeIndex+1:])
				if name != "" {
					result = append(result, networkService{name: name, disabled: prefix == "*"})
					current = &result[len(result)-1]
				}
			}
			continue
		}
		if current == nil || !strings.HasPrefix(line, "(Hardware Port:") {
			continue
		}
		detail := strings.TrimSuffix(strings.TrimPrefix(line, "(Hardware Port:"), ")")
		separator := strings.LastIndex(detail, ", Device:")
		if separator >= 0 {
			current.port = strings.TrimSpace(detail[:separator])
			current.device = strings.TrimSpace(detail[separator+len(", Device:"):])
		}
	}
	return result
}

func modemIdentity(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, "baiwang") ||
		strings.Contains(value, "eg25") ||
		strings.Contains(value, "qdc507")
}
