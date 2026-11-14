package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"time"
)

func main() {
	addr := flag.String("addr", ":9000", "TCP 监听地址")
	statsAddr := flag.String("http", ":9001", "HTTP 统计地址，置空则关闭")
	dataFile := flag.String("data", "players.json", "玩家存档文件")
	tick := flag.Duration("tick", 50*time.Millisecond, "快照频率，50ms = 20Hz")
	flag.Parse()

	store := NewFileStore(*dataFile)
	srv := NewServer(store, *tick)

	if err := srv.Start(*addr); err != nil {
		log.Fatalf("listen %s failed: %v", *addr, err)
	}
	if *statsAddr != "" {
		go srv.StartHTTP(*statsAddr)
	}
	log.Printf("game server ready: tcp=%s tick=%s data=%s", *addr, *tick, *dataFile)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt) // Ctrl+C / kill -INT，两个平台都可用
	<-sig

	log.Printf("shutting down: flushing online players to %s ...", *dataFile)
	srv.Shutdown()
	log.Printf("shutdown complete")
}
