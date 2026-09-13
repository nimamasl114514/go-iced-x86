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

