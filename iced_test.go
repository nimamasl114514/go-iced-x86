package icedx86

import (
	"os"
	"path/filepath"
	"testing"
)

// loadForTest 定位并加载动态库：环境变量 ICEDX86_LIB 优先，
// 其次仓库根目录 / rust-iced target 产物。
func loadForTest(t *testing.T) {
	t.Helper()
	if loaded {
		return
	}
	candidates := []string{}
	if p := os.Getenv("ICEDX86_LIB"); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates,
		libFileName,
		filepath.Join("rust-iced", "target", "release", libFileName),
	)
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			if err := Load(p); err != nil {
				t.Fatalf("Load(%s): %v", p, err)
			}
			t.Logf("loaded library: %s (instructionSize=%d align=%d)", p, instructionSize, instructionAlign)
			return
		}
	}
	t.Skipf("library %s not found, build rust-iced first", libFileName)
}

// 测试字节流（64 位模式，ip=0x1000）：
//
//	0x1000  48 89 D8              mov rax,rbx
//	0x1003  48 8B 43 08           mov rax,[rbx+8]
//	0x1007  0F 84 0A 00 00 00     jz 0x1017        (next=0x100D + disp 0x0A)
//	0x100D  E8 05 00 00 00        call 0x1017      (next=0x1012 + disp 0x05)
//	0x1012  C3                    ret
//	0x1013  90                    nop
//	0x1014  CC                    int3
var testCode = []byte{
	0x48, 0x89, 0xD8,
	0x48, 0x8B, 0x43, 0x08,
	0x0F, 0x84, 0x0A, 0x00, 0x00, 0x00,
	0xE8, 0x05, 0x00, 0x00, 0x00,
	0xC3,
	0x90,
	0xCC,
}

func TestSimpleDecode(t *testing.T) {
	loadForTest(t)
	code := []byte{0x48, 0x89, 0xD8} // mov rax,rbx
	d, err := NewDecoder(64, code, 0x1000)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	inst, ok := d.Decode()
	if !ok {
		t.Fatal("decode failed")
	}
	if got := inst.FormatIntel(); got != "mov rax,rbx" {
		t.Fatalf("format = %q, want %q", got, "mov rax,rbx")
	}
	if inst.Len() != 3 {
		t.Fatalf("len = %d, want 3", inst.Len())
	}
	if inst.IP() != 0x1000 {
		t.Fatalf("ip = %#x, want 0x1000", inst.IP())
	}
	if inst.NextIP() != 0x1003 {
		t.Fatalf("next ip = %#x, want 0x1003", inst.NextIP())
	}
	if inst.Code() == CodeInvalid {
		t.Fatal("code is INVALID")
	}
	if inst.FlowControl() != FlowControlNext {
		t.Fatalf("flow control = %v, want Next", inst.FlowControl())
	}
}

func TestLinearSequence(t *testing.T) {
	loadForTest(t)
	d, err := NewDecoder(64, testCode, 0x1000)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	wantIPs := []uint64{0x1000, 0x1003, 0x1007, 0x100D, 0x1012, 0x1013, 0x1014}
	wantLens := []int{3, 4, 6, 5, 1, 1, 1}
	n := 0
	for {
		inst, ok := d.Decode()
		if !ok {
			break
		}
		if n >= len(wantIPs) {
			t.Fatalf("decoded more than %d instructions", len(wantIPs))
		}
		if inst.IP() != wantIPs[n] {
			t.Fatalf("insn %d: ip = %#x, want %#x", n, inst.IP(), wantIPs[n])
		}
		if inst.Len() != wantLens[n] {
			t.Fatalf("insn %d: len = %d, want %d", n, inst.Len(), wantLens[n])
		}
		t.Logf("%X  %-24s len=%d flow=%v", inst.IP(), inst.FormatIntel(), inst.Len(), inst.FlowControl())
		n++
	}
	if n != len(wantIPs) {
		t.Fatalf("decoded %d instructions, want %d", n, len(wantIPs))
	}
	if d.Position() != uint64(len(testCode)) {
		t.Fatalf("position = %d, want %d", d.Position(), len(testCode))
	}
}

func TestBranchTargets(t *testing.T) {
	loadForTest(t)
	d, err := NewDecoder(64, testCode, 0x1000)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	type want struct {
		flow   FlowControl
		target uint64
	}
	wants := []want{
		{FlowControlNext, 0},
		{FlowControlNext, 0},
		{FlowControlConditionalBranch, 0x1017},
		{FlowControlCall, 0x1017},
		{FlowControlReturn, 0},
	}
	for idx, w := range wants {
		inst, ok := d.Decode()
		if !ok {
			t.Fatalf("insn %d: decode exhausted", idx)
		}
		if inst.FlowControl() != w.flow {
			t.Fatalf("insn %d (%s): flow = %v, want %v", idx, inst.FormatIntel(), inst.FlowControl(), w.flow)
		}
		if w.target != 0 && inst.NearBranchTarget() != w.target {
			t.Fatalf("insn %d (%s): target = %#x, want %#x", idx, inst.FormatIntel(), inst.NearBranchTarget(), w.target)
		}
	}
}

func TestInvalidBitness(t *testing.T) {
	loadForTest(t)
	if _, err := NewDecoder(128, testCode, 0); err == nil {
		t.Fatal("expected error for bitness=128")
	}
}

func TestEmptyCode(t *testing.T) {
	loadForTest(t)
	if _, err := NewDecoder(64, nil, 0); err == nil {
		t.Fatal("expected error for empty code")
	}
}

func TestTruncatedInstruction(t *testing.T) {
	loadForTest(t)
	code := []byte{0x0F, 0x84} // 截断的 jz near（缺 4 字节 rel32）
	d, err := NewDecoder(64, code, 0x2000)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	inst, ok := d.Decode()
	if !ok {
		t.Fatal("decode returned false, want INVALID instruction")
	}
	if inst.Code() != CodeInvalid {
		t.Fatalf("code = %d, want INVALID(0)", inst.Code())
	}
}

func TestFormatNoLeakSmoke(t *testing.T) {
	loadForTest(t)
	code := []byte{0x48, 0x89, 0xD8, 0x48, 0x8B, 0xC3}
	for i := 0; i < 20000; i++ {
		d, err := NewDecoder(64, code, 0)
		if err != nil {
			t.Fatal(err)
		}
		for {
			inst, ok := d.Decode()
			if !ok {
				break
			}
			_ = inst.FormatIntel()
		}
		d.Close()
	}
}
