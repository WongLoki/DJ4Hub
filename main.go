//go:build darwin && cgo

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

func main() {
	var noNotify bool
	flag.BoolVar(&noNotify, "no-notify", false, "do not show macOS notifications")
	flag.Parse()

	logFile, err := openLogFile()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer logFile.Close()
	output := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(output)
	log.SetFlags(log.Ldate | log.Ltime)

	if !noNotify {
		showNotification("DJOneActivator", "正在检查大疆 4G 模块和 macOS 网卡…")
	}
	result, err := activateDJINetwork(output)
	if err != nil {
		log.Printf("激活失败：%v", err)
		if !noNotify {
			showNotification("DJOneActivator 激活失败", err.Error())
		}
		os.Exit(1)
	}
	log.Printf("%s", result)
	if !noNotify {
		showNotification("DJOneActivator", result)
	}
}

func openLogFile() (*os.File, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("读取用户目录失败: %w", err)
	}
	directory := filepath.Join(home, "Library", "Logs", "DJOneActivator")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(directory, "activator.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("打开日志失败: %w", err)
	}
	return file, nil
}
