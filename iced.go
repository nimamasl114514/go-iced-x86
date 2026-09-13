// Package icedx86 provides pure Go bindings for the iced-x86 disassembler
// library. The Rust-compiled C ABI shared library (iced_go_ffi) is loaded at
// runtime via purego — no cgo, no C toolchain required for Go consumers.
package icedx86

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/ebitengine/purego"
)

var (
	libHandle uintptr
	loaded    bool
)

// 导出函数指针，签名须与 rust-iced/src/lib.rs 完全一致。
// purego 在 Linux/Windows 不支持 struct 值传递，Instruction 一律走指针。
var (
	fnDecoderNew                  func(bitness int32, bytes *byte, length uintptr, ip uint64) uintptr
	fnDecode                      func(decoder uintptr, instruction uintptr) int32
	fnDecoderFree                 func(decoder uintptr)
	fnDecoderCanDecode            func(decoder uintptr) int32
	fnDecoderPosition             func(decoder uintptr) uint64
	fnInstructionSize             func() uintptr
	fnInstructionAlign            func() uintptr
	fnInstructionLen              func(instruction uintptr) uintptr
	fnInstructionIP               func(instruction uintptr) uint64
	fnInstructionNextIP           func(instruction uintptr) uint64
	fnInstructionCode             func(instruction uintptr) uint32
	fnInstructionMnemonic         func(instruction uintptr) uint32
	fnInstructionFlowControl      func(instruction uintptr) uint32
	fnInstructionNearBranchTarget func(instruction uintptr) uint64
	fnFormatIntel                 func(instruction uintptr) uintptr
	fnStringFree                  func(s uintptr)
)

var (
	instructionSize  int
	instructionAlign int
)

// ErrNotLoaded 表示尚未调用 Load 加载动态库。
var ErrNotLoaded = errors.New("icedx86: library not loaded, call Load first")

// Load 加载 iced_go_ffi 动态库并绑定全部导出函数。
// path 可以是库文件的完整路径，也可以是包含该库的目录。
// 必须在任何解码操作之前调用一次；重复调用返回错误。
func Load(path string) error {
	if loaded {
		return errors.New("icedx86: library already loaded")
	}
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		path = filepath.Join(path, libFileName)
	}
	h, err := openLibrary(path)
	if err != nil {
		return fmt.Errorf("icedx86: open %s: %w", path, err)
	}
	libHandle = h
	bindFunctions()
	instructionSize = int(fnInstructionSize())
	instructionAlign = int(fnInstructionAlign())
	if instructionSize <= 0 || instructionAlign <= 0 || instructionAlign > 64 {
		return fmt.Errorf("icedx86: bogus instruction size/align from library: %d/%d",
			instructionSize, instructionAlign)
	}
	loaded = true
	return nil
}

func bindFunctions() {
	purego.RegisterLibFunc(&fnDecoderNew, libHandle, "iced_decoder_new")
	purego.RegisterLibFunc(&fnDecode, libHandle, "iced_decode")
	purego.RegisterLibFunc(&fnDecoderFree, libHandle, "iced_decoder_free")
	purego.RegisterLibFunc(&fnDecoderCanDecode, libHandle, "iced_decoder_can_decode")
	purego.RegisterLibFunc(&fnDecoderPosition, libHandle, "iced_decoder_position")
	purego.RegisterLibFunc(&fnInstructionSize, libHandle, "iced_instruction_size")
	purego.RegisterLibFunc(&fnInstructionAlign, libHandle, "iced_instruction_align")
	purego.RegisterLibFunc(&fnInstructionLen, libHandle, "iced_instruction_len")
	purego.RegisterLibFunc(&fnInstructionIP, libHandle, "iced_instruction_ip")
	purego.RegisterLibFunc(&fnInstructionNextIP, libHandle, "iced_instruction_next_ip")
	purego.RegisterLibFunc(&fnInstructionCode, libHandle, "iced_instruction_code")
	purego.RegisterLibFunc(&fnInstructionMnemonic, libHandle, "iced_instruction_mnemonic")
	purego.RegisterLibFunc(&fnInstructionFlowControl, libHandle, "iced_instruction_flow_control")
	purego.RegisterLibFunc(&fnInstructionNearBranchTarget, libHandle, "iced_instruction_near_branch_target")
	purego.RegisterLibFunc(&fnFormatIntel, libHandle, "iced_format_intel")
	purego.RegisterLibFunc(&fnStringFree, libHandle, "iced_string_free")
}

// goString 将 C 字符串指针复制为 Go string（手动测长，不依赖外部转换）。
func goString(cstr uintptr) string {
	if cstr == 0 {
		return ""
	}
	n := 0
	for q := cstr; *(*byte)(unsafe.Pointer(q)) != 0; q++ {
		n++
	}
	if n == 0 {
		return ""
	}
	// 必须复制而非共享：调用方随后会 free 这块 C 内存，
	// unsafe.String 共享底层字节会导致 use-after-free（free 后首字节被分配器改写）。
	return string(unsafe.Slice((*byte)(unsafe.Pointer(cstr)), n))
}

