// Package guac 实现 Guacamole 协议指令的编码、解析与按指令边界切分。
//
// 指令格式：LENGTH "." VALUE ("," LENGTH "." VALUE)* ";"
// 其中 LENGTH 为 VALUE 的 Unicode 码点数（不是字节数，也不是 UTF-16 单元数）。
// 第一个元素为 opcode；opcode 为空串的指令是隧道内部指令（如 "0.,4.ping,13.1700000000000;"）。
// https://guacamole.apache.org/doc/gug/protocol-reference.html
package guac

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// 单条指令最大字节数（guacd 自身限制为 8192 个码点 * 4 字节以内，这里留足余量）
	MaxInstructionBytes = 1 << 20
	// 单元素长度字段最大位数
	maxLengthDigits = 8
	// 单条指令最大元素数
	maxElements = 4096

	// 内部指令 opcode
	InternalOpcode = ""
)

var (
	ErrMalformed = errors.New("guacamole: 指令格式错误")
	ErrTooLarge  = errors.New("guacamole: 指令过长")
)

// 一条已解析的指令
type Instruction struct {
	Opcode string
	Args   []string
}

// 编码单个元素
func appendElement(b []byte, v string) []byte {
	b = strconv.AppendInt(b, int64(utf8.RuneCountInString(v)), 10)
	b = append(b, '.')
	return append(b, v...)
}

// 编码指令
func Encode(opcode string, args ...string) []byte {
	n := len(opcode) + 4
	for _, a := range args {
		n += len(a) + 6
	}
	b := make([]byte, 0, n)
	b = appendElement(b, opcode)
	for _, a := range args {
		b = append(b, ',')
		b = appendElement(b, a)
	}
	return append(b, ';')
}

func (i Instruction) Encode() []byte { return Encode(i.Opcode, i.Args...) }

func (i Instruction) String() string { return string(i.Encode()) }

// 从 b 的开头解析一条完整指令。
// 返回解析结果与消耗的字节数；数据不完整时返回 n==0 且 err==nil；格式错误返回 ErrMalformed。
func Parse(b []byte) (inst Instruction, n int, err error) {
	pos := 0
	first := true
	for {
		// 长度字段
		start := pos
		length := 0
		for {
			if pos >= len(b) {
				if pos-start > maxLengthDigits {
					return Instruction{}, 0, ErrMalformed
				}
				return Instruction{}, 0, nil
			}
			c := b[pos]
			if c == '.' {
				break
			}
			if c < '0' || c > '9' || pos-start >= maxLengthDigits {
				return Instruction{}, 0, ErrMalformed
			}
			length = length*10 + int(c-'0')
			pos++
		}
		if pos == start {
			return Instruction{}, 0, ErrMalformed
		}
		pos++ // '.'
		// 值：按码点前进 length 个字符
		vstart := pos
		for k := 0; k < length; k++ {
			if pos >= len(b) {
				if pos > MaxInstructionBytes {
					return Instruction{}, 0, ErrTooLarge
				}
				return Instruction{}, 0, nil
			}
			c := b[pos]
			if c < utf8.RuneSelf {
				pos++
				continue
			}
			if !utf8.FullRune(b[pos:]) {
				// 多字节字符被截断：若剩余字节已不可能构成合法字符则为格式错误，否则等待更多数据
				if len(b)-pos >= utf8.UTFMax {
					return Instruction{}, 0, ErrMalformed
				}
				return Instruction{}, 0, nil
			}
			r, size := utf8.DecodeRune(b[pos:])
			if r == utf8.RuneError && size <= 1 {
				return Instruction{}, 0, ErrMalformed
			}
			pos += size
		}
		if pos > MaxInstructionBytes {
			return Instruction{}, 0, ErrTooLarge
		}
		v := string(b[vstart:pos])
		if first {
			inst.Opcode = v
			first = false
		} else {
			if len(inst.Args) >= maxElements {
				return Instruction{}, 0, ErrTooLarge
			}
			inst.Args = append(inst.Args, v)
		}
		// 终止符
		if pos >= len(b) {
			return Instruction{}, 0, nil
		}
		switch b[pos] {
		case ',':
			pos++
		case ';':
			return inst, pos + 1, nil
		default:
			return Instruction{}, 0, ErrMalformed
		}
	}
}

// 解析一段完整的指令序列（如浏览器发来的一个 WebSocket 文本帧）。
// 返回每条指令及其原始字节切片；末尾存在不完整指令视为错误。
func ParseAll(b []byte) ([]Instruction, [][]byte, error) {
	var insts []Instruction
	var raws [][]byte
	for len(b) > 0 {
		inst, n, err := Parse(b)
		if err != nil {
			return nil, nil, err
		}
		if n == 0 {
			if len(b) > MaxInstructionBytes {
				return nil, nil, ErrTooLarge
			}
			return nil, nil, ErrMalformed
		}
		insts = append(insts, inst)
		raws = append(raws, b[:n])
		b = b[n:]
	}
	return insts, raws, nil
}

// 从字节流中按指令边界读取。
// 每次 ReadBatch 返回缓冲区中当前所有完整指令（原始字节，可直接作为一个 WebSocket 文本帧转发），
// 不完整的尾部留待下次；同时返回解析后的指令供调用方检查（如 error / clipboard）。
type Reader struct {
	r   io.Reader
	buf []byte
	// 已解析但尚未返回的位置
	scan int
	max  int
}

// batch 为单次返回的最大字节数（不会拆断指令；单条超过 batch 的指令单独返回）
func NewReader(r io.Reader, batch int) *Reader {
	if batch <= 0 {
		batch = 64 << 10
	}
	return &Reader{r: r, buf: make([]byte, 0, batch*2), max: batch}
}

// 读取下一批完整指令。返回的 raw 在下次调用前有效。
func (rd *Reader) ReadBatch() (raw []byte, insts []Instruction, err error) {
	for {
		// 先尝试从已有数据中切出完整指令
		pos := 0
		for pos < len(rd.buf) {
			inst, n, perr := Parse(rd.buf[pos:])
			if perr != nil {
				return nil, nil, perr
			}
			if n == 0 {
				break
			}
			insts = append(insts, inst)
			pos += n
			if pos >= rd.max {
				break
			}
		}
		if pos > 0 {
			out := make([]byte, pos)
			copy(out, rd.buf[:pos])
			rest := copy(rd.buf, rd.buf[pos:])
			rd.buf = rd.buf[:rest]
			return out, insts, nil
		}
		if len(rd.buf) > MaxInstructionBytes {
			return nil, nil, ErrTooLarge
		}
		// 读取更多数据
		if cap(rd.buf)-len(rd.buf) < 4096 {
			nb := make([]byte, len(rd.buf), cap(rd.buf)*2)
			copy(nb, rd.buf)
			rd.buf = nb
		}
		n, rerr := rd.r.Read(rd.buf[len(rd.buf):cap(rd.buf)])
		rd.buf = rd.buf[:len(rd.buf)+n]
		if rerr != nil {
			if n > 0 {
				// 先处理已读到的数据，错误留待下次
				continue
			}
			if rerr == io.EOF && len(rd.buf) > 0 {
				return nil, nil, io.ErrUnexpectedEOF
			}
			return nil, nil, rerr
		}
	}
}

// 读取单条指令（握手阶段使用）
func (rd *Reader) ReadInstruction() (Instruction, error) {
	for {
		if len(rd.buf) > 0 {
			inst, n, err := Parse(rd.buf)
			if err != nil {
				return Instruction{}, err
			}
			if n > 0 {
				rest := copy(rd.buf, rd.buf[n:])
				rd.buf = rd.buf[:rest]
				return inst, nil
			}
		}
		if len(rd.buf) > MaxInstructionBytes {
			return Instruction{}, ErrTooLarge
		}
		if cap(rd.buf)-len(rd.buf) < 4096 {
			nb := make([]byte, len(rd.buf), cap(rd.buf)*2+4096)
			copy(nb, rd.buf)
			rd.buf = nb
		}
		n, err := rd.r.Read(rd.buf[len(rd.buf):cap(rd.buf)])
		rd.buf = rd.buf[:len(rd.buf)+n]
		if err != nil && n == 0 {
			if err == io.EOF && len(rd.buf) > 0 {
				return Instruction{}, io.ErrUnexpectedEOF
			}
			return Instruction{}, err
		}
	}
}

// 是否为合法的 mimetype（握手时由浏览器提供的 audio/video/image 列表）
func ValidMimetype(s string) bool {
	if s == "" || len(s) > 64 || !strings.Contains(s, "/") {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || bytes.IndexByte([]byte("/.+-;=_ "), c) >= 0
		if !ok {
			return false
		}
	}
	return true
}
