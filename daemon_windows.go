//go:build windows

package main

import (
	"fmt"
	"os"
)

func daemonize() {
	fmt.Fprintln(os.Stderr, "[daemon] Windows 不支持 -daemon 模式，请使用 start.sh 或 nohup 在 WSL/Linux 下后台运行。")
	os.Exit(1)
}
