//go:build darwin

package main

import "os/exec"

func showNotification(title, message string) {
	const script = `on run argv
display notification (item 2 of argv) with title (item 1 of argv)
end run`
	_ = exec.Command("/usr/bin/osascript", "-e", script, "--", title, message).Run()
}
