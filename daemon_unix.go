//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func daemonize() {
	args := make([]string, 0, len(os.Args))
	for _, a := range os.Args[1:] {
		if a != "-daemon" && a != "--daemon" {
			args = append(args, a)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Dir = "/"
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "后台启动失败:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "[daemon] 已后台启动，PID: %d\n", cmd.Process.Pid)
}
