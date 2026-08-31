package sessdata

import (
	"fmt"
	"testing"
	"time"
)

func TestLimitAccuracy(t *testing.T) {
	// 1 Mbps = 125000 Byte/s, burst=1500
	rate := 125000
	limit := NewLimitRater(rate)

	totalBytes := 1460 * 100 // 146000 字节
	start := time.Now()
	for range 100 {
		if err := limit.Wait(1460); err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)

	// 理论耗时 = (totalBytes - burst) / rate = (146000 - 1500) / 125000 ≈ 1.156s
	expectedMin := time.Duration(float64(totalBytes-1500)/float64(rate)*float64(time.Second)) - 100*time.Millisecond
	fmt.Printf("传输 %d 字节, 耗时 %v, 实际速率 %.0f Byte/s (理论 %d Byte/s)\n",
		totalBytes, elapsed, float64(totalBytes)/elapsed.Seconds(), rate)

	if elapsed < expectedMin {
		t.Errorf("限速失效: 耗时 %v 小于预期 %v", elapsed, expectedMin)
	}
}

func TestBurstEffect(t *testing.T) {
	rate := 125000
	limit := NewLimitRater(rate)

	// 第一次传 burst 大小，应立即返回
	start := time.Now()
	if err := limit.Wait(1500); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("第一次传 1500 字节(burst), 耗时 %v\n", time.Since(start))

	// 第二次传 1500 字节，应等待 1500/125000 ≈ 12ms
	start = time.Now()
	if err := limit.Wait(1500); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	fmt.Printf("第二次传 1500 字节, 耗时 %v\n", elapsed)
	if elapsed < 10*time.Millisecond {
		t.Errorf("burst 未生效: 耗时 %v，预期至少 12ms", elapsed)
	}
}

func TestLargePacket(t *testing.T) {
	// 5000 字节超过 burst=1500，应分批等待
	rate := 125000
	limit := NewLimitRater(rate)

	start := time.Now()
	if err := limit.Wait(5000); err != nil {
		t.Fatalf("大包处理失败: %v", err)
	}
	elapsed := time.Since(start)
	// 理论耗时 = (5000 - 1500) / 125000 ≈ 28ms
	fmt.Printf("传 5000 字节(超burst), 耗时 %v\n", elapsed)
	if elapsed < 20*time.Millisecond {
		t.Errorf("大包分批等待失效: 耗时 %v，预期至少 28ms", elapsed)
	}
}
