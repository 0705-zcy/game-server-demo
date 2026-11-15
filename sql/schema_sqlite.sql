-- game-server-demo 存档层表结构（SQLite 版）
-- 对应代码：protocol.go 里的 PlayerState 结构体
-- 设计要点：
--   1) 存档表一行一个玩家，主键就是登录 ID，读档一条 SELECT 就够，不再全量读 JSON；
--   2) login_log 只追加不更新，用来做"最近登录时间"和"登录频次"，不和存档表混在一张表里；
--   3) last_seen 用 unix 秒存整数，方便做"超过 N 天未登录就清理"的范围查询。

DROP TABLE IF EXISTS login_log;
DROP TABLE IF EXISTS players;

CREATE TABLE players (
    id          TEXT    PRIMARY KEY,          -- 玩家 ID（登录标识）
    name        TEXT    NOT NULL,             -- 昵称
    x           REAL    NOT NULL DEFAULT 0,   -- 坐标（float64）
    y           REAL    NOT NULL DEFAULT 0,
    hp          INTEGER NOT NULL DEFAULT 100,
    online      INTEGER NOT NULL DEFAULT 0,   -- 0 离线 / 1 在线（SQLite 没有 bool）
    last_seen   INTEGER NOT NULL,             -- 最后一次在线时间，unix 秒
    created_at  INTEGER NOT NULL              -- 首次建号时间，unix 秒
);

-- 联合索引：查"在线玩家"和"清理长期离线"这两条最常用的语句都会走它
CREATE INDEX idx_players_online ON players (online, last_seen);

CREATE TABLE login_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id   TEXT    NOT NULL REFERENCES players (id),  -- 外键：流水不能挂在不存在的玩家上
    login_at    INTEGER NOT NULL
);

-- 按玩家查登录记录时会用到
CREATE INDEX idx_login_log_player ON login_log (player_id, login_at);
