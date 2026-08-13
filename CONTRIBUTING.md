# Contributing

Contributions are welcome. Please open an issue before large behavioral changes
so the implementation and hardware test plan can be discussed first.

## Development

Requirements:

- Apple Silicon Mac
- macOS 13 or newer
- Go 1.26 or newer
- Xcode Command Line Tools
- `pkg-config`

Run tests with a prepared libusb pkg-config path, or use the full build script:

```sh
./scripts/build-app.sh dev
```

The script downloads libusb 1.0.30 from the official release, verifies its
SHA-256, builds a bundled dynamic library, creates the macOS `.app`, applies an
ad-hoc signature, verifies it, and produces a ZIP plus checksum.

## Pull requests

- Keep network-service cleanup narrowly scoped to Baiwang/EG25/QDC507 entries.
- Never modify unrelated macOS network services.
- Do not add telemetry, analytics, or remote data collection.
- Add tests for parsing or state-selection changes.
- Describe real-hardware validation when hardware behavior changes.

By contributing, you agree that your contribution is licensed under the MIT
License included in this repository.
