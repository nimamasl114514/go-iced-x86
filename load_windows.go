//go:build windows

package icedx86

import "syscall"

// libFileName Rust cdylib 在 Windows 上的产物名。
const libFileName = "iced_go_ffi.dll"

// openLibrary Windows 平台经 syscall.LoadLibrary 加载。
// purego 不为 Windows 提供 Dlopen（官方 examples/libc 同款做法）；
// 拿到句柄后照常走 purego.RegisterLibFunc 绑定。
func openLibrary(path string) (uintptr, error) {
	h, err := syscall.LoadLibrary(path)
	return uintptr(h), err
}
