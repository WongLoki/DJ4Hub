package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const cellularProbeTimeout = 10 * time.Second

var (
	cellularDomainProbeTargets = []string{
		"http://captive.apple.com/hotspot-detect.html",
		"http://www.msftconnecttest.com/connecttest.txt",
	}
	cellularIPProbeTargets = []string{
		"http://1.1.1.1/cdn-cgi/trace",
	}
)

// probeCellularInternet verifies that traffic can leave through the selected
// USB interface. A hostname probe also checks the path needed by browsers. If
// only the direct-IP probe succeeds, the cellular path is up but name-based
// browsing is still considered unavailable.
func probeCellularInternet(ctx context.Context, interfaceName string, sourceIPv4 string) (target string, dnsOK bool, err error) {
	if target, err = probeFirstTarget(ctx, interfaceName, sourceIPv4, cellularDomainProbeTargets); err == nil {
		return target, true, nil
	}
	domainErr := err
	if target, err = probeFirstTarget(ctx, interfaceName, sourceIPv4, cellularIPProbeTargets); err == nil {
		return target, false, nil
	}
	return "", false, errors.Join(domainErr, err)
}

func probeFirstTarget(ctx context.Context, interfaceName string, sourceIPv4 string, targets []string) (string, error) {
	var probeErrors []error
	for _, target := range targets {
		if err := probeHTTPFromInterface(ctx, interfaceName, sourceIPv4, target); err == nil {
			return target, nil
		} else {
			probeErrors = append(probeErrors, fmt.Errorf("%s: %w", target, err))
		}
	}
	if len(probeErrors) == 0 {
		return "", errors.New("没有配置公网检测地址")
	}
	return "", errors.Join(probeErrors...)
}

func probeHTTPFromInterface(ctx context.Context, interfaceName string, sourceIPv4 string, target string) error {
	interfaceInfo, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return fmt.Errorf("读取网卡 %s: %w", interfaceName, err)
	}
	sourceIP := net.ParseIP(sourceIPv4)
	if sourceIP == nil || sourceIP.To4() == nil {
		return fmt.Errorf("无效的 IPv4 地址 %q", sourceIPv4)
	}

	dialer := &net.Dialer{
		Timeout:   4 * time.Second,
		LocalAddr: &net.TCPAddr{IP: sourceIP},
		Control: func(_, _ string, connection syscall.RawConn) error {
			var socketErr error
			if err := connection.Control(func(fd uintptr) {
				socketErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_BOUND_IF, interfaceInfo.Index)
			}); err != nil {
				return err
			}
			return socketErr
		},
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 4 * time.Second,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "DJ4Hub-Network-Check/1.0")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if err := response.Body.Close(); err != nil {
		return fmt.Errorf("关闭公网检测响应: %w", err)
	}
	return nil
}
