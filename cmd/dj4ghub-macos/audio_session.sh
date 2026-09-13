#!/bin/sh
# Temporary, leased audio session. Never writes persistent USB configuration.
set -eu
dir=$1
base=/sys/class/android_usb/android0
original=$(cat "$base/functions")
case "$original" in
  diag,serial,rmnet,ffs|diag,serial,ecm,ffs) ;;
  *) exit 21 ;;
esac
test "$(uname -r)" = 3.18.44 || exit 22
route_pid=
calibration_pid=
phase=drivers
stop_child() {
  test -n "$1" || return 0
  kill -TERM "$1" 2>/dev/null || true
  n=0
  while kill -0 "$1" 2>/dev/null; do
    n=$((n+1)); test "$n" -lt 100 || return 1
    sleep 0.1
  done
  wait "$1" 2>/dev/null || true
}
cleanup() {
  result=$?
  trap - EXIT
  if test "$result" -ne 0; then printf '%s (exit %s)\n' "$phase" "$result" > "$dir/failure"; fi
  printf 'stopping\n' > "$dir/state"
  if ! stop_child "$route_pid" || ! stop_child "$calibration_pid"; then
    printf 'reboot_required\n' > "$dir/state"
    exit 1
  fi
  echo 0 > /sys/class/android_usb/f_audio/audio_enable
  echo 0 > "$base/enable"
  printf '%s' "$original" > "$base/functions"
  echo 1 > "$base/enable"
  test "$(cat "$base/functions")" = "$original"
  printf 'closed\n' > "$dir/state"
}
trap cleanup EXIT
trap 'exit 0' TERM INT
printf 'preparing\n' > "$dir/state"
boot=$(cat /proc/sys/kernel/random/boot_id)
marker=/tmp/dj4hub-audio-runtime-owner
if test -d /sys/module/qdc507_voice || test -d /sys/module/qdc507_aprv3; then
  test -f "$marker" && test "$(cat "$marker")" = "$boot" || exit 23
else
  insmod "$dir/qdc507_aprv3.ko"
  insmod "$dir/qdc507_voice.ko"
  printf '%s' "$boot" > "$marker"
fi
n=0
until test -c /dev/snd/pcmC0D4p && test -c /dev/snd/pcmC0D6c; do
  n=$((n+1)); test "$n" -lt 50 || exit 24
  sleep 0.1
done
# Do not replace a vendor daemon or another application's audio session.
phase=calibration-start
if pidof alsaucm_test >/dev/null 2>&1; then exit 25; fi
/usr/bin/alsaucm_test > "$dir/calibration.log" 2>&1 &
calibration_pid=$!
n=0
until test -p /run/alsaucm_test && grep -q 'Online service registered with ACPH' "$dir/calibration.log"; do
  kill -0 "$calibration_pid"
  n=$((n+1)); test "$n" -lt 150 || exit 26
  sleep 0.1
done
phase=calibration-commands
timeout -t 10 sh -c 'printf "open snd_soc_msm_9x07_Tomtom_I2S\n" > /run/alsaucm_test; sleep 0.2; printf "set _verb VoLTE\n" > /run/alsaucm_test; sleep 0.2; printf "set _enadev Auxpcm Rx\n" > /run/alsaucm_test; sleep 0.2; printf "set _enadev Auxpcm Tx\n" > /run/alsaucm_test'
phase=calibration-ready
n=0
until grep -q 'Sent VocProc Cal!' "$dir/calibration.log"; do
  n=$((n+1)); test "$n" -lt 100 || exit 27
  sleep 0.1
done
phase=voice-route
"$dir/mavo-pcm-bridge.armv7" --voice-route-session > "$dir/route.log" 2>&1 &
route_pid=$!
n=0
until grep -q 'VoLTE route session active' "$dir/route.log"; do
  kill -0 "$route_pid"
  n=$((n+1)); test "$n" -lt 100 || exit 28
  sleep 0.1
done
phase=usb-audio
echo 0 > "$base/enable"
printf '%s' "$original,audio" > "$base/functions"
echo 1 > "$base/enable"
printf 'ready\n' > "$dir/state"
phase=running
while test ! -f "$dir/stop"; do
  now=$(cut -d . -f 1 /proc/uptime)
  lease=$(cat "$dir/lease")
  case "$lease" in ''|*[!0-9]*) exit 29;; esac
  test "$((now-lease))" -lt 45 || break
  kill -0 "$route_pid" && kill -0 "$calibration_pid" || exit 30
  sleep 1
done
