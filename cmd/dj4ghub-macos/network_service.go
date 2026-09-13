package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	networkServiceDHCPWait = 15 * time.Second
)

type macNetworkService struct {
	Name             string `json:"name"`
	HardwarePort     string `json:"hardware_port"`
	Device           string `json:"device"`
	Disabled         bool   `json:"disabled"`
	InterfacePresent bool   `json:"interface_present"`
	InterfaceStatus  string `json:"interface_status,omitempty"`
	IPv4             string `json:"ipv4,omitempty"`
}

type networkServiceRepairResult struct {
	Ready          bool               `json:"ready"`
	Summary        string             `json:"summary"`
	NetworkService *macNetworkService `json:"network_service,omitempty"`
}

func discoverMacNetworkServices() []macNetworkService {
	output, err := exec.Command("networksetup", "-listnetworkserviceorder").Output()
	if err != nil {
		return nil
	}
	return parseMacNetworkServiceOrder(string(output))
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

func currentDJINetworkService(services []macNetworkService, interfaces []macNetInterface, currentProduct string) *macNetworkService {
	interfaceByName := make(map[string]macNetInterface, len(interfaces))
	for _, item := range interfaces {
		interfaceByName[item.Name] = item
	}

	type candidate struct {
		service macNetworkService
		score   int
	}
	var candidates []candidate
	for _, service := range services {
		if !isDJINetworkService(service) || service.Device == "" {
			continue
		}
		score := 0
		if item, ok := interfaceByName[service.Device]; ok {
			service.InterfacePresent = true
			service.InterfaceStatus = item.Status
			service.IPv4 = item.IPv4
			score += 100
		}
		if networkIdentityMatchesProduct(service, currentProduct) {
			score += 20
		}
		if !service.Disabled {
			score++
		}
		candidates = append(candidates, candidate{service: service, score: score})
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i int, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].service.Name < candidates[j].service.Name
		}
		return candidates[i].score > candidates[j].score
	})
	selected := candidates[0].service
	return &selected
}

func networkServiceReady(service *macNetworkService) bool {
	return service != nil && !service.Disabled && service.InterfacePresent && service.InterfaceStatus == "active" && service.IPv4 != ""
}

func enableMacNetworkService(serviceName string) error {
	if strings.TrimSpace(serviceName) == "" {
		return errors.New("network service name is empty")
	}
	const script = `on run argv
set serviceName to item 1 of argv
do shell script "/usr/sbin/networksetup -setnetworkserviceenabled " & quoted form of serviceName & " on && /usr/sbin/networksetup -setdhcp " & quoted form of serviceName with administrator privileges
end run`
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "osascript", "-e", script, "--", serviceName).CombinedOutput()
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		detail = err.Error()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("等待 macOS 管理员授权超时")
	}
	return fmt.Errorf("启用 macOS 网络服务失败: %s", detail)
}

func waitForMacNetworkService(serviceName string, currentProduct string, timeout time.Duration) *macNetworkService {
	deadline := time.Now().Add(timeout)
	for {
		service := currentDJINetworkService(discoverMacNetworkServices(), discoverMacNetworkInterfaces(), currentProduct)
		if service != nil && service.Name == serviceName && networkServiceReady(service) {
			return service
		}
		if time.Now().After(deadline) {
			return service
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func isDJINetworkService(service macNetworkService) bool {
	identity := strings.ToLower(service.Name + " " + service.HardwarePort)
	return strings.Contains(identity, "baiwang") ||
		strings.Contains(identity, "eg25") ||
		strings.Contains(identity, "qdc507")
}

func networkIdentityMatchesProduct(service macNetworkService, product string) bool {
	identity := compactNetworkIdentity(service.Name + service.HardwarePort)
	// ECM enumeration uses the EG25/QDC507 identity even when the initial
	// vendor-specific enumeration reports only "Baiwang". Preserve this service
	// while the module is between enumerations; the old Baiwang service can be
	// removed after the ECM device appears.
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
