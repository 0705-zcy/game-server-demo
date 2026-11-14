package main

import (
	"bufio"
	"encoding/json"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxSessions   = 500             // 单机硬上限，先挡住雪崩，再谈扩容
	outBufferSize = 64              // 每连接的发送队列长度
	sendTimeout   = 5 * time.Second // 单条消息写超时，防止半死连接占住写协程
	worldSize     = 100.0           // 地图边长，坐标范围 [0,100]
	maxMoveStep   = 5.0             // 单帧最大位移，最朴素的"加速外挂"防线
	autoSaveEvery = 10 * time.Second
)

type Session struct {
	conn     net.Conn
	out      chan []byte // 下行队列：读协程只负责塞，写协程独占 conn
	playerID string
	name     string
	closed   atomic.Bool
}

type Server struct {
	store Store
	tick  time.Duration

	mu       sync.RWMutex
	sessions map[*Session]struct{}
	players  map[string]*PlayerState // 只放"在线"玩家，离线即出房间并落盘
	saved    map[string]*PlayerState // 只放"离线档案"，启动时从文件预载，登录时取用

	tickNo    uint64 // atomic
	startedAt time.Time

	quit     chan struct{}
	quitOnce sync.Once
	wg       sync.WaitGroup // 主循环
	connWG   sync.WaitGroup // 连接处理
	ln       net.Listener
}

func NewServer(store Store, tick time.Duration) *Server {
	s := &Server{
		store:     store,
		tick:      tick,
		sessions:  make(map[*Session]struct{}),
		players:   make(map[string]*PlayerState),
		saved:     make(map[string]*PlayerState),
		startedAt: time.Now(),
		quit:      make(chan struct{}),
	}

	// 启动时把文件里的存档读进"离线档案"表，而不是直接塞进房间。
	// 这里的坑：一旦把离线玩家当成房间里的玩家，他下次登录就会被判定为
	// "已经在房间"而登录失败。所以在线表和离线表必须分开。
	all, err := store.LoadAll()
	if err != nil {
		log.Printf("warn: load save file failed: %v (starting empty)", err)
	} else {
		for id, p := range all {
			p.Online = false // 上次可能异常退出，一律先标离线
			s.saved[id] = p
		}
		log.Printf("loaded %d offline archive(s)", len(all))
	}
	return s
}

func (s *Server) Start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.ln = ln
	s.wg.Add(3)
	go s.acceptLoop()
	go s.tickLoop()
	go s.autoSaveLoop()
	return nil
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
			}
			log.Printf("accept error: %v", err)
			continue
		}
		s.connWG.Add(1)
		go func() {
			defer s.connWG.Done()
			s.handleConn(conn)
		}()
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	s.mu.Lock()
	if len(s.sessions) >= maxSessions {
		s.mu.Unlock()
		s.writeOnce(conn, ServerMsg{Type: TypeError, Msg: "server_full"})
		return
	}
	sess := &Session{conn: conn, out: make(chan []byte, outBufferSize)}
	s.sessions[sess] = struct{}{}
	s.mu.Unlock()

	go s.writeLoop(sess)

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), 1<<20) // 单条消息上限 1MB，挡住畸形包
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg ClientMsg
		if err := json.Unmarshal(line, &msg); err != nil {
			s.push(sess, ServerMsg{Type: TypeError, Msg: "bad_json"})
			continue
		}
		s.handleMessage(sess, msg)
		if sess.closed.Load() {
			break
		}
	}
	s.disconnect(sess)
}

func (s *Server) writeLoop(sess *Session) {
	for {
		select {
		case b := <-sess.out:
			sess.conn.SetWriteDeadline(time.Now().Add(sendTimeout))
			if _, err := sess.conn.Write(b); err != nil {
				sess.closed.Store(true)
				sess.conn.Close()
				return
			}
		case <-s.quit:
			return
		}
	}
}

func (s *Server) handleMessage(sess *Session, msg ClientMsg) {
	switch msg.Type {
	case TypeLogin:
		s.handleLogin(sess, msg)
	case TypeMove:
		s.handleMove(sess, msg)
	case TypeChat:
		s.handleChat(sess, msg)
	case TypePing:
		s.push(sess, ServerMsg{Type: TypePong})
	case TypeLogout:
		sess.conn.Close()
	default:
		s.push(sess, ServerMsg{Type: TypeError, Msg: "unknown_type:" + msg.Type})
	}
}

func (s *Server) handleLogin(sess *Session, msg ClientMsg) {
	id := strings.TrimSpace(msg.PlayerID)
	if id == "" || len(id) > 32 {
		s.push(sess, ServerMsg{Type: TypeError, Msg: "invalid_player_id"})
		return
	}
	if sess.playerID != "" {
		s.push(sess, ServerMsg{Type: TypeError, Msg: "already_login"})
		return
	}

	name := strings.TrimSpace(msg.Name)
	if name == "" {
		name = id
	}
	if r := []rune(name); len(r) > 16 {
		name = string(r[:16])
	}

	s.mu.Lock()
	if cur, ok := s.players[id]; ok && cur.Online {
		// 同一个 ID 二次登录（挤号）：这里直接拒绝，真实项目里通常踢掉旧连接
		s.mu.Unlock()
		s.push(sess, ServerMsg{Type: TypeError, Msg: "already_online"})
		return
	}

	st := s.saved[id] // 从离线档案取，取到就说明是老玩家回流
	restored := st != nil
	if st == nil {
		st = &PlayerState{
			ID: id,
			X:  rand.Float64() * worldSize,
			Y:  rand.Float64() * worldSize,
			HP: 100,
		}
	}
	delete(s.saved, id) // 移出离线档案，进入房间
	st.Name = name
	st.Online = true
	st.LastSeen = time.Now().Unix()
	s.players[id] = st

	sess.playerID = id
	sess.name = name
	tick := atomic.LoadUint64(&s.tickNo)
	s.mu.Unlock()

	s.push(sess, ServerMsg{
		Type:     TypeLoginOK,
		PlayerID: id,
		Name:     name,
		Tick:     tick,
		Restored: st.snapshotIf(restored),
	})
	log.Printf("login: %s (%s) restored=%v online=%d", id, name, restored, s.OnlineCount())
}

// snapshotIf 只在"读到了旧存档"时返回快照，新号不返回，避免客户端误判。
func (p *PlayerState) snapshotIf(cond bool) *PlayerState {
	if !cond {
		return nil
	}
	s := *p
	return &s
}

func (s *Server) handleMove(sess *Session, msg ClientMsg) {
	if sess.playerID == "" {
		s.push(sess, ServerMsg{Type: TypeError, Msg: "not_logged_in"})
		return
	}
	dx, dy := msg.X, msg.Y
	if dx > maxMoveStep {
		dx = maxMoveStep
	} else if dx < -maxMoveStep {
		dx = -maxMoveStep
	}
	if dy > maxMoveStep {
		dy = maxMoveStep
	} else if dy < -maxMoveStep {
		dy = -maxMoveStep
	}

	s.mu.Lock()
	p := s.players[sess.playerID]
	if p != nil {
		p.X = clamp(p.X+dx, 0, worldSize)
		p.Y = clamp(p.Y+dy, 0, worldSize)
	}
	s.mu.Unlock()
}

func (s *Server) handleChat(sess *Session, msg ClientMsg) {
	if sess.playerID == "" {
		s.push(sess, ServerMsg{Type: TypeError, Msg: "not_logged_in"})
		return
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
	if r := []rune(text); len(r) > 200 {
		text = string(r[:200])
	}
	s.broadcast(ServerMsg{Type: TypeChat, From: sess.playerID, Text: text})
}

func (s *Server) tickLoop() {
	defer s.wg.Done()
	t := time.NewTicker(s.tick)
	defer t.Stop()
	for {
		select {
		case <-s.quit:
			return
		case <-t.C:
			tick := atomic.AddUint64(&s.tickNo, 1)
			s.broadcastSnapshot(tick)
		}
	}
}

// broadcastSnapshot 是热路径，三条规矩：
// 1) 锁内只做拷贝，绝不写网络；
// 2) 快照只序列化一次，再发给所有人；
// 3) 发送永不阻塞，队列满了直接踢掉慢客户端。
func (s *Server) broadcastSnapshot(tick uint64) {
	s.mu.RLock()
	states := make([]PlayerState, 0, len(s.players))
	for _, p := range s.players {
		if p.Online {
			states = append(states, p.snapshot())
		}
	}
	targets := make([]*Session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess.playerID != "" && !sess.closed.Load() {
			targets = append(targets, sess)
		}
	}
	s.mu.RUnlock()

	msg := ServerMsg{Type: TypeSnapshot, Tick: tick, Players: states}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	b = append(b, '\n')
	for _, sess := range targets {
		s.pushRaw(sess, b)
	}
}

func (s *Server) broadcast(msg ServerMsg) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	b = append(b, '\n')

	s.mu.RLock()
	targets := make([]*Session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess.playerID != "" && !sess.closed.Load() {
			targets = append(targets, sess)
		}
	}
	s.mu.RUnlock()

	for _, sess := range targets {
		s.pushRaw(sess, b)
	}
}

func (s *Server) push(sess *Session, msg ServerMsg) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	b = append(b, '\n')
	s.pushRaw(sess, b)
}

func (s *Server) pushRaw(sess *Session, b []byte) {
	select {
	case sess.out <- b:
	default:
		// 队列满 = 这个客户端已经跟不上 tick，继续留着他只会拖慢所有人
		sess.closed.Store(true)
		sess.conn.Close()
	}
}

func (s *Server) writeOnce(conn net.Conn, msg ServerMsg) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	b = append(b, '\n')
	conn.SetWriteDeadline(time.Now().Add(sendTimeout))
	conn.Write(b)
}

func (s *Server) disconnect(sess *Session) {
	s.mu.Lock()
	delete(s.sessions, sess)
	pid := sess.playerID
	if pid == "" {
		s.mu.Unlock()
		return
	}
	p, ok := s.players[pid]
	if !ok {
		s.mu.Unlock()
		return
	}
	// 离线即出房间：但先把最终状态挪进离线档案，再整体落盘
	p.Online = false
	p.LastSeen = time.Now().Unix()
	cp := p.snapshot()
	s.saved[pid] = &cp
	delete(s.players, pid)
	s.mu.Unlock()

	s.saveAll()
	log.Printf("logout: %s online=%d", pid, s.OnlineCount())
}

func (s *Server) autoSaveLoop() {
	defer s.wg.Done()
	t := time.NewTicker(autoSaveEvery)
	defer t.Stop()
	for {
		select {
		case <-s.quit:
			return
		case <-t.C:
			s.saveAll()
		}
	}
}

func (s *Server) saveAll() {
	s.mu.RLock()
	all := make(map[string]*PlayerState, len(s.saved)+len(s.players))
	for k, v := range s.saved {
		cp := v.snapshot()
		all[k] = &cp
	}
	for k, v := range s.players {
		cp := v.snapshot()
		all[k] = &cp
	}
	s.mu.RUnlock()

	if err := s.store.SaveAll(all); err != nil {
		log.Printf("save failed: %v", err)
	}
}

func (s *Server) OnlineCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.players)
}

func (s *Server) StartHTTP(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		online, sess := len(s.players), len(s.sessions)
		s.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]any{
			"online_players": online,
			"sessions":       sess,
			"tick":           atomic.LoadUint64(&s.tickNo),
			"uptime_sec":     int(time.Since(s.startedAt).Seconds()),
		})
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	log.Printf("stats endpoint: http://%s/stats", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("http stats stopped: %v", err)
	}
}

func (s *Server) Shutdown() {
	s.quitOnce.Do(func() { close(s.quit) })
	if s.ln != nil {
		s.ln.Close()
	}

	s.mu.Lock()
	for sess := range s.sessions {
		sess.closed.Store(true)
		sess.conn.Close()
	}
	s.mu.Unlock()

	s.saveAll()
	s.wg.Wait()

	done := make(chan struct{})
	go func() {
		s.connWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		log.Printf("warning: some connections did not close in time")
	}
}
