package icedx86

import (
	"errors"
	"fmt"
	"unsafe"
)

// FlowControl 指令控制流类别，取值与 iced-x86 的 FlowControl 枚举一致
// （数值经 iced-x86 1.21 源码 flow_control.rs 实证，勿凭记忆改动）。
type FlowControl uint32

const (
	FlowControlNext                FlowControl = 0
	FlowControlUnconditionalBranch FlowControl = 1
	FlowControlIndirectBranch      FlowControl = 2
	FlowControlConditionalBranch   FlowControl = 3
	FlowControlReturn              FlowControl = 4
	FlowControlCall                FlowControl = 5
	FlowControlIndirectCall        FlowControl = 6
	FlowControlInterrupt           FlowControl = 7
	FlowControlXbeginXabortXend    FlowControl = 8
	FlowControlException           FlowControl = 9
)

func (f FlowControl) String() string {
	switch f {
	case FlowControlNext:
		return "Next"
	case FlowControlUnconditionalBranch:
		return "UnconditionalBranch"
	case FlowControlIndirectBranch:
		return "IndirectBranch"
	case FlowControlConditionalBranch:
		return "ConditionalBranch"
	case FlowControlReturn:
		return "Return"
	case FlowControlCall:
		return "Call"
	case FlowControlIndirectCall:
		return "IndirectCall"
	case FlowControlInterrupt:
		return "Interrupt"
	case FlowControlXbeginXabortXend:
		return "XbeginXabortXend"
	case FlowControlException:
		return "Exception"
	default:
		return fmt.Sprintf("FlowControl(%d)", uint32(f))
	}
}

// CodeInvalid 是 iced-x86 Code 枚举的零值（INVALID）。
const CodeInvalid uint32 = 0

// Decoder 包装 Rust iced-x86 Decoder（堆上不透明指针）。
// 非并发安全。用毕须调用 Close 释放 Rust 侧内存。
type Decoder struct {
	ptr  uintptr
	code []byte // 保活引用：Rust Decoder 借用此内存，Close 之前不得被 GC 回收
}

// NewDecoder 创建解码器。
//   - bitness: 16 / 32 / 64
//   - code: 待解码机器码（解码期间该切片被 Rust 借用，调用方不得修改）
//   - ip: 第一条指令的虚拟地址（影响相对寻址类指令的目标解析）
func NewDecoder(bitness int, code []byte, ip uint64) (*Decoder, error) {
	if !loaded {
		return nil, ErrNotLoaded
	}
	if bitness != 16 && bitness != 32 && bitness != 64 {
		return nil, fmt.Errorf("icedx86: invalid bitness %d (want 16/32/64)", bitness)
	}
	if len(code) == 0 {
		return nil, errors.New("icedx86: empty code slice")
	}
	h := fnDecoderNew(int32(bitness), &code[0], uintptr(len(code)), ip)
	if h == 0 {
		return nil, errors.New("icedx86: iced_decoder_new returned null")
	}
	return &Decoder{ptr: h, code: code}, nil
}

// Decode 解码下一条指令；数据耗尽返回 (nil, false)。
// 遇到无法解码的字节会产出 Code() == CodeInvalid 的指令（ok 仍为 true）。
func (d *Decoder) Decode() (*Instruction, bool) {
	if d.ptr == 0 {
		return nil, false
	}
	inst := newInstruction()
	if fnDecode(d.ptr, inst.data) == 0 {
		return nil, false
	}
	return inst, true
}

// CanDecode 报告是否还有数据可解码。
func (d *Decoder) CanDecode() bool {
	return d.ptr != 0 && fnDecoderCanDecode(d.ptr) != 0
}

// Position 返回已消耗的字节数。
func (d *Decoder) Position() uint64 {
	if d.ptr == 0 {
		return 0
	}
	return fnDecoderPosition(d.ptr)
}

// Close 释放 Rust 侧 Decoder。可重复调用。
func (d *Decoder) Close() {
	if d.ptr != 0 {
		fnDecoderFree(d.ptr)
		d.ptr = 0
		d.code = nil
	}
}

// Instruction 一条已解码指令。
// 内存由 Go 侧分配（尺寸与对齐由 Rust 库运行时报告），Rust 侧按值写入。
type Instruction struct {
	buf  []byte  // 底层分配，防止 GC
	data uintptr // buf 内满足对齐要求的起始地址
}

func newInstruction() *Instruction {
	raw := make([]byte, instructionSize+instructionAlign-1)
	base := uintptr(unsafe.Pointer(&raw[0]))
	mask := uintptr(instructionAlign - 1)
	aligned := (base + mask) &^ mask
	return &Instruction{buf: raw, data: aligned}
}

// Len 指令长度（字节）。
func (i *Instruction) Len() int { return int(fnInstructionLen(i.data)) }

// IP 指令地址。
func (i *Instruction) IP() uint64 { return fnInstructionIP(i.data) }

// NextIP 下一条指令地址（IP + Len）。
func (i *Instruction) NextIP() uint64 { return fnInstructionNextIP(i.data) }

// Code iced-x86 Code 枚举值（指令精确标识）。
func (i *Instruction) Code() uint32 { return fnInstructionCode(i.data) }

// Mnemonic iced-x86 Mnemonic 枚举值。
func (i *Instruction) Mnemonic() uint32 { return fnInstructionMnemonic(i.data) }

// FlowControl 控制流类别（CFG 恢复用）。
func (i *Instruction) FlowControl() FlowControl { return FlowControl(fnInstructionFlowControl(i.data)) }

// NearBranchTarget 近分支目标地址（仅对 branch/call 类指令有意义）。
func (i *Instruction) NearBranchTarget() uint64 { return fnInstructionNearBranchTarget(i.data) }

// FormatIntel 格式化为 Intel 语法（如 "mov rax,rbx"）。
// 每次调用在 Rust 侧新建 formatter，返回前已释放 C 字符串，无泄漏。
func (i *Instruction) FormatIntel() string {
	p := fnFormatIntel(i.data)
	if p == 0 {
		return ""
	}
	s := goString(p)
	fnStringFree(p)
	return s
}

// String 实现 fmt.Stringer，等同 FormatIntel。
func (i *Instruction) String() string { return i.FormatIntel() }

// ============================================================================
// 扩展 API：多语法格式化 / 批量解码 / Code 常量表
// ============================================================================

// Syntax 汇编输出语法风格。
type Syntax int

const (
	SyntaxIntel Syntax = iota // Intel 语法（默认）
	SyntaxGas                 // AT&T 语法
	SyntaxMasm                // MASM 语法
	SyntaxNasm                // NASM 语法
)

// FormatOptions 格式化选项位掩码，位定义与 rust-iced lib.rs 保持一致。
type FormatOptions uint32

const (
	OptSpaceAfterOperandSeparator FormatOptions = 1 << iota // 操作数分隔符后加空格
	OptSpaceAfterMemoryBracket                              // 内存括号后加空格
	OptUppercaseAll                                         // 全部大写
	OptShowZeroDisplacements                                // 显示零位移
	OptUppercaseHex                                         // 十六进制数字大写
	OptAlwaysShowSegmentRegister                            // 总显示段寄存器
)

// Format 按指定语法与选项格式化指令；syntax 非法或库未加载时返回空串。
// 每次调用在 Rust 侧新建 formatter；如需高频格式化请批量调用方复用结果。
func (i *Instruction) Format(syntax Syntax, opts FormatOptions) string {
	p := fnFormat(i.data, int32(syntax), uint32(opts))
	if p == 0 {
		return ""
	}
	s := goString(p)
	fnStringFree(p)
	return s
}

// DecodeAll 批量解码至多 max 条指令，数据耗尽时返回条数少于 max。
// 与逐条 Decode 语义等价，但只跨一次 FFI 边界，适合整段反汇编。
// 返回的指令共享同一块底层缓冲（随任一指令保活）。
func (d *Decoder) DecodeAll(max int) []*Instruction {
	if d.ptr == 0 || max <= 0 {
		return nil
	}
	stride := instructionSize + instructionAlign
	raw := make([]byte, max*stride+instructionAlign)
	mask := uintptr(instructionAlign - 1)
	first := (uintptr(unsafe.Pointer(&raw[0])) + mask) &^ mask
	n := int(fnDecodeAll(d.ptr, first, uintptr(stride), uintptr(max)))
	out := make([]*Instruction, 0, n)
	for k := 0; k < n; k++ {
		out = append(out, &Instruction{buf: raw, data: first + uintptr(k*stride)})
	}
	return out
}

// CodeCount 返回 iced-x86 Code 枚举的条目总数。
func CodeCount() int { return int(fnCodeCount()) }

// CodeName 返回 Code 枚举值对应的名称（如 "Mov_rm64_r64"）；非法值 ok=false。
func CodeName(code uint32) (name string, ok bool) {
	p := fnCodeName(code)
	if p == 0 {
		return "", false
	}
	s := goString(p)
	fnStringFree(p)
	return s, true
}

// CodeAt 按枚举序号取 Code 值与名称（0 <= index < CodeCount()）。
// 用于离线生成常量表（见 cmd/gencode）。
func CodeAt(index int) (code uint32, name string, ok bool) {
	var v uint32
	p := fnCodeAt(uint32(index), &v)
	if p == 0 {
		return 0, "", false
	}
	s := goString(p)
	fnStringFree(p)
	return v, s, true
}
