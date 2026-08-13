//go:build !darwin || !cgo

package main

import (
	"errors"
	"io"
)

func activateDJINetwork(io.Writer) error {
	return errors.New("上网激活仅支持启用 CGO 的 macOS 构建")
}
