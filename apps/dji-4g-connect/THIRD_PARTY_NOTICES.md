# Third-Party and Trademark Notices

DJI 4G Connect dynamically links against libusb 1.0.30.

- Project: <https://github.com/libusb/libusb>
- License: GNU Lesser General Public License v2.1 or later

The release app includes libusb's complete `COPYING` file in `Contents/Resources/libusb-COPYING`.

## Protocol information

This project uses publicly documented USB standards, the public libusb API, and
Quectel AT commands such as `AT+QCFG="usbnet"` and `AT+CFUN=1,1` to interoperate
with compatible hardware.

The current DJI 4G Connect source tree is an independent implementation. It does
not include source code from DJOneHub or VoHive. Archived v0.1.x releases retain
the license terms and notices shipped inside those archives.

## Trademarks

DJI and the DJI logo are trademarks of SZ DJI Technology Co., Ltd. Quectel is a
trademark of Quectel Wireless Solutions Co., Ltd. Other names may be trademarks
of their respective owners. Their use identifies compatible hardware only and
does not imply sponsorship, endorsement, authorization, or affiliation.
