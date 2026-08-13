# DJ 4G Hub for macOS

This branch adds a native macOS service for supported DJI 4G modules / Quectel
EG25-G. It does not require UTM for AT-mode management.

## Current scope

- Automatic discovery of DJI (`2ca3`) and Quectel (`2c7c`) USB serial ports
- Modem, SIM, operator, registration and signal status
- Receive and send SMS through the modem AT port
- Execute explicit AT commands
- Read and switch physical eUICC profiles through AT APDU transport
- Local management page at `http://127.0.0.1:7575`
- Packaged Apple Silicon release (Intel packaging is planned separately)

The cellular data interface remains managed by macOS. This allows macOS to use
the dongle as its network connection while DJ 4G Hub uses a separate USB serial
interface for management.

## Downloaded release

The Apple Silicon ZIP contains the executable, its libusb runtime, licenses and
the `dj4ghub` terminal launcher. It does not require Go, Homebrew or a separately
installed libusb on the user's Mac.

From the extracted release directory:

```sh
./dj4ghub start
```

The terminal remains attached to the service and the management page opens
automatically. Press `Control+C` to stop it, or run `./dj4ghub stop` from another
terminal in the same directory. Logs are stored in
`~/Library/Logs/DJ 4G Hub/dj4ghub.log`.

## Build from source

Requirements:

- macOS 13 or newer
- Go 1.26 or newer

```sh
./scripts/package-macos-arm64.sh v0.1.0-preview
```

Release outputs:

- `dist/release/DJ-4G-Hub-macOS-arm64-v0.1.0-preview/`
- `dist/release/DJ-4G-Hub-macOS-arm64-v0.1.0-preview.zip`
- `dist/release/DJ-4G-Hub-macOS-arm64-v0.1.0-preview.zip.sha256`

The packaging script downloads the official libusb source archive, verifies its
SHA-256, builds it for macOS 13 or newer and bundles the resulting runtime.

## Run

Connect the modem and run:

```sh
./dist/dj4ghub-macos
```

If automatic discovery picks no AT port, inspect `/dev/cu.*` and pass it:

```sh
./dist/dj4ghub-macos -port /dev/cu.usbmodemXXXX
```

The server only listens on localhost by default. Open:

```text
http://127.0.0.1:7575
```

## Demo without hardware

To explore the management page before buying the module, run:

```sh
./dist/dj4ghub-macos -demo
```

Then open `http://127.0.0.1:7575`. Demo mode provides simulated modem status,
SMS messages, AT command responses and eSIM profiles. It does not access a real
SIM, send messages or switch a physical eSIM profile.

## One-shot network activation

To prepare the ECM network interface without starting the web service, stop any
running DJ 4G Hub service and run:

```sh
./dj4ghub activate
```

The command removes or disables only orphaned Baiwang/EG25/QDC507 network
services, ensures `usbnet=1`, reboots the module when the ECM interface is
unavailable, waits for macOS DHCP, and then exits.

## Logs

Logs are written to `~/Library/Logs/DJ 4G Hub/dj4ghub.log`.

## Platform limitations

- Native QMI/MBIM control, Linux udev and network-namespace orchestration are
  excluded from this macOS entry point.
- eSIM behavior depends on the physical eUICC and modem firmware. Profile
  switching must be verified with real hardware.
- The release uses an ad-hoc signature rather than an Apple Developer ID. On
  first run, macOS may require approval in Privacy & Security.
