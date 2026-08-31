package handler

import (
	"testing"
	"time"
)

func makeTCPStreamSegment(seq uint32, payload string) []byte {
	segment := make([]byte, 20+len(payload))
	segment[4] = byte(seq >> 24)
	segment[5] = byte(seq >> 16)
	segment[6] = byte(seq >> 8)
	segment[7] = byte(seq)
	segment[12] = 5 << 4
	copy(segment[20:], payload)
	return segment
}

func TestSNIParserWaitsForCompleteClientHello(t *testing.T) {
	body := make([]byte, 0, 64)
	body = append(body, 0x03, 0x03)
	body = append(body, make([]byte, 32)...)
	body = append(body, 0) // session id length
	body = append(body, 0, 2, 0, 0x2f)
	body = append(body, 1, 0) // compression methods
	name := []byte("example.com")
	ext := make([]byte, 0, len(name)+9)
	ext = append(ext, 0, 0, 0, byte(len(name)+5), 0, byte(len(name)+3), 0, 0, byte(len(name)))
	ext = append(ext, name...)
	body = append(body, byte(len(ext)>>8), byte(len(ext)))
	body = append(body, ext...)
	record := []byte{0x16, 0x03, 0x03, byte((4 + len(body)) >> 8), byte(4 + len(body)), 1, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	record = append(record, body...)
	for _, cut := range []int{1, 8, len(record) - 1} {
		if proto, info := sniNewParser(record[:cut]); proto != acc_proto_tcp || info != "" {
			t.Fatalf("partial ClientHello at %d = (%d, %q), want incomplete TCP", cut, proto, info)
		}
	}
	if proto, info := sniNewParser(record); proto != acc_proto_https || info != "example.com" {
		t.Fatalf("complete ClientHello = (%d, %q), want HTTPS example.com", proto, info)
	}
}

func TestTCPStreamCacheReassemblesHTTPOutOfOrder(t *testing.T) {
	cache := newTCPStreamCache()
	key := tcpStreamKey{Username: "user", Src: "10.0.0.2", Dst: "10.0.0.3", SrcPort: 40000, DstPort: 80}
	first := "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
	cut := 12
	now := time.Unix(100, 0)

	if proto, info, ok := cache.add(key, makeTCPStreamSegment(100+uint32(cut), first[cut:]), now); ok || proto != acc_proto_tcp || info != "" {
		t.Fatalf("out-of-order tail result = (%d, %q, %v)", proto, info, ok)
	}
	proto, info, ok := cache.add(key, makeTCPStreamSegment(100, first[:cut]), now.Add(time.Millisecond))
	if !ok || proto != acc_proto_http || info != "example.com" {
		t.Fatalf("reassembled result = (%d, %q, %v), want HTTP example.com", proto, info, ok)
	}
	if len(cache.streams) != 0 || cache.total != 0 {
		t.Fatalf("recognized stream was not released: streams=%d total=%d", len(cache.streams), cache.total)
	}
}

func TestTCPStreamCacheReassemblesSequenceWrap(t *testing.T) {
	cache := newTCPStreamCache()
	key := tcpStreamKey{Src: "10.0.0.2", Dst: "10.0.0.3", SrcPort: 40000, DstPort: 80}
	first := "GET / HTTP/1.1\r\nHost: wrap.example\r\n\r\n"
	cut := 12
	now := time.Unix(150, 0)
	if _, _, ok := cache.add(key, makeTCPStreamSegment(^uint32(0)-uint32(cut)+1, first[cut:]), now); ok {
		t.Fatal("out-of-order wrapped tail should not identify")
	}
	proto, info, ok := cache.add(key, makeTCPStreamSegment(^uint32(0)-uint32(cut)+1-uint32(cut), first[:cut]), now.Add(time.Millisecond))
	if !ok || proto != acc_proto_http || info != "wrap.example" {
		t.Fatalf("wrapped stream result = (%d, %q, %v)", proto, info, ok)
	}
}

func TestTCPStreamCacheKeepsLongerRetransmission(t *testing.T) {
	cache := newTCPStreamCache()
	key := tcpStreamKey{Src: "10.0.0.2", Dst: "10.0.0.3", SrcPort: 40000, DstPort: 80}
	now := time.Unix(180, 0)
	cache.add(key, makeTCPStreamSegment(100, "AAAA"), now)
	cache.add(key, makeTCPStreamSegment(100, "AAAAAA"), now.Add(time.Millisecond))
	assembled := assembleTCPStream(cache.streams[key].segments, cache.streams[key].baseSeq)
	if string(assembled) != "AAAAAA" {
		t.Fatalf("assembled retransmission = %q", assembled)
	}
}

func TestTCPStreamCacheIgnoresDuplicateAndExpires(t *testing.T) {
	cache := newTCPStreamCache()
	key := tcpStreamKey{Src: "10.0.0.2", Dst: "10.0.0.3", SrcPort: 40000, DstPort: 443}
	segment := makeTCPStreamSegment(100, "not a complete application message")
	now := time.Unix(200, 0)
	cache.add(key, segment, now)
	cache.add(key, segment, now.Add(time.Second))
	if cache.total != len(segment)-20 {
		t.Fatalf("duplicate segment increased cache: total=%d", cache.total)
	}
	cache.add(key, makeTCPStreamSegment(200, "next"), now.Add(tcpStreamIdleExpiry+time.Second))
	if len(cache.streams) != 1 || cache.total != 4 {
		t.Fatalf("expired stream was not removed: streams=%d total=%d", len(cache.streams), cache.total)
	}
}

func TestTCPStreamCacheCapsSingleStream(t *testing.T) {
	cache := newTCPStreamCache()
	key := tcpStreamKey{Src: "10.0.0.2", Dst: "10.0.0.3", SrcPort: 40000, DstPort: 80}
	cache.add(key, makeTCPStreamSegment(100, string(make([]byte, tcpStreamMaxBytes+512))), time.Unix(300, 0))
	if cache.total > tcpStreamMaxBytes {
		t.Fatalf("single stream exceeded limit: total=%d", cache.total)
	}
}

func TestTCPStreamCacheCloseRemovesBothDirections(t *testing.T) {
	cache := newTCPStreamCache()
	key := tcpStreamKey{Username: "user", Src: "10.0.0.2", Dst: "10.0.0.3", SrcPort: 40000, DstPort: 443}
	reverse := reverseTCPStreamKey(key)
	now := time.Unix(400, 0)
	cache.add(key, makeTCPStreamSegment(100, "client"), now)
	cache.add(reverse, makeTCPStreamSegment(200, "server"), now)
	cache.close(key)
	if len(cache.streams) != 0 || cache.total != 0 {
		t.Fatalf("closed stream directions were not released: streams=%d total=%d", len(cache.streams), cache.total)
	}
}
