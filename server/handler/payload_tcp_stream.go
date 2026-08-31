package handler

import (
	"sort"
	"sync"
	"time"
)

const (
	tcpStreamMaxBytes   = 4 * 1024
	tcpStreamMaxTotal   = 1 * 1024 * 1024
	tcpStreamMaxCount   = 4096
	tcpStreamIdleExpiry = 10 * time.Second
)

type tcpStreamKey struct {
	Username  string
	GroupName string
	Protocol  uint8
	Src       string
	Dst       string
	SrcPort   uint16
	DstPort   uint16
}

type tcpStreamSegment struct {
	seq  uint32
	data []byte
}

type tcpStream struct {
	segments map[uint32][]byte
	baseSeq  uint32
	bytes    int
	lastSeen time.Time
}

type tcpStreamCache struct {
	mu      sync.Mutex
	streams map[tcpStreamKey]*tcpStream
	total   int
}

func newTCPStreamCache() *tcpStreamCache {
	return &tcpStreamCache{streams: make(map[tcpStreamKey]*tcpStream)}
}

// add 收集 TCP 应用层数据，并在已有连续数据可识别时返回应用协议。
func (c *tcpStreamCache) add(key tcpStreamKey, tcpSeg []byte, now time.Time) (uint8, string, bool) {
	if len(tcpSeg) < 20 {
		return acc_proto_tcp, "", false
	}
	headerLen := int(tcpSeg[12]>>4) << 2
	if headerLen < 20 || headerLen > len(tcpSeg) {
		return acc_proto_tcp, "", false
	}
	payload := tcpSeg[headerLen:]
	if len(payload) == 0 {
		return acc_proto_tcp, "", false
	}
	if len(payload) > tcpStreamMaxBytes {
		payload = payload[:tcpStreamMaxBytes]
	}
	seq := uint32(tcpSeg[4])<<24 | uint32(tcpSeg[5])<<16 | uint32(tcpSeg[6])<<8 | uint32(tcpSeg[7])

	c.mu.Lock()
	c.expireLocked(now)
	stream := c.streams[key]
	if stream == nil {
		if len(c.streams) >= tcpStreamMaxCount {
			c.evictOldestLocked()
		}
		stream = &tcpStream{segments: make(map[uint32][]byte), baseSeq: seq}
		c.streams[key] = stream
	}
	stream.lastSeen = now
	if relativeSeq(seq, stream.baseSeq) > 1<<31 {
		stream.baseSeq = seq
	}
	data := append([]byte(nil), payload...)
	if old, exists := stream.segments[seq]; exists {
		if len(old) >= len(data) {
			data = nil
		} else {
			stream.segments[seq] = data
			stream.bytes += len(data) - len(old)
			c.total += len(data) - len(old)
		}
	} else {
		stream.segments[seq] = data
		stream.bytes += len(data)
		c.total += len(data)
	}
	c.trimLocked(key, stream)
	assembled := assembleTCPStream(stream.segments, stream.baseSeq)
	c.mu.Unlock()

	if len(assembled) == 0 {
		return acc_proto_tcp, "", false
	}
	proto, info := onTCPPayload(assembled)
	if proto != acc_proto_tcp {
		c.mu.Lock()
		c.removeLocked(key)
		c.mu.Unlock()
		return proto, info, true
	}
	return acc_proto_tcp, "", false
}

func onTCPPayload(data []byte) (uint8, string) {
	for _, parser := range tcpParsers {
		if proto, info := parser(data); proto != acc_proto_tcp {
			return proto, info
		}
	}
	return acc_proto_tcp, ""
}

func relativeSeq(seq, base uint32) uint32 {
	return seq - base
}

func assembleTCPStream(segments map[uint32][]byte, baseSeq uint32) []byte {
	ordered := make([]tcpStreamSegment, 0, len(segments))
	for seq, data := range segments {
		ordered = append(ordered, tcpStreamSegment{seq: seq, data: data})
	}
	sort.Slice(ordered, func(i, j int) bool {
		return relativeSeq(ordered[i].seq, baseSeq) < relativeSeq(ordered[j].seq, baseSeq)
	})
	if len(ordered) == 0 {
		return nil
	}
	result := append([]byte(nil), ordered[0].data...)
	end := relativeSeq(ordered[0].seq, baseSeq) + uint32(len(result))
	for _, segment := range ordered[1:] {
		segmentSeq := relativeSeq(segment.seq, baseSeq)
		if segmentSeq > end {
			break
		}
		overlap := int(end - segmentSeq)
		if overlap < len(segment.data) {
			result = append(result, segment.data[overlap:]...)
			end += uint32(len(segment.data) - overlap)
		}
	}
	return result
}

func (c *tcpStreamCache) trimLocked(key tcpStreamKey, stream *tcpStream) {
	for stream.bytes > tcpStreamMaxBytes {
		var oldest uint32
		found := false
		for seq := range stream.segments {
			if !found || relativeSeq(seq, stream.baseSeq) < relativeSeq(oldest, stream.baseSeq) {
				oldest, found = seq, true
			}
		}
		if !found {
			break
		}
		stream.bytes -= len(stream.segments[oldest])
		c.total -= len(stream.segments[oldest])
		delete(stream.segments, oldest)
	}
	for c.total > tcpStreamMaxTotal {
		var oldKey tcpStreamKey
		var oldTime time.Time
		found := false
		for candidate, candidateStream := range c.streams {
			if candidate == key && len(c.streams) > 1 {
				continue
			}
			if !found || candidateStream.lastSeen.Before(oldTime) {
				oldKey, oldTime, found = candidate, candidateStream.lastSeen, true
			}
		}
		if !found {
			break
		}
		c.removeLocked(oldKey)
	}
}

func (c *tcpStreamCache) expireLocked(now time.Time) {
	for key, stream := range c.streams {
		if now.Sub(stream.lastSeen) >= tcpStreamIdleExpiry {
			c.removeLocked(key)
		}
	}
}

func (c *tcpStreamCache) cleanup(now time.Time) {
	c.mu.Lock()
	c.expireLocked(now)
	c.mu.Unlock()
}

func (c *tcpStreamCache) evictOldestLocked() {
	var oldKey tcpStreamKey
	var oldTime time.Time
	found := false
	for key, stream := range c.streams {
		if !found || stream.lastSeen.Before(oldTime) {
			oldKey, oldTime, found = key, stream.lastSeen, true
		}
	}
	if found {
		c.removeLocked(oldKey)
	}
}

func (c *tcpStreamCache) close(key tcpStreamKey) {
	c.mu.Lock()
	c.removeLocked(key)
	c.removeLocked(reverseTCPStreamKey(key))
	c.mu.Unlock()
}

func reverseTCPStreamKey(key tcpStreamKey) tcpStreamKey {
	key.Src, key.Dst = key.Dst, key.Src
	key.SrcPort, key.DstPort = key.DstPort, key.SrcPort
	return key
}

func (c *tcpStreamCache) removeLocked(key tcpStreamKey) {
	if stream := c.streams[key]; stream != nil {
		c.total -= stream.bytes
		delete(c.streams, key)
	}
}
