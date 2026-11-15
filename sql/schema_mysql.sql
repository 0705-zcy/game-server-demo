-- game-server-demo 存档层表结构（MySQL 8 版）
-- 与 schema_sqlite.sql 是同一套设计，差异只在类型和引擎，面试可以主动讲这几条：
--   TEXT 主键  → VARCHAR(64)：MySQL 的 TEXT 不能直接做主键，且 VARCHAR 有长度上限更省空间
--   REAL      → DOUBLE
--   无 bool   → TINYINT(1)
--   INTEGER PK AUTOINCREMENT → BIGINT AUTO_INCREMENT
--   额外需要：InnoDB（支持事务和外键）、utf8mb4（昵称可能有 emoji）

CREATE TABLE IF NOT EXISTS players (
    id          VARCHAR(64)  NOT NULL,
    name        VARCHAR(64)  NOT NULL,
    x           DOUBLE       NOT NULL DEFAULT 0,
    y           DOUBLE       NOT NULL DEFAULT 0,
    hp          INT          NOT NULL DEFAULT 100,
    online      TINYINT(1)   NOT NULL DEFAULT 0,
    last_seen   BIGINT       NOT NULL,
    created_at  BIGINT       NOT NULL,
    PRIMARY KEY (id),
    KEY idx_players_online (online, last_seen)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS login_log (
    id          BIGINT       NOT NULL AUTO_INCREMENT,
    player_id   VARCHAR(64)  NOT NULL,
    login_at    BIGINT       NOT NULL,
    PRIMARY KEY (id),
    KEY idx_login_log_player (player_id, login_at),
    CONSTRAINT fk_login_log_player FOREIGN KEY (player_id) REFERENCES players (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
