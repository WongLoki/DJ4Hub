//go:build darwin && cgo

package main

/*
#cgo pkg-config: libusb-1.0
#include <libusb.h>
#include <stdlib.h>

typedef struct {
    libusb_context *context;
    libusb_device_handle *handle;
    int interface_number;
    unsigned char endpoint_in;
    unsigned char endpoint_out;
} modem_pipe;

static int dji_topology(int *has_control, int *has_data) {
    libusb_context *context = NULL;
    libusb_device **devices = NULL;
    int found = 0;
    *has_control = 0;
    *has_data = 0;
    if (libusb_init(&context) != 0) return 0;
    ssize_t count = libusb_get_device_list(context, &devices);
    for (ssize_t i = 0; i < count; i++) {
        struct libusb_device_descriptor descriptor;
        if (libusb_get_device_descriptor(devices[i], &descriptor) != 0) continue;
        if (descriptor.idVendor != 0x2ca3 || descriptor.idProduct != 0x4006) continue;
        found = 1;
        struct libusb_config_descriptor *configuration = NULL;
        if (libusb_get_active_config_descriptor(devices[i], &configuration) == 0) {
            for (int j = 0; j < configuration->bNumInterfaces; j++) {
                const struct libusb_interface *interface = &configuration->interface[j];
                for (int k = 0; k < interface->num_altsetting; k++) {
                    unsigned char class_code = interface->altsetting[k].bInterfaceClass;
                    if (class_code == LIBUSB_CLASS_COMM) *has_control = 1;
                    if (class_code == LIBUSB_CLASS_DATA) *has_data = 1;
                }
            }
            libusb_free_config_descriptor(configuration);
        }
        break;
    }
    if (devices != NULL) libusb_free_device_list(devices, 1);
    libusb_exit(context);
    return found;
}

// Each call opens one bulk endpoint pair by ordinal. Go probes the pair with AT
// and keeps the first responsive channel.
static modem_pipe *dji_open_pipe(int ordinal, int *status) {
    modem_pipe *pipe = calloc(1, sizeof(modem_pipe));
    libusb_device **devices = NULL;
    struct libusb_config_descriptor *configuration = NULL;
    int seen = 0;
    *status = LIBUSB_ERROR_NOT_FOUND;
    if (pipe == NULL) {
        *status = LIBUSB_ERROR_NO_MEM;
        return NULL;
    }
    *status = libusb_init(&pipe->context);
    if (*status != 0) goto fail;
    ssize_t count = libusb_get_device_list(pipe->context, &devices);
    for (ssize_t i = 0; i < count; i++) {
        struct libusb_device_descriptor descriptor;
        if (libusb_get_device_descriptor(devices[i], &descriptor) != 0) continue;
        if (descriptor.idVendor != 0x2ca3 || descriptor.idProduct != 0x4006) continue;
        *status = libusb_open(devices[i], &pipe->handle);
        if (*status != 0) goto fail;
        *status = libusb_get_active_config_descriptor(devices[i], &configuration);
        if (*status != 0) goto fail;
        for (int j = 0; j < configuration->bNumInterfaces; j++) {
            const struct libusb_interface *interface = &configuration->interface[j];
            for (int k = 0; k < interface->num_altsetting; k++) {
                const struct libusb_interface_descriptor *alternate = &interface->altsetting[k];
                unsigned char endpoint_in = 0;
                unsigned char endpoint_out = 0;
                for (int e = 0; e < alternate->bNumEndpoints; e++) {
                    const struct libusb_endpoint_descriptor *endpoint = &alternate->endpoint[e];
                    if ((endpoint->bmAttributes & LIBUSB_TRANSFER_TYPE_MASK) != LIBUSB_TRANSFER_TYPE_BULK) continue;
                    if ((endpoint->bEndpointAddress & LIBUSB_ENDPOINT_DIR_MASK) == LIBUSB_ENDPOINT_IN)
                        endpoint_in = endpoint->bEndpointAddress;
                    else
                        endpoint_out = endpoint->bEndpointAddress;
                }
                if (endpoint_in == 0 || endpoint_out == 0) continue;
                if (seen++ != ordinal) continue;
                pipe->interface_number = alternate->bInterfaceNumber;
                pipe->endpoint_in = endpoint_in;
                pipe->endpoint_out = endpoint_out;
                *status = libusb_claim_interface(pipe->handle, pipe->interface_number);
                if (*status != 0) goto fail;
                libusb_free_config_descriptor(configuration);
                libusb_free_device_list(devices, 1);
                return pipe;
            }
        }
        *status = LIBUSB_ERROR_NOT_FOUND;
        break;
    }
fail:
    if (configuration != NULL) libusb_free_config_descriptor(configuration);
    if (devices != NULL) libusb_free_device_list(devices, 1);
    if (pipe->handle != NULL) libusb_close(pipe->handle);
    if (pipe->context != NULL) libusb_exit(pipe->context);
    free(pipe);
    return NULL;
}

static void dji_close_pipe(modem_pipe *pipe) {
    if (pipe == NULL) return;
    if (pipe->handle != NULL) {
        libusb_release_interface(pipe->handle, pipe->interface_number);
        libusb_close(pipe->handle);
    }
    if (pipe->context != NULL) libusb_exit(pipe->context);
    free(pipe);
}

static int dji_pipe_write(modem_pipe *pipe, unsigned char *bytes, int length,
                          unsigned int timeout_ms, int *transferred) {
    return libusb_bulk_transfer(pipe->handle, pipe->endpoint_out, bytes, length,
                                transferred, timeout_ms);
}

static int dji_pipe_read(modem_pipe *pipe, unsigned char *bytes, int capacity,
                         unsigned int timeout_ms, int *transferred) {
    return libusb_bulk_transfer(pipe->handle, pipe->endpoint_in, bytes, capacity,
                                transferred, timeout_ms);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"
)

type modemChannel struct {
	pipe *C.modem_pipe
	mu   sync.Mutex
}

func moduleUSBState() (present, ecm bool) {
	var control, data C.int
	present = C.dji_topology(&control, &data) != 0
	return present, control != 0 && data != 0
}

func openModemChannel() (*modemChannel, error) {
	var lastError error
	for ordinal := 0; ordinal < 16; ordinal++ {
		var status C.int
		pipe := C.dji_open_pipe(C.int(ordinal), &status)
		if pipe == nil {
			if ordinal == 0 {
				return nil, fmt.Errorf("无法打开 USB 2ca3:4006: %s", libusbStatus(status))
			}
			break
		}
		channel := &modemChannel{pipe: pipe}
		answer, err := channel.Send("AT", 1200*time.Millisecond)
		if err == nil && responseOK(answer) {
			return channel, nil
		}
		if err == nil {
			err = fmt.Errorf("AT 探测返回 %q", answer)
		}
		lastError = err
		channel.Close()
	}
	if lastError != nil {
		return nil, fmt.Errorf("没有找到可用的 USB AT 通道: %w", lastError)
	}
	return nil, errors.New("模块没有暴露可用的 USB bulk AT 通道")
}

func (channel *modemChannel) Close() {
	if channel == nil {
		return
	}
	channel.mu.Lock()
	defer channel.mu.Unlock()
	if channel.pipe != nil {
		C.dji_close_pipe(channel.pipe)
		channel.pipe = nil
	}
}

func (channel *modemChannel) Send(command string, timeout time.Duration) (string, error) {
	command = strings.TrimSpace(command)
	if !strings.HasPrefix(strings.ToUpper(command), "AT") {
		return "", errors.New("无效的 AT 指令")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	channel.mu.Lock()
	defer channel.mu.Unlock()
	if channel.pipe == nil {
		return "", errors.New("USB AT 通道已经关闭")
	}
	channel.discardPendingInput()
	request := []byte(command + "\r")
	if err := channel.write(request, timeout); err != nil {
		return "", err
	}
	deadline := time.Now().Add(timeout)
	var response strings.Builder
	for time.Now().Before(deadline) {
		window := minDuration(700*time.Millisecond, time.Until(deadline))
		chunk, err := channel.read(window)
		if errors.Is(err, errTransferTimeout) {
			continue
		}
		if err != nil {
			return cleanATResponse(response.String()), err
		}
		response.Write(chunk)
		if responseFinished(response.String()) {
			return cleanATResponse(response.String()), nil
		}
	}
	if response.Len() == 0 {
		return "", errors.New("AT 指令等待响应超时")
	}
	return cleanATResponse(response.String()), nil
}

var errTransferTimeout = errors.New("USB transfer timeout")

func (channel *modemChannel) discardPendingInput() {
	for {
		if _, err := channel.read(50 * time.Millisecond); err != nil {
			return
		}
	}
}

func (channel *modemChannel) write(payload []byte, timeout time.Duration) error {
	var transferred C.int
	status := C.dji_pipe_write(
		channel.pipe,
		(*C.uchar)(unsafe.Pointer(&payload[0])),
		C.int(len(payload)),
		C.uint(timeout.Milliseconds()),
		&transferred,
	)
	if status != 0 {
		return fmt.Errorf("USB 写入失败: %s", libusbStatus(status))
	}
	if int(transferred) != len(payload) {
		return fmt.Errorf("USB 写入不完整: %d/%d", transferred, len(payload))
	}
	return nil
}

func (channel *modemChannel) read(timeout time.Duration) ([]byte, error) {
	buffer := make([]byte, 1024)
	var transferred C.int
	status := C.dji_pipe_read(
		channel.pipe,
		(*C.uchar)(unsafe.Pointer(&buffer[0])),
		C.int(len(buffer)),
		C.uint(max(timeout.Milliseconds(), 1)),
		&transferred,
	)
	if status == C.LIBUSB_ERROR_TIMEOUT {
		return nil, errTransferTimeout
	}
	if status != 0 {
		return nil, fmt.Errorf("USB 读取失败: %s", libusbStatus(status))
	}
	return buffer[:int(transferred)], nil
}

func libusbStatus(status C.int) string {
	return C.GoString(C.libusb_error_name(status))
}

func responseFinished(value string) bool {
	upper := "\n" + strings.ToUpper(strings.ReplaceAll(value, "\r", "")) + "\n"
	return strings.Contains(upper, "\nOK\n") ||
		strings.Contains(upper, "\nERROR\n") ||
		strings.Contains(upper, "+CME ERROR:") ||
		strings.Contains(upper, "+CMS ERROR:")
}

func responseOK(value string) bool {
	lines := strings.Split(strings.ReplaceAll(value, "\r", ""), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "OK" {
			return true
		}
	}
	return false
}

func cleanATResponse(value string) string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r", ""), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\r\n")
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
