package guac

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestEncodeCountsCodePoints(t *testing.T) {
	cases := map[string]string{
		string(Encode("size", "1024", "768", "96")): "4.size,4.1024,3.768,2.96;",
		string(Encode("", "ping", "1700000000000")): "0.,4.ping,13.1700000000000;",
		string(Encode("name", "测试中文")):              "4.name,4.测试中文;", // 4 个码点，12 字节
		string(Encode("x", "😀a")):                   "1.x,2.😀a;",      // emoji 为 1 个码点（UTF-16 下为 2 个单元）
		string(Encode("nop")):                       "3.nop;",
		string(Encode("a", "")):                     "1.a,0.;",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("Encode = %q, want %q", got, want)
		}
	}
}

func TestParseRoundtrip(t *testing.T) {
	insts := []Instruction{
		{Opcode: "select", Args: []string{"rdp"}},
		{Opcode: "", Args: []string{"ping", "123"}},
		{Opcode: "clipboard", Args: []string{"1", "text/plain"}},
		{Opcode: "blob", Args: []string{"1", "5L2g5aW9,;.;"}},             // 值中含 , ; . 等分隔符
		{Opcode: "name", Args: []string{"中文😀,;混合", "Ünïcödé"}},            // 多字节 + 分隔符
		{Opcode: "args", Args: []string{"VERSION_1_5_0", "hostname", ""}}, // 空值
	}
	var stream []byte
	for _, in := range insts {
		stream = append(stream, in.Encode()...)
	}
	got, raws, err := ParseAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(insts) {
		t.Fatalf("got %d instructions", len(got))
	}
	for i := range insts {
		if got[i].Opcode != insts[i].Opcode || !reflect.DeepEqual(got[i].Args, insts[i].Args) {
			t.Errorf("#%d got %+v want %+v", i, got[i], insts[i])
		}
		if string(raws[i]) != insts[i].String() {
			t.Errorf("#%d raw %q", i, raws[i])
		}
	}
}

func TestParseIncompleteAtEveryBoundary(t *testing.T) {
	full := Encode("name", "中文😀混合", "x")
	for i := 0; i < len(full); i++ {
		_, n, err := Parse(full[:i])
		if err != nil || n != 0 {
			t.Fatalf("prefix len %d: n=%d err=%v（不完整数据应等待更多输入）", i, n, err)
		}
	}
	inst, n, err := Parse(full)
	if err != nil || n != len(full) || inst.Args[0] != "中文😀混合" {
		t.Fatalf("full parse: %+v %d %v", inst, n, err)
	}
}

func TestParseLengthIsCodePointsNotBytes(t *testing.T) {
	// 按字节计数的错误编码：长度 6（"中文" 的字节数）会越过值，导致格式错误
	if _, _, err := ParseAll([]byte("4.name,6.中文;")); err == nil {
		t.Fatal("按字节计数的长度应判为格式错误")
	}
	// 正确：长度 2
	inst, n, err := Parse([]byte("4.name,2.中文;rest"))
	if err != nil || inst.Args[0] != "中文" || n != len("4.name,2.中文;") {
		t.Fatalf("got %+v %d %v", inst, n, err)
	}
	// 按 UTF-16 单元计数的 emoji（长度 2）在码点语义下会多吞一个字符
	inst, _, err = Parse([]byte("1.x,2.😀;;"))
	if err != nil || inst.Args[0] != "😀;" {
		t.Fatalf("码点计数语义错误: %+v %v", inst, err)
	}
}

func TestParseMalformed(t *testing.T) {
	bad := []string{
		"a.size;",          // 长度非数字
		".size;",           // 缺长度
		"4.size:",          // 错误终止符
		"4.sizeX;",         // 值后不是 , 或 ;
		"999999999.x;",     // 长度位数过多
		"3.ab\xff;",        // 非法 UTF-8
		"4.size,-1.x;",     // 负长度
		"4.size,1.a,1.b;;", // 末尾多余（ParseAll 中 ";" 不是合法指令）
	}
	for _, b := range bad {
		if _, _, err := ParseAll([]byte(b)); err == nil {
			t.Errorf("%q 应判为格式错误", b)
		}
	}
	// 不完整帧在 ParseAll 中为错误
	if _, _, err := ParseAll([]byte("4.size,4.10")); err == nil {
		t.Error("不完整帧应报错")
	}
}

// 每次只返回随机少量字节的读取器
type chunkReader struct {
	data []byte
	rng  *rand.Rand
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n := 1 + c.rng.Intn(7)
	if n > len(p) {
		n = len(p)
	}
	if n > len(c.data) {
		n = len(c.data)
	}
	copy(p, c.data[:n])
	c.data = c.data[n:]
	return n, nil
}

func TestReaderBatchesOnInstructionBoundaries(t *testing.T) {
	var want []Instruction
	var stream []byte
	for i := 0; i < 500; i++ {
		in := Instruction{Opcode: "blob", Args: []string{"3", strings.Repeat("中😀a", i%17)}}
		want = append(want, in)
		stream = append(stream, in.Encode()...)
	}
	rd := NewReader(&chunkReader{data: stream, rng: rand.New(rand.NewSource(1))}, 256)
	var got []Instruction
	var rejoined []byte
	for {
		raw, insts, err := rd.ReadBatch()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		// 每批都必须是完整的指令序列
		chk, _, perr := ParseAll(raw)
		if perr != nil || len(chk) != len(insts) {
			t.Fatalf("批次未按指令边界切分: %v", perr)
		}
		got = append(got, insts...)
		rejoined = append(rejoined, raw...)
	}
	if !bytes.Equal(rejoined, stream) {
		t.Fatal("转发内容与原始流不一致")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("解析结果不一致")
	}
}

func TestReaderTruncatedStream(t *testing.T) {
	rd := NewReader(bytes.NewReader([]byte("4.sync,3.123;4.sync,2.1")), 0)
	raw, _, err := rd.ReadBatch()
	if err != nil || string(raw) != "4.sync,3.123;" {
		t.Fatalf("first batch %q %v", raw, err)
	}
	if _, _, err := rd.ReadBatch(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("截断流应返回 ErrUnexpectedEOF: %v", err)
	}
}

func TestReadInstruction(t *testing.T) {
	rd := NewReader(&chunkReader{data: []byte("4.args,13.VERSION_1_5_0,8.hostname;5.ready,5.$abcd;"), rng: rand.New(rand.NewSource(2))}, 0)
	a, err := rd.ReadInstruction()
	if err != nil || a.Opcode != "args" || len(a.Args) != 2 || a.Args[1] != "hostname" {
		t.Fatalf("%+v %v", a, err)
	}
	r, err := rd.ReadInstruction()
	if err != nil || r.Opcode != "ready" || r.Args[0] != "$abcd" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestReaderTooLarge(t *testing.T) {
	huge := "9999999." + strings.Repeat("a", MaxInstructionBytes+10)
	rd := NewReader(strings.NewReader(huge), 0)
	if _, _, err := rd.ReadBatch(); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("超长指令应报错: %v", err)
	}
}

func TestValidMimetype(t *testing.T) {
	if !ValidMimetype("image/png") || !ValidMimetype("audio/L16;rate=44100") {
		t.Fatal("合法 mimetype 被拒绝")
	}
	for _, bad := range []string{"", "png", "image/png,1.x", "a/b;\x00", strings.Repeat("a", 65) + "/b", "image/png\";"} {
		if ValidMimetype(bad) {
			t.Errorf("%q 应拒绝", bad)
		}
	}
}
