//go:build darwin && cgo

package main

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const activationWaitTimeout = 40 * time.Second

func activateDJINetwork(out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	usbDevice := discoverDJIUSBDevice()
	if usbDevice == nil {
		return errors.New("未检测到受支持的 DJI 4G 模块（USB 2ca3:4006）")
	}
	fmt.Fprintf(out, "已检测到模块：%s %s（%s:%s）\n",
		usbDevice.Vendor, usbDevice.Product, usbDevice.VendorID, usbDevice.ProductID)

	cleanStaleDJINetworkServices(out, usbDevice.Product)
	if interfaceName, address := currentDJINetworkInterface(); interfaceName != "" {
		fmt.Fprintf(out, "上网网卡已经可用：%s，IP %s\n", interfaceName, address)
		return nil
	}

	device, err := openDJIUSBAT()
	if err != nil {
		return fmt.Errorf("打开 USB AT 接口失败: %w", err)
	}
	defer device.Close()

	response, err := device.Command(`AT+QCFG="usbnet"`, 3*time.Second)
	if err != nil {
		return fmt.Errorf("读取上网模式失败: %w", err)
	}
	mode := parseUSBNetMode(response)
	if mode != "1" {
		fmt.Fprintf(out, "当前 usbnet=%s，正在写入 ECM 上网模式 1...\n", displayActivationMode(mode))
		response, err = device.Command(`AT+QCFG="usbnet",1`, 5*time.Second)
		if err != nil {
			return fmt.Errorf("写入上网模式失败: %w", err)
		}
		if !atProbeSucceeded(response) {
			return fmt.Errorf("写入上网模式失败，模块返回 %q", response)
		}
	} else {
		fmt.Fprintln(out, "模块已保存 ECM 上网模式 1。")
	}

	fmt.Fprintln(out, "正在软重启模块以触发 macOS 重新枚举网卡...")
	response, err = device.Command("AT+CFUN=1,1", 4*time.Second)
	if err != nil && !expectedActivationDisconnect(err) {
		return fmt.Errorf("重启模块失败: %w", err)
	}
	if err == nil && !atProbeSucceeded(response) {
		return fmt.Errorf("重启模块失败，模块返回 %q", response)
	}
	device.Close()

	deadline := time.Now().Add(activationWaitTimeout)
	for time.Now().Before(deadline) {
		if interfaceName, address := currentDJINetworkInterface(); interfaceName != "" {
			cleanStaleDJINetworkServices(out, currentDJIProductName())
			fmt.Fprintf(out, "激活完成：%s，IP %s\n", interfaceName, address)
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("模块已重启，但 %s 内没有获得可用的 ECM 网卡和 DHCP 地址", activationWaitTimeout)
}

func displayActivationMode(mode string) string {
	if strings.TrimSpace(mode) == "" {
		return "未知"
	}
	return mode
}

func expectedActivationDisconnect(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToUpper(err.Error())
	return strings.Contains(message, "LIBUSB_ERROR_NO_DEVICE") ||
		strings.Contains(message, "DEVICE DISCONNECTED")
}

func currentDJIProductName() string {
	if device := discoverDJIUSBDevice(); device != nil {
		return device.Product
	}
	return ""
}

func currentDJINetworkInterface() (string, string) {
	device := discoverDJIUSBDevice()
	if device == nil || !hasECMUSBInterfaces(device.Interfaces) {
		return "", ""
	}
	for _, item := range discoverMacNetworkInterfaces() {
		if item.Kind != "ethernet" || item.Status != "active" || item.IPv4 == "" {
			continue
		}
		if strings.HasPrefix(item.IPv4, "192.168.225.") {
			return item.Name, item.IPv4
		}
	}
	return "", ""
}

func hasECMUSBInterfaces(interfaces []usbInterfaceStatus) bool {
	var control, data bool
	for _, item := range interfaces {
		switch item.Class {
		case 2:
			control = true
		case 10:
			data = true
		}
	}
	return control && data
}

func cleanStaleDJINetworkServices(out io.Writer, currentProduct string) {
	servicesOutput, err := exec.Command("networksetup", "-listnetworkserviceorder").Output()
	if err != nil {
		fmt.Fprintf(out, "警告：无法读取 macOS 网络服务：%v\n", err)
		return
	}
	hardwareOutput, err := exec.Command("networksetup", "-listallhardwareports").Output()
	if err != nil {
		fmt.Fprintf(out, "警告：无法读取 macOS 网卡列表：%v\n", err)
		return
	}
	stale := staleDJINetworkServices(
		parseMacNetworkServiceOrder(string(servicesOutput)),
		parseMacHardwareDevices(string(hardwareOutput)),
		currentProduct,
	)
	for _, service := range stale {
		command := exec.Command("networksetup", "-removenetworkservice", service.Name)
		if response, removeErr := command.CombinedOutput(); removeErr != nil {
			disable := exec.Command("networksetup", "-setnetworkserviceenabled", service.Name, "off")
			if _, disableErr := disable.CombinedOutput(); disableErr == nil {
				fmt.Fprintf(out, "macOS 不允许移除最后一个同端口服务，已禁用残留网络服务：%s（%s）\n", service.Name, service.Device)
				continue
			}
			detail := strings.TrimSpace(string(response))
			if detail == "" {
				detail = removeErr.Error()
			}
			fmt.Fprintf(out, "警告：清理或禁用残留网络服务 %q 失败：%s\n", service.Name, detail)
			continue
		}
		fmt.Fprintf(out, "已清理残留网络服务：%s（%s）\n", service.Name, service.Device)
	}
}

func parseMacHardwareDevices(output string) map[string]bool {
	devices := make(map[string]bool)
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "Device:") {
			continue
		}
		device := strings.TrimSpace(strings.TrimPrefix(line, "Device:"))
		if device != "" {
			devices[device] = true
		}
	}
	return devices
}

func staleDJINetworkServices(services []macNetworkService, presentDevices map[string]bool, currentProduct string) []macNetworkService {
	var stale []macNetworkService
	for _, service := range services {
		if service.Disabled || !isDJINetworkService(service) || service.Device == "" || presentDevices[service.Device] {
			continue
		}
		if networkIdentityMatchesProduct(service, currentProduct) {
			continue
		}
		stale = append(stale, service)
	}
	sort.Slice(stale, func(i, j int) bool { return stale[i].Name < stale[j].Name })
	return stale
}
