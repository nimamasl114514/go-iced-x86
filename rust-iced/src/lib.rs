//! iced-go-ffi: iced-x86 的 C ABI 导出层。
//!
//! 设计要点：
//! - Decoder 为堆上不透明指针（Box::into_raw），调用方仅持有指针，生命周期由本层管理。
//! - Instruction 为固定大小 POD（Copy 类型），由调用方分配内存（尺寸经 iced_instruction_size()
//!   运行时获取），本层按值写入，规避 purego 在 Linux/Windows 不支持 struct 值传递的限制。
//! - 字符串遵循「谁分配谁释放」：format_intel 返回的 *mut c_char 必须交还 iced_string_free。
//! - 一切可能 panic 的入口（非法 bitness、空指针）在 FFI 边界前置拦截，panic 不可跨 FFI。

use iced_x86::{Decoder, DecoderOptions, Formatter, Instruction, IntelFormatter};
use std::ffi::CString;
use std::os::raw::{c_char, c_int};

/// 创建 Decoder，返回堆上裸指针；失败（空指针/空数据/非法 bitness）返回 null。
/// 注意：Decoder 借用 bytes 指向的内存，调用方必须保证该内存在 decoder 存活期间有效
/// （Go 侧由 Decoder 结构体持有 code 切片引用保活）。
#[no_mangle]
pub extern "C" fn iced_decoder_new(
    bitness: c_int,
    bytes: *const u8,
    len: usize,
    ip: u64,
) -> *mut Decoder<'static> {
    if bytes.is_null() || len == 0 {
        return std::ptr::null_mut();
    }
    if bitness != 16 && bitness != 32 && bitness != 64 {
        return std::ptr::null_mut();
    }
    let slice: &'static [u8] = unsafe { std::slice::from_raw_parts(bytes, len) };
    let decoder = Decoder::with_ip(bitness as u32, slice, ip, DecoderOptions::NONE);
    Box::into_raw(Box::new(decoder))
}

/// 释放 Decoder。
#[no_mangle]
pub extern "C" fn iced_decoder_free(decoder: *mut Decoder<'static>) {
    if !decoder.is_null() {
        unsafe { drop(Box::from_raw(decoder)); }
    }
}

/// 解码一条指令，按值写入 *instruction（调用方预分配的 iced_instruction_size() 字节）。
/// 返回 1 = 成功；0 = 无可解码数据或参数为空。decode() 本身不 panic（坏字节产出 INVALID 指令）。
#[no_mangle]
pub extern "C" fn iced_decode(decoder: *mut Decoder<'static>, instruction: *mut Instruction) -> c_int {
    if decoder.is_null() || instruction.is_null() {
        return 0;
    }
    let decoder = unsafe { &mut *decoder };
    if !decoder.can_decode() {
        return 0;
    }
    let out = unsafe { &mut *instruction };
    *out = decoder.decode();
    1
}

/// 是否还有数据可解码（1/0）。
#[no_mangle]
pub extern "C" fn iced_decoder_can_decode(decoder: *const Decoder<'static>) -> c_int {
    if decoder.is_null() {
        return 0;
    }
    let decoder = unsafe { &*decoder };
    if decoder.can_decode() { 1 } else { 0 }
}

/// 当前解码位置（已消耗字节数）。
#[no_mangle]
pub extern "C" fn iced_decoder_position(decoder: *const Decoder<'static>) -> u64 {
    if decoder.is_null() {
        return 0;
    }
    let decoder = unsafe { &*decoder };
    decoder.position() as u64
}

/// Instruction 结构体大小（字节），Go 侧分配内存的依据。
#[no_mangle]
pub extern "C" fn iced_instruction_size() -> usize {
    std::mem::size_of::<Instruction>()
}

/// Instruction 结构体对齐要求（字节），Go 侧分配内存的对齐依据。
#[no_mangle]
pub extern "C" fn iced_instruction_align() -> usize {
    std::mem::align_of::<Instruction>()
}

/// 指令长度（字节）。
#[no_mangle]
pub extern "C" fn iced_instruction_len(instruction: *const Instruction) -> usize {
    let inst = unsafe { &*instruction };
    inst.len()
}

/// 指令地址（IP）。
#[no_mangle]
pub extern "C" fn iced_instruction_ip(instruction: *const Instruction) -> u64 {
    let inst = unsafe { &*instruction };
    inst.ip()
}

/// 下一条指令地址（IP + 长度）。
#[no_mangle]
pub extern "C" fn iced_instruction_next_ip(instruction: *const Instruction) -> u64 {
    let inst = unsafe { &*instruction };
    inst.next_ip()
}

/// Code 枚举值（指令精确标识，如 Mov_rm64_r64）。
#[no_mangle]
pub extern "C" fn iced_instruction_code(instruction: *const Instruction) -> u32 {
    let inst = unsafe { &*instruction };
    inst.code() as u32
}

/// Mnemonic 枚举值（助记符类别，如 Mov）。
#[no_mangle]
pub extern "C" fn iced_instruction_mnemonic(instruction: *const Instruction) -> u32 {
    let inst = unsafe { &*instruction };
    inst.mnemonic() as u32
}

/// FlowControl 枚举值（控制流类别：Next/ConditionalBranch/Call/Return 等）。
#[no_mangle]
pub extern "C" fn iced_instruction_flow_control(instruction: *const Instruction) -> u32 {
    let inst = unsafe { &*instruction };
    inst.flow_control() as u32
}

/// 近分支目标地址（仅对 near branch/call 类指令有意义）。
#[no_mangle]
pub extern "C" fn iced_instruction_near_branch_target(instruction: *const Instruction) -> u64 {
    let inst = unsafe { &*instruction };
    inst.near_branch_target()
}

/// 格式化为 Intel 语法字符串。返回堆上 C 字符串，调用方须用 iced_string_free 释放；
/// 失败返回 null。
#[no_mangle]
pub extern "C" fn iced_format_intel(instruction: *const Instruction) -> *mut c_char {
    if instruction.is_null() {
        return std::ptr::null_mut();
    }
    let inst = unsafe { &*instruction };
    let mut formatter = IntelFormatter::new();
    let mut output = String::new();
    formatter.format(inst, &mut output);
    match CString::new(output) {
        Ok(s) => s.into_raw(),
        Err(_) => std::ptr::null_mut(),
    }
}

/// 释放 iced_format_intel 返回的字符串。
#[no_mangle]
pub extern "C" fn iced_string_free(s: *mut c_char) {
    if !s.is_null() {
        unsafe { drop(CString::from_raw(s)); }
    }
}


// ============================================================================
// 扩展 API：多语法格式化 / 批量解码 / Code 常量表
// ============================================================================

use iced_x86::{Code, GasFormatter, MasmFormatter, NasmFormatter};
use std::convert::TryFrom;

/// 字符串装箱辅助：String -> 堆上 C 字符串（内嵌 NUL 时失败返回 null）。
fn cstr(s: String) -> *mut c_char {
    match CString::new(s) {
        Ok(s) => s.into_raw(),
        Err(_) => std::ptr::null_mut(),
    }
}

/// 按位掩码应用格式化选项（位定义见 iced_format 文档注释）。
fn apply_opts<F: Formatter>(f: &mut F, opts: u32) {
    let o = f.options_mut();
    if opts & (1 << 0) != 0 { o.set_space_after_operand_separator(true); }
    if opts & (1 << 1) != 0 { o.set_space_after_memory_bracket(true); }
    if opts & (1 << 2) != 0 { o.set_uppercase_all(true); }
    if opts & (1 << 3) != 0 { o.set_show_zero_displacements(true); }
    if opts & (1 << 4) != 0 { o.set_uppercase_hex(true); }
    if opts & (1 << 5) != 0 { o.set_always_show_segment_register(true); }
}

/// 多语法格式化。syntax: 0=Intel 1=Gas(AT&T) 2=Masm 3=Nasm；opts 位掩码：
/// bit0 操作数分隔符后空格 / bit1 内存括号后空格 / bit2 全大写 /
/// bit3 显示零位移 / bit4 十六进制大写 / bit5 总显示段寄存器。
/// 返回堆上 C 字符串（调用方用 iced_string_free 释放）；非法参数返回 null。
#[no_mangle]
pub extern "C" fn iced_format(instruction: *const Instruction, syntax: c_int, opts: u32) -> *mut c_char {
    if instruction.is_null() {
        return std::ptr::null_mut();
    }
    let inst = unsafe { &*instruction };
    let mut output = String::new();
    macro_rules! fmt_with {
        ($f:expr) => {{
            let mut f = $f;
            apply_opts(&mut f, opts);
            f.format(inst, &mut output);
        }};
    }
    match syntax {
        0 => fmt_with!(IntelFormatter::new()),
        1 => fmt_with!(GasFormatter::new()),
        2 => fmt_with!(MasmFormatter::new()),
        3 => fmt_with!(NasmFormatter::new()),
        _ => return std::ptr::null_mut(),
    }
    cstr(output)
}

/// 批量解码：至多 max 条，按 stride 字节间隔写入 out
/// （stride 必须 >= iced_instruction_size()；out 无需对齐，内部 write_unaligned）。
/// 返回实际写入条数。
#[no_mangle]
pub extern "C" fn iced_decode_all(
    decoder: *mut Decoder<'static>,
    out: *mut u8,
    stride: usize,
    max: usize,
) -> usize {
    if decoder.is_null() || out.is_null() || max == 0 || stride < std::mem::size_of::<Instruction>() {
        return 0;
    }
    let decoder = unsafe { &mut *decoder };
    let mut n = 0usize;
    while n < max && decoder.can_decode() {
        let slot = unsafe { out.add(n * stride) as *mut Instruction };
        unsafe { std::ptr::write_unaligned(slot, decoder.decode()) };
        n += 1;
    }
    n
}

/// Code 枚举条目总数。
#[no_mangle]
pub extern "C" fn iced_code_count() -> u32 {
    Code::values().len() as u32
}

/// 按枚举序号取 Code：名称经返回值返回（调用方用 iced_string_free 释放），
/// 枚举值写入 *out_value（可为 null）。index 越界返回 null。
#[no_mangle]
pub extern "C" fn iced_code_at(index: u32, out_value: *mut u32) -> *mut c_char {
    match Code::values().nth(index as usize) {
        Some(c) => {
            if !out_value.is_null() {
                unsafe { *out_value = c as u32; }
            }
            cstr(format!("{:?}", c))
        }
        None => std::ptr::null_mut(),
    }
}

/// 按枚举值取 Code 名称（如 "Mov_rm64_r64"）；非法值返回 null。
#[no_mangle]
pub extern "C" fn iced_code_name(code: u32) -> *mut c_char {
    match Code::try_from(code as usize) {
        Ok(c) => cstr(format!("{:?}", c)),
        Err(_) => std::ptr::null_mut(),
    }
}
