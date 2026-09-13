# go-iced-x86: Pure Go bindings for iced-x86

**No cgo. Powered by purego.**

将 [iced-x86](https://github.com/icedland/iced)（工业级 x86/x64 反汇编库，Rust 实现）编译为 C ABI 动态库，经 [purego](https://github.com/ebitengine/purego) 在 Go 中直接调用。Go 侧构建全程 `CGO_ENABLED=0`，**无需任何 C 编译器**，交叉编译无忧。

## 特性

- **纯 Go 消费侧**：`go build` 不需要 gcc/clang，`CGO_ENABLED=0` 照常工作
- ** iced-x86 全精度解码**：指令长度、Code/Mnemonic 枚举、FlowControl、近分支目标地址
- **Intel 语法格式化**：`FormatIntel()` 直接产出可读汇编
- **明确的内存所有权**：Decoder 不透明指针由 Rust 管理；Instruction 由 Go 分配、Rust 按值写入；C 字符串谁分配谁释放
- **三平台**：Windows（`.dll`）/ Linux（`.so`）/ macOS（`.dylib`）

## 架构

```
┌────────────── Go (package icedx86) ──────────────┐
│  Decoder / Instruction / FlowControl 公开 API     │
│  iced.go: purego.Dlopen + RegisterLibFunc 绑定    │
└────────────────── C ABI (cdylib) ─────────────────┘
┌────────────── Rust (rust-iced) ──────────────────┐
│  iced_decoder_new / iced_decode / iced_format_…  │
│  内联依赖 iced-x86 = "1.21"（cargo 编译）         │
└──────────────────────────────────────────────────┘
```

## 构建

前置：Rust 工具链（cargo）+ Go 1.21+。

```bash
# 1. 编译 Rust FFI 动态库
cd rust-iced && cargo build --release

# 2. 把产物复制到仓库根（Windows 示例）
cp target/release/iced_go_ffi.dll ..

# 3. 构建并测试 Go 侧
cd .. && go mod tidy
CGO_ENABLED=0 go test -v .
```

Windows 一键：`build.bat`；类 Unix：`make`。

## 快速上手

```go
package main

import (
	"fmt"
	"log"

	icedx86 "github.com/nimamasl114514/go-iced-x86"
)

func main() {
	// 加载动态库（参数可为库文件路径或所在目录）
	if err := icedx86.Load("iced_go_ffi.dll"); err != nil {
		log.Fatal(err)
	}

	code := []byte{0x48, 0x89, 0xD8, 0xC3} // mov rax,rbx; ret
	d, err := icedx86.NewDecoder(64, code, 0x1000)
	if err != nil {
		log.Fatal(err)
	}
	defer d.Close()

	for {
		inst, ok := d.Decode()
		if !ok {
			break
		}
		fmt.Printf("%016X  %-20s flow=%v\n",
			inst.IP(), inst.FormatIntel(), inst.FlowControl())
	}
}
```

输出：

```
0000000000001000  mov rax,rbx          flow=Next
0000000000001003  ret                  flow=Return
```

## API 概览

| Go API | 说明 |
|---|---|
| `Load(path)` | 加载动态库并绑定导出函数（必先调用一次） |
| `NewDecoder(bitness, code, ip)` | 创建解码器（bitness ∈ 16/32/64） |
| `(*Decoder).Decode()` | 解码下一条，耗尽返回 `(nil, false)` |
| `(*Decoder).CanDecode() / Position() / Close()` | 状态与释放 |
| `(*Instruction).IP() / NextIP() / Len()` | 地址与长度 |
| `(*Instruction).Code() / Mnemonic()` | iced-x86 枚举值 |
| `(*Instruction).FlowControl()` | 控制流类别（CFG 恢复） |
| `(*Instruction).NearBranchTarget()` | 近分支目标地址 |
| `(*Instruction).FormatIntel()` | Intel 语法文本 |

## 设计约束（为何这么实现）

- **struct 走指针不走值**：purego 的 struct 值传递仅支持 Darwin amd64/arm64，`Instruction` 由 Go 预分配内存（尺寸/对齐经 `iced_instruction_size/align` 运行时获取），Rust 按值写入，三平台行为一致。
- **code 切片保活**：Rust `Decoder<'a>` 借用输入字节流，Go `Decoder` 结构体持有 `code` 引用至 `Close`，防止 GC 提前回收。
- **panic 不出 FFI**：非法 bitness、空指针在 Rust 边界前置拦截（返回 null/0）。
- **字符串所有权**：`iced_format_intel` 返回的 C 字符串在 Go 侧拷贝后立即 `iced_string_free`。

## 目录

```
go-iced-x86/
├── rust-iced/          # Rust C ABI 封装层（cargo 项目）
│   ├── Cargo.toml      # crate-type = ["cdylib"], iced-x86 = "1.21"
│   └── src/lib.rs      # 导出函数集
├── iced.go             # purego 加载与绑定
├── types.go            # Decoder / Instruction / FlowControl 公开 API
├── load_windows.go     # Windows 库加载
├── load_unix.go        # Linux/macOS 库加载
├── iced_test.go        # 对拍测试（地址序列 / 分支目标 / 边界）
├── Makefile            # 类 Unix 构建编排
└── build.bat           # Windows 一键构建
```

## 许可证

绑定层代码 MIT。iced-x86 本身遵循其上游许可证（MIT）。


## 扩展 API

- `Instruction.Format(syntax, opts)`：多语法格式化（`SyntaxIntel` / `SyntaxGas` / `SyntaxMasm` / `SyntaxNasm`），
  选项位 `OptUppercaseAll`、`OptSpaceAfterOperandSeparator`、`OptShowZeroDisplacements` 等；
  `FormatIntel()` 等价于 `Format(SyntaxIntel, 0)`
- `Decoder.DecodeAll(max)`：单次跨越 FFI 边界的批量解码，语义与逐条 `Decode()` 等价
- Code 常量表：`codes.go` 内置全部 4935 项 `Code*` 常量（数据源 iced-x86 1.21.0）；
  运行时查询 `CodeCount()` / `CodeName(code)` / `CodeAt(index)`；
  重新生成：`go run ./cmd/gencode > codes.go`
