package main

// 协议层：只定义客户端与服务端之间收发的消息结构。
// 设计理由：全部走 JSON + 换行分隔（newline-delimited JSON），
// 好处是抓包可读、任意语言都能接，缺点是比二进制协议多占带宽，
// 面试时可以主动说明这个取舍。

const (
	TypeLogin    = "login"
	TypeLoginOK  = "login_ok"
	TypeMove     = "move"
	TypeChat     = "chat"
	TypePing     = "ping"
	TypePong     = "pong"
	TypeLogout   = "logout"
	TypeSnapshot = "snapshot"
	TypeError    = "error"
)

// ClientMsg 客户端上行消息。X/Y 表示本帧的位移增量（不是绝对坐标），
// 这样客户端不需要知道世界坐标，服务端也能统一做速度限制。
type ClientMsg struct {
	Type     string  `json:"type"`
	PlayerID string  `json:"playerId,omitempty"`
	Name     string  `json:"name,omitempty"`
	X        float64 `json:"x,omitempty"`
	Y        float64 `json:"y,omitempty"`
	Text     string  `json:"text,omitempty"`
}

// PlayerState 玩家状态。它同时是「房间内的实时状态」和「存档里的持久状态」，
// 字段一套到底，避免出现两套结构互相转换的维护成本。
type PlayerState struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	HP       int     `json:"hp"`
	Online   bool    `json:"online"`
	LastSeen int64   `json:"lastSeen"` // unix 秒，用于判断是否该清理离线数据
}

func (p *PlayerState) snapshot() PlayerState { return *p }

// ServerMsg 服务端下行消息。用一个联合结构承载所有类型，
// 便于统一序列化；缺点是字段冗余，量大时可以按类型拆成不同结构。
type ServerMsg struct {
	Type     string        `json:"type"`
	Tick     uint64        `json:"tick,omitempty"`
	PlayerID string        `json:"playerId,omitempty"`
	Name     string        `json:"name,omitempty"`
	Restored *PlayerState  `json:"restored,omitempty"`
	Players  []PlayerState `json:"players,omitempty"`
	From     string        `json:"from,omitempty"`
	Text     string        `json:"text,omitempty"`
	Msg      string        `json:"msg,omitempty"`
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
