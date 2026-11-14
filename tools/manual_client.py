#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""手动试玩客户端：连上服务端，登录，随机走动，实时打印收到的快照与聊天。

用法：
    python tools/manual_client.py                      # 默认 127.0.0.1:9000
    python tools/manual_client.py -addr 127.0.0.1:9000 -id demo01

按 Ctrl+C 退出（退出时会发 logout，服务端把状态落盘）。
"""

import argparse
import json
import random
import socket
import sys
import threading
import time


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("-addr", default="127.0.0.1:9000", help="服务端地址 host:port")
    ap.add_argument("-id", dest="pid", default="demo01", help="玩家 ID")
    ap.add_argument("-name", default="演示玩家", help="昵称")
    args = ap.parse_args()

    host, port = args.addr.rsplit(":", 1)
    sock = socket.create_connection((host, int(port)), timeout=5)
    sock.settimeout(None)

    def send(payload: dict) -> None:
        sock.sendall(json.dumps(payload).encode("utf-8") + b"\n")

    send({"type": "login", "playerId": args.pid, "name": args.name})

    stop = threading.Event()

    def reader() -> None:
        buf = b""
        last_print = 0.0
        while not stop.is_set():
            try:
                chunk = sock.recv(65536)
            except OSError:
                break
            if not chunk:
                break
            buf += chunk
            while b"\n" in buf:
                line, buf = buf.split(b"\n", 1)
                if not line:
                    continue
                try:
                    msg = json.loads(line)
                except json.JSONDecodeError:
                    continue
                t = msg.get("type")
                if t == "login_ok":
                    r = msg.get("restored")
                    tag = "（读到旧存档）" if r else "（新号）"
                    print(f"登录成功 {msg.get('playerId')} {tag}")
                    if r:
                        print(f"  还原位置 x={r['x']:.1f} y={r['y']:.1f}")
                elif t == "snapshot":
                    now = time.time()
                    if now - last_print >= 1.0:  # 每秒只打印一次，别刷屏
                        last_print = now
                        total = len(msg.get("players") or [])
                        preview = ", ".join(
                            f"{p['name']}({p['x']:.0f},{p['y']:.0f})"
                            for p in (msg.get("players") or [])[:4]
                        )
                        print(f"[tick {msg.get('tick')}] 在线 {total} 人：{preview}")
                elif t == "chat":
                    print(f"聊天 {msg.get('from')}: {msg.get('text')}")
                elif t == "error":
                    print(f"服务端错误：{msg.get('msg')}")

    th = threading.Thread(target=reader, daemon=True)
    th.start()

    try:
        while True:
            send({"type": "move", "x": random.uniform(-2, 2), "y": random.uniform(-2, 2)})
            time.sleep(0.2)
    except KeyboardInterrupt:
        print("\n退出中...")
        try:
            send({"type": "logout"})
        except OSError:
            pass
    finally:
        stop.set()
        sock.close()
        th.join(timeout=1)


if __name__ == "__main__":
    sys.exit(main())
