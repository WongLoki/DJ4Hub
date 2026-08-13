//go:build darwin && cgo

package main

/*
#cgo pkg-config: libusb-1.0
#include <stdlib.h>
#include <libusb.h>
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

const (
	djiUSBVendorID  = 0x2ca3
	djiUSBProductID = 0x4006
)

type usbAT struct {
	ctx         *C.libusb_context
	handle      *C.libusb_device_handle
	iface       int
	endpointIn  byte
	endpointOut byte
	mu          sync.Mutex
}

type usbATCandidate struct {
	iface       int
	endpointIn  byte
	endpointOut byte
}

func openDJIUSBAT() (*usbAT, error) {
	var context *C.libusb_context
	if result := C.libusb_init(&context); result != 0 {
		return nil, fmt.Errorf("libusb init: %s", usbErrorName(result))
	}
	handle := C.libusb_open_device_with_vid_pid(context, djiUSBVendorID, djiUSBProductID)
	if handle == nil {
		C.libusb_exit(context)
		return nil, errors.New("DJI USB AT device 2ca3:4006 not found")
	}
	candidates, err := usbATCandidates(handle)
	if err != nil {
		C.libusb_close(handle)
		C.libusb_exit(context)
		return nil, err
	}
	var lastErr error
	for _, candidate := range candidates {
		if result := C.libusb_claim_interface(handle, C.int(candidate.iface)); result != 0 {
			lastErr = fmt.Errorf("claim USB AT interface %d: %s", candidate.iface, usbErrorName(result))
			continue
		}
		device := &usbAT{
			ctx:         context,
			handle:      handle,
			iface:       candidate.iface,
			endpointIn:  candidate.endpointIn,
			endpointOut: candidate.endpointOut,
		}
		if response, probeErr := device.Command("AT", 900*time.Millisecond); probeErr == nil && atSucceeded(response) {
			return device, nil
		} else {
			if probeErr == nil {
				probeErr = fmt.Errorf("unexpected AT probe response %q", response)
			}
			lastErr = fmt.Errorf("probe USB AT interface %d: %w", candidate.iface, probeErr)
		}
		C.libusb_release_interface(handle, C.int(candidate.iface))
	}
	C.libusb_close(handle)
	C.libusb_exit(context)
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("no USB bulk interface candidates found for DJI AT bridge")
}

func usbATCandidates(handle *C.libusb_device_handle) ([]usbATCandidate, error) {
	device := C.libusb_get_device(handle)
	if device == nil {
		return nil, errors.New("libusb device handle has no device")
	}
	var config *C.struct_libusb_config_descriptor
	if result := C.libusb_get_active_config_descriptor(device, &config); result != 0 {
		return nil, fmt.Errorf("get active USB config descriptor: %s", usbErrorName(result))
	}
	defer C.libusb_free_config_descriptor(config)

	var candidates []usbATCandidate
	interfaces := unsafe.Slice(config._interface, int(config.bNumInterfaces))
	for _, item := range interfaces {
		altsettings := unsafe.Slice(item.altsetting, int(item.num_altsetting))
		for _, alt := range altsettings {
			var endpointIn, endpointOut byte
			endpoints := unsafe.Slice(alt.endpoint, int(alt.bNumEndpoints))
			for _, endpoint := range endpoints {
				attributes := byte(endpoint.bmAttributes) & byte(C.LIBUSB_TRANSFER_TYPE_MASK)
				if attributes != byte(C.LIBUSB_TRANSFER_TYPE_BULK) {
					continue
				}
				address := byte(endpoint.bEndpointAddress)
				if address&byte(C.LIBUSB_ENDPOINT_IN) != 0 {
					endpointIn = address
				} else {
					endpointOut = address
				}
			}
			if endpointIn != 0 && endpointOut != 0 {
				candidates = append(candidates, usbATCandidate{
					iface:       int(alt.bInterfaceNumber),
					endpointIn:  endpointIn,
					endpointOut: endpointOut,
				})
			}
		}
	}
	return candidates, nil
}

func (device *usbAT) Close() {
	if device == nil {
		return
	}
	device.mu.Lock()
	defer device.mu.Unlock()
	if device.handle == nil {
		return
	}
	C.libusb_release_interface(device.handle, C.int(device.iface))
	C.libusb_close(device.handle)
	C.libusb_exit(device.ctx)
	device.handle = nil
	device.ctx = nil
}

func (device *usbAT) Command(command string, timeout time.Duration) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" || !strings.HasPrefix(strings.ToUpper(command), "AT") {
		return "", errors.New("AT command is invalid")
	}
	device.mu.Lock()
	defer device.mu.Unlock()
	if device.handle == nil {
		return "", errors.New("USB AT device is not open")
	}
	device.drainLocked()
	if err := device.bulkWriteLocked(device.endpointOut, []byte(command+"\r"), timeout); err != nil {
		return "", err
	}
	deadline := time.Now().Add(timeout)
	var chunks []string
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining > 900*time.Millisecond {
			remaining = 900 * time.Millisecond
		}
		data, err := device.bulkReadLocked(device.endpointIn, remaining)
		if err != nil {
			if errors.Is(err, errUSBTimeout) {
				continue
			}
			return strings.Join(chunks, ""), err
		}
		chunks = append(chunks, string(data))
		joined := strings.Join(chunks, "")
		if atResponseComplete(joined) {
			return normalizeATResponse(joined), nil
		}
	}
	if len(chunks) == 0 {
		return "", errors.New("USB AT command timed out without response")
	}
	return normalizeATResponse(strings.Join(chunks, "")), nil
}

var errUSBTimeout = errors.New("usb timeout")

func (device *usbAT) drainLocked() {
	for {
		if _, err := device.bulkReadLocked(device.endpointIn, 80*time.Millisecond); err != nil {
			return
		}
	}
}

func (device *usbAT) bulkWriteLocked(endpoint byte, payload []byte, timeout time.Duration) error {
	var transferred C.int
	result := C.libusb_bulk_transfer(
		device.handle,
		C.uchar(endpoint),
		(*C.uchar)(unsafe.Pointer(&payload[0])),
		C.int(len(payload)),
		&transferred,
		C.uint(timeout.Milliseconds()),
	)
	if result != 0 {
		return fmt.Errorf("USB bulk write: %s", usbErrorName(result))
	}
	if int(transferred) != len(payload) {
		return fmt.Errorf("USB bulk write short transfer: %d/%d", transferred, len(payload))
	}
	return nil
}

func (device *usbAT) bulkReadLocked(endpoint byte, timeout time.Duration) ([]byte, error) {
	buffer := make([]byte, 512)
	var transferred C.int
	result := C.libusb_bulk_transfer(
		device.handle,
		C.uchar(endpoint),
		(*C.uchar)(unsafe.Pointer(&buffer[0])),
		C.int(len(buffer)),
		&transferred,
		C.uint(timeout.Milliseconds()),
	)
	if result == C.LIBUSB_ERROR_TIMEOUT {
		return nil, errUSBTimeout
	}
	if result != 0 {
		return nil, fmt.Errorf("USB bulk read: %s", usbErrorName(result))
	}
	return buffer[:int(transferred)], nil
}

func usbErrorName(result C.int) string {
	return C.GoString(C.libusb_error_name(result))
}

func atResponseComplete(response string) bool {
	normalized := strings.ReplaceAll(response, "\r\n", "\n")
	return strings.Contains(normalized, "\nOK\n") ||
		strings.HasSuffix(normalized, "\nOK") ||
		atResponseIsError(normalized)
}

func atResponseIsError(response string) bool {
	normalized := strings.ToUpper(strings.ReplaceAll(response, "\r\n", "\n"))
	return strings.Contains(normalized, "\nERROR\n") ||
		strings.HasSuffix(normalized, "\nERROR") ||
		strings.Contains(normalized, "+CME ERROR:") ||
		strings.Contains(normalized, "+CMS ERROR:")
}

func atSucceeded(response string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(response), "\r\n", "\n")
	return normalized == "OK" || strings.HasSuffix(normalized, "\nOK")
}

func normalizeATResponse(response string) string {
	response = strings.ReplaceAll(response, "\r\r\n", "\r\n")
	response = strings.TrimSpace(response)
	lines := strings.Split(response, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) != "" {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\r\n")
}
