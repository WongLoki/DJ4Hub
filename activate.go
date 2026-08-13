//go:build darwin && cgo

package main

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const activationWaitTimeout = 40 * time.Second

type usbInterfaceStatus struct {
	Number   int
	Class    int
	Subclass int
	Protocol int
}

type usbDeviceStatus struct {
	Product    string
	Vendor     string
	VendorID   string
	ProductID  string
	Interfaces []usbInterfaceStatus
}

type macNetworkInterface struct {
	Name   string
	Status string
	IPv4   string
}

type macNetworkService struct {
	Name         string
	HardwarePort string
	Device       string
	Disabled     bool
}

func activateDJINetwork(out io.Writer) (string, error) {
	if out == nil {
		out = io.Discard
	}
	usbDevice := discoverDJIUSBDevice()
	if usbDevice == nil {
		return "", errors.New("未检测到大疆一代 4G 模块（USB 2ca3:4006）")
	}
	fmt.Fprintf(out, "已检测到模块：%s %s（%s:%s）\n",
		usbDevice.Vendor, usbDevice.Product, usbDevice.VendorID, usbDevice.ProductID)

	cleanupMessage := cleanStaleDJINetworkServices(out, usbDevice.Product)
	if interfaceName, address := currentDJINetworkInterface(); interfaceName != "" {
		result := fmt.Sprintf("4G 网卡已可用：%s，IP %s", interfaceName, address)
		if cleanupMessage != "" {
			result += "；" + cleanupMessage
		}
		return result, nil
	}

	device, err := openDJIUSBAT()
	if err != nil {
		return "", fmt.Errorf("打开 USB AT 接口失败: %w", err)
	}
	defer device.Close()

	response, err := device.Command(`AT+QCFG="usbnet"`, 3*time.Second)
	if err != nil {
		return "", fmt.Errorf("读取上网模式失败: %w", err)
	}
	mode := parseUSBNetMode(response)
	if mode != "1" {
		fmt.Fprintf(out, "当前 usbnet=%s，正在写入 ECM 上网模式 1…\n", displayMode(mode))
		response, err = device.Command(`AT+QCFG="usbnet",1`, 5*time.Second)
		if err != nil {
			return "", fmt.Errorf("写入上网模式失败: %w", err)
		}
		if !atSucceeded(response) {
			return "", fmt.Errorf("写入上网模式失败，模块返回 %q", response)
		}
	} else {
		fmt.Fprintln(out, "模块已保存 ECM 上网模式 1。")
	}

	fmt.Fprintln(out, "正在软重启模块以触发 macOS 重新枚举网卡…")
	response, err = device.Command("AT+CFUN=1,1", 4*time.Second)
	if err != nil && !expectedRebootDisconnect(err) {
		return "", fmt.Errorf("重启模块失败: %w", err)
	}
	if err == nil && !atSucceeded(response) {
		return "", fmt.Errorf("重启模块失败，模块返回 %q", response)
	}
	device.Close()

	deadline := time.Now().Add(activationWaitTimeout)
	for time.Now().Before(deadline) {
		if interfaceName, address := currentDJINetworkInterface(); interfaceName != "" {
			cleanStaleDJINetworkServices(out, currentDJIProductName())
			return fmt.Sprintf("激活完成：%s，IP %s", interfaceName, address), nil
		}
		time.Sleep(time.Second)
	}
	return "", fmt.Errorf("模块已重启，但 %s 内没有获得 ECM 网卡和 DHCP 地址", activationWaitTimeout)
}

func displayMode(mode string) string {
	if strings.TrimSpace(mode) == "" {
		return "未知"
	}
	return mode
}

func expectedRebootDisconnect(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToUpper(err.Error())
	return strings.Contains(message, "LIBUSB_ERROR_NO_DEVICE") ||
		strings.Contains(message, "DEVICE DISCONNECTED")
}

func parseUSBNetMode(response string) string {
	pattern := regexp.MustCompile(`\+QCFG:\s*"usbnet",(\d+)`)
	match := pattern.FindStringSubmatch(response)
	if len(match) != 2 {
		return ""
	}
	return match[1]
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
		if item.Status == "active" && strings.HasPrefix(item.IPv4, "192.168.225.") {
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

func discoverDJIUSBDevice() *usbDeviceStatus {
	output, err := exec.Command("ioreg", "-r", "-c", "IOUSBHostInterface", "-l", "-w", "0").Output()
	if err != nil {
		return nil
	}
	var device *usbDeviceStatus
	for _, block := range strings.Split(string(output), "\n\n") {
		vendorID, vendorOK := intProperty(block, "idVendor")
		productID, productOK := intProperty(block, "idProduct")
		if !vendorOK || !productOK || vendorID != djiUSBVendorID || productID != djiUSBProductID {
			continue
		}
		if device == nil {
			device = &usbDeviceStatus{
				Product:   stringProperty(block, "USB Product Name"),
				Vendor:    stringProperty(block, "USB Vendor Name"),
				VendorID:  fmt.Sprintf("%04x", vendorID),
				ProductID: fmt.Sprintf("%04x", productID),
			}
			if device.Product == "" {
				device.Product = "DJI 4G Module"
			}
			if device.Vendor == "" {
				device.Vendor = "DJI"
			}
		}
		interfaceNumber, interfaceOK := intProperty(block, "bInterfaceNumber")
		if !interfaceOK {
			continue
		}
		device.Interfaces = append(device.Interfaces, usbInterfaceStatus{
			Number:   interfaceNumber,
			Class:    intPropertyOrZero(block, "bInterfaceClass"),
			Subclass: intPropertyOrZero(block, "bInterfaceSubClass"),
			Protocol: intPropertyOrZero(block, "bInterfaceProtocol"),
		})
	}
	if device != nil {
		sort.Slice(device.Interfaces, func(i, j int) bool {
			return device.Interfaces[i].Number < device.Interfaces[j].Number
		})
	}
	return device
}

func discoverMacNetworkInterfaces() []macNetworkInterface {
	output, err := exec.Command("ifconfig").Output()
	if err != nil {
		return nil
	}
	var interfaces []macNetworkInterface
	for _, block := range splitIfconfigBlocks(string(output)) {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) == 0 {
			continue
		}
		fields := strings.Fields(lines[0])
		if len(fields) == 0 {
			continue
		}
		item := macNetworkInterface{Name: strings.TrimSuffix(fields[0], ":"), Status: "unknown"}
		for _, rawLine := range lines {
			line := strings.TrimSpace(rawLine)
			if strings.HasPrefix(line, "status:") {
				item.Status = strings.TrimSpace(strings.TrimPrefix(line, "status:"))
			}
			if strings.HasPrefix(line, "inet ") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					item.IPv4 = parts[1]
				}
			}
		}
		interfaces = append(interfaces, item)
	}
	return interfaces
}

func splitIfconfigBlocks(output string) []string {
	var blocks []string
	var current []string
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		if line[0] != '\t' && line[0] != ' ' && strings.Contains(line, ":") {
			if len(current) > 0 {
				blocks = append(blocks, strings.Join(current, "\n"))
			}
			current = []string{line}
			continue
		}
		if len(current) > 0 {
			current = append(current, line)
		}
	}
	if len(current) > 0 {
		blocks = append(blocks, strings.Join(current, "\n"))
	}
	return blocks
}

func cleanStaleDJINetworkServices(out io.Writer, currentProduct string) string {
	servicesOutput, err := exec.Command("networksetup", "-listnetworkserviceorder").Output()
	if err != nil {
		fmt.Fprintf(out, "警告：无法读取 macOS 网络服务：%v\n", err)
		return ""
	}
	hardwareOutput, err := exec.Command("networksetup", "-listallhardwareports").Output()
	if err != nil {
		fmt.Fprintf(out, "警告：无法读取 macOS 网卡列表：%v\n", err)
		return ""
	}
	stale := staleDJINetworkServices(
		parseMacNetworkServiceOrder(string(servicesOutput)),
		parseMacHardwareDevices(string(hardwareOutput)),
		currentProduct,
	)
	var cleaned []string
	for _, service := range stale {
		command := exec.Command("networksetup", "-removenetworkservice", service.Name)
		if response, removeErr := command.CombinedOutput(); removeErr != nil {
			disable := exec.Command("networksetup", "-setnetworkserviceenabled", service.Name, "off")
			if _, disableErr := disable.CombinedOutput(); disableErr == nil {
				fmt.Fprintf(out, "已禁用残留网络服务：%s（%s）\n", service.Name, service.Device)
				cleaned = append(cleaned, "已禁用残留 "+service.Name)
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
		cleaned = append(cleaned, "已清理残留 "+service.Name)
	}
	return strings.Join(cleaned, "、")
}

func parseMacNetworkServiceOrder(output string) []macNetworkService {
	servicePattern := regexp.MustCompile(`^\((\d+|\*)\)\s+(.+)$`)
	detailPattern := regexp.MustCompile(`^\(Hardware Port:\s*(.*),\s*Device:\s*([^)]*)\)$`)
	var services []macNetworkService
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if match := servicePattern.FindStringSubmatch(line); len(match) == 3 {
			services = append(services, macNetworkService{
				Name:     strings.TrimSpace(match[2]),
				Disabled: match[1] == "*",
			})
			continue
		}
		if len(services) == 0 {
			continue
		}
		if match := detailPattern.FindStringSubmatch(line); len(match) == 3 {
			services[len(services)-1].HardwarePort = strings.TrimSpace(match[1])
			services[len(services)-1].Device = strings.TrimSpace(match[2])
		}
	}
	return services
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

func isDJINetworkService(service macNetworkService) bool {
	identity := strings.ToLower(service.Name + " " + service.HardwarePort)
	return strings.Contains(identity, "baiwang") ||
		strings.Contains(identity, "eg25") ||
		strings.Contains(identity, "qdc507")
}

func networkIdentityMatchesProduct(service macNetworkService, product string) bool {
	identity := compactNetworkIdentity(service.Name + service.HardwarePort)
	if strings.Contains(identity, "eg25") || strings.Contains(identity, "qdc507") {
		return true
	}
	product = compactNetworkIdentity(product)
	return product != "" && strings.Contains(identity, product)
}

func compactNetworkIdentity(value string) string {
	value = strings.ToLower(value)
	var result strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			result.WriteRune(char)
		}
	}
	return result.String()
}

func intPropertyOrZero(block, name string) int {
	value, _ := intProperty(block, name)
	return value
}

func intProperty(block, name string) (int, bool) {
	pattern := regexp.MustCompile(`"` + regexp.QuoteMeta(name) + `"\s*=\s*(\d+)`)
	match := pattern.FindStringSubmatch(block)
	if len(match) != 2 {
		return 0, false
	}
	value, err := strconv.Atoi(match[1])
	return value, err == nil
}

func stringProperty(block, name string) string {
	pattern := regexp.MustCompile(`"` + regexp.QuoteMeta(name) + `"\s*=\s*"([^"]*)"`)
	match := pattern.FindStringSubmatch(block)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}
