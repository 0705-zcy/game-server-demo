"""
game-server-demo 存档层 SQL 最小闭环（SQLite 版，零第三方依赖）

对应代码：store.go 里的 Store 接口 / FileStore 实现
要解决的问题：现在存档是"每次 SaveAll 全量写一个 JSON 文件"，
读一个玩家要把整份档案读进来。换成一个玩家一行、一个主键查询，行为就对了。

跑法：
    python sql/demo_sqlite.py

输出：
    控制台打印每一步的 SQL 和真实返回行，同时写一份 sql/demo_output.txt 存档。
"""

import os
import sqlite3
import sys
import time
from datetime import datetime

HERE = os.path.dirname(os.path.abspath(__file__))
DB_PATH = os.path.join(HERE, "players.db")
OUT_PATH = os.path.join(HERE, "demo_output.txt")
SCHEMA_PATH = os.path.join(HERE, "schema_sqlite.sql")

lines = []


def log(s=""):
    lines.append(str(s))


def dump(cur, title):
    rows = cur.fetchall()
    log("  --- " + title + " ---")
    if not rows:
        log("  (无返回行)")
        log()
        return rows
    for r in rows:
        log("  " + " | ".join("" if c is None else str(c) for c in r))
    log()
    return rows


def ts(v):
    return datetime.fromtimestamp(v).strftime("%Y-%m-%d %H:%M")


def main():
    now = int(time.time())

    if os.path.exists(DB_PATH):
        os.remove(DB_PATH)

    conn = sqlite3.connect(DB_PATH)
    conn.execute("PRAGMA foreign_keys = ON")   # SQLite 默认不启用外键，必须显式打开
    cur = conn.cursor()

    log("=" * 78)
    log("game-server-demo 存档层 SQL 最小闭环（SQLite）")
    log("库文件        : " + DB_PATH)
    log("SQLite 版本   : " + sqlite3.sqlite_version)
    log("=" * 78)
    log()

    # ---------- 1 建表 ----------
    with open(SCHEMA_PATH, "r", encoding="utf-8") as f:
        conn.executescript(f.read())
    conn.commit()

    log("【1】建表：players（存档表）+ login_log（登录流水表）")
    cur.execute(
        "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
    )
    dump(cur, "表清单")
    cur.execute(
        "SELECT name FROM sqlite_master WHERE type='index' AND name NOT LIKE 'sqlite_%' ORDER BY name"
    )
    dump(cur, "索引清单")

    # ---------- 2 首次存档 ----------
    log("【2】首次存档：一条多行 INSERT 写入 3 个玩家")
    cur.executemany(
        "INSERT INTO players (id, name, x, y, hp, online, last_seen, created_at) "
        "VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
        [
            ("p1001", "阿雨", 12.5, 8.0, 100, 1, now, now),
            ("p1002", "小陈", 30.0, 44.0, 80, 0, now - 3600, now - 7200),
            ("p1003", "老王", 5.0, 5.0, 60, 0, now - 86400 * 40, now - 86400 * 45),
        ],
    )
    conn.commit()
    cur.execute("SELECT id, name, hp, online, last_seen FROM players ORDER BY id")
    dump(cur, "写入后（last_seen 存的是 unix 秒）")
    # 下面这轮把 unix 秒转成可读时间，方便肉眼核对
    cur.execute("SELECT id, name, last_seen FROM players ORDER BY id")
    for pid, name, ls in cur.fetchall():
        log("  " + pid + " " + name + " 最后在线 " + ts(ls))
    log()

    # ---------- 3 覆盖写：UPSERT ----------
    log("【3】覆盖写：ON CONFLICT DO UPDATE（断线重连时用，一条语句完成插入或更新）")
    log("    SQL: INSERT ... ON CONFLICT(id) DO UPDATE SET hp=excluded.hp, ...")
    cur.execute(
        "INSERT INTO players (id, name, x, y, hp, online, last_seen, created_at) "
        "VALUES (?, ?, ?, ?, ?, ?, ?, ?) "
        "ON CONFLICT(id) DO UPDATE SET "
        "  name = excluded.name, x = excluded.x, y = excluded.y, hp = excluded.hp, "
        "  online = excluded.online, last_seen = excluded.last_seen",
        ("p1001", "阿雨", 20.0, 15.0, 35, 1, now + 60, now),
    )
    conn.commit()
    cur.execute("SELECT id, name, x, y, hp, online FROM players WHERE id = 'p1001'")
    dump(cur, "p1001 覆盖写后（hp 100 -> 35）")

    # ---------- 4 读档 ----------
    log("【4】读档：按主键一条 SELECT，不再全量读文件")
    cur.execute("SELECT id, name, x, y, hp, online FROM players WHERE id = ?", ("p1002",))
    dump(cur, "SELECT ... WHERE id = 'p1002'")

    # ---------- 5 更新状态 ----------
    log("【5】更新状态：玩家下线，改 online 与 last_seen")
    cur.execute(
        "UPDATE players SET online = 0, last_seen = ? WHERE id = ?", (now + 120, "p1001")
    )
    conn.commit()
    cur.execute("SELECT id, name, hp, online FROM players ORDER BY id")
    dump(cur, "更新后（p1001 已置为离线）")

    # ---------- 6 登录流水 + JOIN ----------
    log("【6】登录流水：往 login_log 追加记录，再用 JOIN 汇总")
    cur.executemany(
        "INSERT INTO login_log (player_id, login_at) VALUES (?, ?)",
        [("p1001", now - 7200), ("p1001", now - 3600), ("p1001", now + 60), ("p1002", now - 1800)],
    )
    conn.commit()
    log("    SQL: SELECT p.id, COUNT(l.id), MAX(l.login_at) FROM players p LEFT JOIN login_log l ...")
    cur.execute(
        "SELECT p.id, p.name, COUNT(l.id) AS 登录次数, MAX(l.login_at) AS 最近登录, p.online "
        "FROM players p LEFT JOIN login_log l ON l.player_id = p.id "
        "GROUP BY p.id, p.name, p.online ORDER BY 登录次数 DESC"
    )
    rows = cur.fetchall()
    log("  --- LEFT JOIN 汇总 ---")
    for pid, name, cnt, last, online in rows:
        log("  " + pid + " " + name + " 登录" + str(cnt) + "次 最近 " + (ts(last) if last else "无记录") + " 在线=" + str(online))
    log()

    # ---------- 7 聚合查询 ----------
    log("【7】聚合查询：在线人数、平均血量、最近 24 小时登录人次")
    cur.execute(
        "SELECT COUNT(*) FROM players WHERE online = 1"
    )
    log("  在线人数        = " + str(cur.fetchone()[0]))
    cur.execute("SELECT ROUND(AVG(hp), 1) FROM players")
    log("  平均血量        = " + str(cur.fetchone()[0]))
    cur.execute("SELECT COUNT(*) FROM login_log WHERE login_at >= ?", (now - 86400,))
    log("  24 小时登录人次 = " + str(cur.fetchone()[0]))
    log()

    # ---------- 8 清理 ----------
    log("【8】清理：删除超过 30 天未登录的存档（先查出来，再删，再核）")
    cutoff = now - 86400 * 30
    cur.execute("SELECT id, name, last_seen FROM players WHERE last_seen < ?", (cutoff,))
    dump(cur, "命中待清理")
    cur.execute("DELETE FROM players WHERE last_seen < ? AND online = 0", (cutoff,))
    conn.commit()
    log("  实际删除行数 = " + str(cur.rowcount))
    cur.execute("SELECT id, name FROM players ORDER BY id")
    dump(cur, "清理后剩余")

    # ---------- 9 事务 ----------
    log("【9】事务：一批写入要么全成功、要么全部回滚")
    before = len(lines)
    try:
        conn.execute("BEGIN")
        cur.execute(
            "INSERT INTO players (id, name, x, y, hp, online, last_seen, created_at) "
            "VALUES (?,?,?,?,?,?,?,?)",
            ("p1004", "测试号A", 0, 0, 100, 1, now, now),
        )
        # 故意写一条违规数据：name 为 NULL，触发 NOT NULL 约束
        cur.execute(
            "INSERT INTO players (id, name, x, y, hp, online, last_seen, created_at) "
            "VALUES (?,?,?,?,?,?,?,?)",
            ("p1005", None, 0, 0, 100, 1, now, now),
        )
        conn.commit()
        log("  意外：没有触发约束错误")
    except sqlite3.IntegrityError as e:
        conn.rollback()
        log("  触发约束错误：" + str(e))
        log("  已回滚 —— p1004 也不该存在")
    cur.execute("SELECT COUNT(*) FROM players WHERE id IN ('p1004','p1005')")
    log("  回滚后 p1004/p1005 行数 = " + str(cur.fetchone()[0]) + "（应为 0）")
    log()

    # ---------- 10 索引是否生效 ----------
    log("【10】看索引有没有被用到：EXPLAIN QUERY PLAN")
    log("    SQL: SELECT id FROM players WHERE online = 1 ORDER BY last_seen DESC")
    cur.execute("EXPLAIN QUERY PLAN SELECT id FROM players WHERE online = 1 ORDER BY last_seen DESC")
    for row in cur.fetchall():
        log("  " + str(row[-1]))
    log()

    # ---------- 汇总 ----------
    cur.execute("SELECT COUNT(*) FROM players")
    n_players = cur.fetchone()[0]
    cur.execute("SELECT COUNT(*) FROM login_log")
    n_logs = cur.fetchone()[0]

    log("=" * 78)
    log("完成。players 表 " + str(n_players) + " 行，login_log 表 " + str(n_logs) + " 行。")
    log("本次覆盖：建表 / 插入 / UPSERT / 单条读档 / 更新 / JOIN / 聚合 / 条件删除 / 事务回滚 / 执行计划")
    log("=" * 78)

    conn.close()

    text = "\n".join(lines)
    with open(OUT_PATH, "w", encoding="utf-8") as f:
        f.write(text + "\n")

    try:
        sys.stdout.reconfigure(encoding="utf-8")
    except Exception:
        pass
    print(text)
    print()
    print("输出已写入: " + OUT_PATH)


if __name__ == "__main__":
    main()
