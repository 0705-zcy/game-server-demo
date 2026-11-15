# sql/ —— 存档层的表结构与最小闭环

这里的 SQL 不是练习，是 `store.go` 里那个 `Store` 接口的第二套实现思路：
把「每次 SaveAll 全量写一个 JSON 文件」换成「一个玩家一行、按主键查一条」。
表结构直接对应 `protocol.go` 的 `PlayerState`。

## 文件

| 文件 | 作用 |
|---|---|
| `schema_sqlite.sql` | 本地跑用的建表语句（SQLite） |
| `schema_mysql.sql` | 线上会用到的同一套设计（MySQL 8／InnoDB） |
| `demo_sqlite.py` | 零依赖脚本，跑完 10 个动作并打印真实返回行 |
| `demo_output.txt` | 上一次运行的完整输出，可直接作为证据 |

## 怎么跑

```powershell
python sql/demo_sqlite.py
```

不需要装 MySQL，也不需要装 SQLite——Python 自带 `sqlite3` 模块，库文件 `players.db` 会在同目录生成（已在 `.gitignore` 里）。

## 覆盖了什么

建表与索引 → 多行 INSERT → UPSERT（`ON CONFLICT DO UPDATE`）→ 单条主键读档 → UPDATE 状态 →
流水表 + `LEFT JOIN` 汇总 → 聚合查询（在线数／平均血量／24h 登录人次）→ 条件 DELETE 清理 →
事务回滚（故意触发 `NOT NULL` 约束，验证整批不落地）→ `EXPLAIN QUERY PLAN` 确认索引生效。

## 两张表为什么这样切

- **players**：一行一个玩家，主键就是登录 ID。原来的 `FileStore` 改一个玩家要把整份档案读进来再写回去，写放大严重；换成按主键定位，读写都只碰一行。
- **login_log**：登录记录只追加、不更新。这类"流水"如果塞进 players 会变成数组字段，无法做时间范围查询，也无法统计频次。
- **last_seen 存 unix 整数**：清理长期离线存档时可以直接做范围比较（`WHERE last_seen < ?`），不用把时间当字符串解析。

## 面试可以这么讲（三句）

1. 存档从 JSON 文件换成表，核心变化是「全量读写」变成「按主键读写」，写放大和并发覆盖的问题一起解决。
2. 同一套设计写了两份 DDL：SQLite 本地零依赖跑，MySQL 线上用。差异在类型和引擎——`TEXT` 主键要换成 `VARCHAR`，`REAL` 换 `DOUBLE`，`bool` 换 `TINYINT(1)`，还要显式 `utf8mb4` 和 `InnoDB`。
3. 改表之前没动 `server.go` 一行，因为 `Store` 接口是按语义抽的——这是先留接口再谈扩容的价值。

## 边界（别把 demo 说成线上方案）

单机文件库、没有连接池、没有主从、没有分库分表；并发写入靠 SQLite 自身锁，量大了要换 MySQL／Redis，热点数据（在线状态）还应放内存，只把落库当作异步快照。这些是面试里应该主动交代的部分，不是减分项。
