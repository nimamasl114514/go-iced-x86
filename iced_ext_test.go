package icedx86

import (
	"strings"
	"testing"
)

// 扩展测试字节流（64 位，ip=0x1000）：与 iced_test.go 头部注释的流一致。
var extTestCode = []byte{
	0x48, 0x89, 0xD8, // mov rax,rbx
	0x48, 0x8B, 0x43, 0x08, // mov rax,[rbx+8]
	0x0F, 0x84, 0x0A, 0x00, 0x00, 0x00, // jz 0x1017
	0xE8, 0x02, 0x00, 0x00, 0x00, // call 0x1012
	0xC3, // ret
	0xCC, // int3
	0x90, // nop
}

func TestFormatSyntaxes(t *testing.T) {
	loadForTest(t)
	d, err := NewDecoder(64, extTestCode[:3], 0x1000)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	inst, ok := d.Decode()
	if !ok {
		t.Fatal("decode failed")
	}
	intel := inst.Format(SyntaxIntel, 0)
	if intel != "mov rax,rbx" {
		t.Errorf("intel: got %q, want %q", intel, "mov rax,rbx")
	}
	gas := inst.Format(SyntaxGas, 0)
	if !strings.Contains(gas, "%") {
		t.Errorf("gas(AT&T) 应含 %% 寄存器前缀: %q", gas)
	}
	masm := inst.Format(SyntaxMasm, 0)
	nasm := inst.Format(SyntaxNasm, 0)
	if masm == "" || nasm == "" {
		t.Errorf("masm/nasm 输出为空: %q / %q", masm, nasm)
	}
	t.Logf("intel=%q gas=%q masm=%q nasm=%q", intel, gas, masm, nasm)

	upper := inst.Format(SyntaxIntel, OptUppercaseAll)
	if !strings.Contains(upper, "MOV") {
		t.Errorf("OptUppercaseAll 未生效: %q", upper)
	}
	if s := inst.Format(Syntax(99), 0); s != "" {
		t.Errorf("非法 syntax 应返回空串: %q", s)
	}
}

func TestDecodeAll(t *testing.T) {
	loadForTest(t)
	d1, err := NewDecoder(64, extTestCode, 0x1000)
	if err != nil {
		t.Fatal(err)
	}
	defer d1.Close()
	batch := d1.DecodeAll(64)

	// 逐条 Decode 对拍
	d2, err := NewDecoder(64, extTestCode, 0x1000)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	var seq []*Instruction
	for {
		inst, ok := d2.Decode()
		if !ok {
			break
		}
		seq = append(seq, inst)
	}
	if len(batch) != len(seq) {
		t.Fatalf("DecodeAll 条数 %d != 逐条 %d", len(batch), len(seq))
	}
	if len(batch) != 7 {
		t.Fatalf("期望 7 条, 实际 %d", len(batch))
	}
	for i := range batch {
		if batch[i].IP() != seq[i].IP() || batch[i].Len() != seq[i].Len() ||
			batch[i].Code() != seq[i].Code() || batch[i].NextIP() != seq[i].NextIP() {
			t.Errorf("第 %d 条不一致: batch(ip=%x len=%d code=%d) seq(ip=%x len=%d code=%d)",
				i, batch[i].IP(), batch[i].Len(), batch[i].Code(),
				seq[i].IP(), seq[i].Len(), seq[i].Code())
		}
	}
	// DecodeAll 后 decoder 应已耗尽
	if d1.CanDecode() {
		t.Error("DecodeAll 后仍可解码")
	}
	// 小缓冲截断
	d3, _ := NewDecoder(64, extTestCode, 0x1000)
	defer d3.Close()
	if got := len(d3.DecodeAll(3)); got != 3 {
		t.Errorf("DecodeAll(3) 返回 %d 条", got)
	}
}

func TestCodeTable(t *testing.T) {
	loadForTest(t)
	n := CodeCount()
	if n < 1000 {
		t.Fatalf("CodeCount=%d 异常（iced-x86 1.21 应 >4000）", n)
	}
	t.Logf("CodeCount=%d", n)
	if name, ok := CodeName(0); !ok || name != "INVALID" {
		t.Errorf("CodeName(0)=%q,%v, want INVALID", name, ok)
	}
	v, name, ok := CodeAt(0)
	if !ok || v != 0 || name != "INVALID" {
		t.Errorf("CodeAt(0)=(%d,%q,%v)", v, name, ok)
	}
	// 序号->值->名称 互洽抽样
	for _, i := range []int{1, n / 2, n - 1} {
		v, name, ok := CodeAt(i)
		if !ok {
			t.Fatalf("CodeAt(%d) 失败", i)
		}
		name2, ok2 := CodeName(v)
		if !ok2 || name2 != name {
			t.Errorf("CodeAt(%d)=(%d,%q) 与 CodeName(%d)=(%q,%v) 不一致", i, v, name, v, name2, ok2)
		}
	}
	// 越界与非法值
	if _, _, ok := CodeAt(n); ok {
		t.Error("CodeAt(count) 应失败")
	}
	if _, ok := CodeName(0xFFFFFFFF); ok {
		t.Error("CodeName(0xFFFFFFFF) 应失败")
	}
}
