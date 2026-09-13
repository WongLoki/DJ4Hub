package main

import (
	"errors"
	"os"
	"path/filepath"
)

// Import approved runtime snapshots without executing them or replacing an installation.
func installAudioRuntime(source string) error {
	files, err := audioRuntimeFiles(source)
	if err != nil {
		return err
	}
	destination, _, err := moduleAudioPaths()
	if err != nil {
		return err
	}
	if _, err = audioRuntimeFiles(destination); err == nil {
		return nil
	}
	if _, err = os.Lstat(destination); err == nil {
		return errors.New("目标目录已存在但校验未通过；为保护原文件，请先将该目录改名备份再导入")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".audio-import-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	for name := range moduleAudioHashes {
		if err = os.WriteFile(filepath.Join(stage, name), files[name], 0600); err != nil {
			return err
		}
	}
	return os.Rename(stage, destination)
}
