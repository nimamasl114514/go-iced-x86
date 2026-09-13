// gencode 生成 codes.go：Code 枚举常量表（数据源为运行时加载的 iced_go_ffi）。
//
// 用法（仓库根目录，需先构建 rust-iced）：
//
//	go run ./cmd/gencode > codes.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	icedx86 "github.com/nimamasl114514/go-iced-x86"
)

func main() {
	lib := os.Getenv("ICEDX86_LIB")
	if lib == "" {
		for _, p := range []string{"iced_go_ffi.dll", filepath.Join("rust-iced", "target", "release", "iced_go_ffi.dll")} {
			if _, err := os.Stat(p); err == nil {
				lib = p
				break
			}
		}
	}
	if lib == "" {
		fmt.Fprintln(os.Stderr, "gencode: 找不到 iced_go_ffi 动态库（先 cargo build --release，或设 ICEDX86_LIB）")
		os.Exit(1)
	}
	if err := icedx86.Load(lib); err != nil {
		fmt.Fprintln(os.Stderr, "gencode:", err)
		os.Exit(1)
	}

	n := icedx86.CodeCount()
	fmt.Println("// Code 枚举常量表（数据源 iced-x86 1.21.0），由 cmd/gencode 自动生成，勿手改。")
	fmt.Println("// 重新生成：go run ./cmd/gencode > codes.go")
	fmt.Println()
	fmt.Println("package icedx86")
	fmt.Println()
	fmt.Println("const (")
	seen := map[string]int{}
	for i := 0; i < n; i++ {
		v, name, ok := icedx86.CodeAt(i)
		if !ok {
			fmt.Fprintf(os.Stderr, "gencode: index %d 取名失败\n", i)
			os.Exit(1)
		}
		ident := goIdent(name)
		if ident == "Invalid" {
			// types.go 已提供 CodeInvalid，避免重复定义
			fmt.Printf("\t// Code%s = %d（见 types.go CodeInvalid）\n", ident, v)
			continue
		}
		if prev, dup := seen[ident]; dup {
			fmt.Fprintf(os.Stderr, "gencode: 标识符冲突 Code%s（值 %d 与 %d），该条用去下划线原名\n", ident, v, prev)
			ident = strings.ReplaceAll(name, "_", "")
		}
		seen[ident] = int(v)
		fmt.Printf("\tCode%s uint32 = %d // %s\n", ident, v, name)
	}
	fmt.Println(")")
}

// goIdent 把 iced-x86 枚举变体名（如 Mov_rm64_r64、VZEROUPPER）转为 Go 导出标识片段。
func goIdent(s string) string {
	var b strings.Builder
	for _, p := range strings.Split(s, "_") {
		if p == "" {
			continue
		}
		if p == strings.ToUpper(p) {
			b.WriteString(title(p))
		} else {
			b.WriteString(strings.ToUpper(p[:1]) + p[1:])
		}
	}
	return b.String()
}

func title(s string) string {
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}
