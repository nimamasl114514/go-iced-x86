//go:build linux || darwin

package icedx86

import (
	"runtime"

	"github.com/ebitengine/purego"
)

// libFileName 按平台选择（linux: .so, darwin: .dylib）。
var libFileName = map[string]string{
	"linux":  "libiced_go_ffi.so",
	"darwin": "libiced_go_ffi.dylib",
}[runtime.GOOS]

func openLibrary(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}
