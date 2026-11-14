package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Store 是持久化抽象。当前只有文件实现，但接口按"存档库"的语义设计：
// 换成 MySQL / Redis 时，只需要再实现一个 Store，服务端逻辑一行不用改。
// 这是这个 demo 里最值得在面试里讲的一点：先留接口，再谈扩容。
type Store interface {
	Load(id string) (*PlayerState, error)
	LoadAll() (map[string]*PlayerState, error)
	SaveAll(all map[string]*PlayerState) error
}

// FileStore 用单个 JSON 文件存全部玩家。
// 定位：本地跑得起来、零依赖；不适用于真实线上（写放大、无并发隔离）。
type FileStore struct {
	path string
	mu   sync.Mutex
}

func NewFileStore(path string) *FileStore { return &FileStore{path: path} }

func (f *FileStore) LoadAll() (map[string]*PlayerState, error) {
	b, err := os.ReadFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*PlayerState{}, nil
		}
		return nil, err
	}
	var data map[string]*PlayerState
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	if data == nil {
		data = map[string]*PlayerState{}
	}
	return data, nil
}

func (f *FileStore) Load(id string) (*PlayerState, error) {
	all, err := f.LoadAll()
	if err != nil {
		return nil, err
	}
	return all[id], nil
}

// SaveAll 先写临时文件再改名，避免进程在写一半时被 kill 导致存档损坏。
func (f *FileStore) SaveAll(all map[string]*PlayerState) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(f.path), ".players-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}

	if err := os.Rename(tmpName, f.path); err != nil {
		// Windows 上目标文件存在时 Rename 会失败，退化为先删后改名。
		// 代价是这一步不再原子，仅影响进程崩溃瞬间，demo 可接受。
		os.Remove(f.path)
		if err2 := os.Rename(tmpName, f.path); err2 != nil {
			os.Remove(tmpName)
			return err2
		}
	}
	return nil
}
