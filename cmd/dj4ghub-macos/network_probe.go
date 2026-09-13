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

const (
	cellularProbeTimeout = 10 * time.Second
)

var (
	cellularDomainProbeTargets = []string{
		"https://www.baidu.com/",
		"https://www.google.com/generate_204",
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
	if target, err = probeAnyTarget(ctx, interfaceName, sourceIPv4, cellularDomainProbeTargets); err == nil {
		return target, true, nil
	}
	domainErr := err
	if target, err = probeAnyTarget(ctx, interfaceName, sourceIPv4, cellularIPProbeTargets); err == nil {
		return target, false, nil
	}
	return "", false, errors.Join(domainErr, err)
}

func probeAnyTarget(ctx context.Context, interfaceName string, sourceIPv4 string, targets []string) (string, error) {
	if len(targets) == 0 {
		return "", errors.New("没有配置公网检测地址")
	}
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type probeResult struct {
		target string
		err    error
	}
	results := make(chan probeResult, len(targets))
	for _, target := range targets {
		go func(target string) {
			results <- probeResult{
				target: target,
				err:    probeHTTPFromInterface(probeCtx, interfaceName, sourceIPv4, target),
			}
		}(target)
	}
	probeErrors := make([]error, 0, len(targets))
	for range targets {
		result := <-results
		if result.err == nil {
			cancel()
			return result.target, nil
		}
		probeErrors = append(probeErrors, fmt.Errorf("%s: %w", result.target, result.err))
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
