package main

// 压测客户端：模拟 N 个玩家同时登录、每 200ms 上报一次位移，并统计收到的快照。
// 用途有两个：验证服务端能扛住并发；面试时能报出"多少在线、每秒多少快照"。

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9000", "server address")
	n := flag.Int("n", 50, "number of fake players")
	dur := flag.Duration("d", 15*time.Second, "duration")
	flag.Parse()

	var snaps, moves, errs int64
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < *n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			conn, err := net.Dial("tcp", *addr)
			if err != nil {
				atomic.AddInt64(&errs, 1)
				return
			}
			defer conn.Close()

			id := fmt.Sprintf("bot_%03d", i)
			send := func(v map[string]any) {
				b, _ := json.Marshal(v)
				b = append(b, '\n')
				conn.Write(b)
			}
			send(map[string]any{"type": "login", "playerId": id, "name": id})

			// 读协程：统计快照条数，模拟真实客户端每帧都要处理一次快照
			go func() {
				sc := bufio.NewScanner(conn)
				sc.Buffer(make([]byte, 4096), 1<<20)
				for sc.Scan() {
					if strings.Contains(string(sc.Bytes()), `"snapshot"`) {
						atomic.AddInt64(&snaps, 1)
					}
				}
			}()

			tk := time.NewTicker(200 * time.Millisecond)
			defer tk.Stop()
			for {
				select {
				case <-stop:
					send(map[string]any{"type": "logout"})
					return
				case <-tk.C:
					send(map[string]any{
						"type": "move",
						"x":    rand.Float64()*2 - 1,
						"y":    rand.Float64()*2 - 1,
					})
					atomic.AddInt64(&moves, 1)
				}
			}
		}(i)
	}

	start := time.Now()
	stats := time.NewTicker(time.Second)
	defer stats.Stop()
	deadline := time.After(*dur)

	for {
		select {
		case <-deadline:
			close(stop)
			time.Sleep(800 * time.Millisecond)
			elapsed := time.Since(start).Seconds()
			fmt.Fprintf(os.Stdout, "\n[RESULT] bots=%d duration=%.0fs conn_errors=%d\n", *n, elapsed, atomic.LoadInt64(&errs))
			fmt.Fprintf(os.Stdout, "[RESULT] snapshots_received=%.0f/s (total %d)\n",
				float64(atomic.LoadInt64(&snaps))/elapsed, atomic.LoadInt64(&snaps))
			fmt.Fprintf(os.Stdout, "[RESULT] move_reports=%.0f/s (total %d)\n",
				float64(atomic.LoadInt64(&moves))/elapsed, atomic.LoadInt64(&moves))
			wg.Wait()
			return
		case <-stats.C:
			elapsed := time.Since(start).Seconds()
			fmt.Printf("[%4.0fs] snapshots=%.0f/s moves=%.0f/s errors=%d\n",
				elapsed,
				float64(atomic.LoadInt64(&snaps))/elapsed,
				float64(atomic.LoadInt64(&moves))/elapsed,
				atomic.LoadInt64(&errs))
		}
	}
}
