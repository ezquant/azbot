package localkv

import (
	"fmt"
	"os"
	"path"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/buntdb"
)

// LocalKV Structure to hold the db client
type LocalKV struct {
	db     *buntdb.DB
	dbPath string
}

// NewLocalKV constructor for the db client
func NewLocalKV(databasePath *string) (*LocalKV, error) {
	var dbPath string

	if databasePath == nil {
		// 如果没有提供路径，使用内存存储
		dbPath = ":memory:"
	} else {
		// 确保目录存在
		if err := os.MkdirAll(*databasePath, 0755); err != nil {
			return nil, fmt.Errorf("创建数据库目录失败: %v", err)
		}
		dbPath = path.Join(*databasePath, "kv.db")
	}

	// 打开数据库（如果是 :memory: 则创建内存数据库）
	db, err := buntdb.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %v", err)
	}

	// 设置数据库配置
	if err := db.SetConfig(buntdb.Config{
		SyncPolicy:         buntdb.Never, // 优化性能，因为是临时存储
		AutoShrinkDisabled: true,         // 禁用自动收缩
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("设置数据库配置失败: %v", err)
	}

	return &LocalKV{
		db:     db,
		dbPath: dbPath,
	}, nil
}

// Close closes the db
func (l *LocalKV) Close() error {
	return l.db.Close()
}

// Get gets a value from the db
func (l *LocalKV) Get(key string) (string, error) {
	var val string

	err := l.db.View(func(tx *buntdb.Tx) error {
		v, err := tx.Get(key)
		if err != nil {
			return err
		}

		val = v
		return nil
	})

	return val, err
}

// Set sets a value in the db
func (l *LocalKV) Set(key, value string) error {
	return l.db.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set(key, value, nil)

		return err
	})
}

// RemoveDB removes db file
func (l *LocalKV) RemoveDB() error {
	if l.db != nil {
		l.db.Close()
	}

	// 只有在使用文件存储时才删除文件
	if l.dbPath != ":memory:" && l.dbPath != "" {
		if err := os.Remove(l.dbPath); err != nil {
			if !os.IsNotExist(err) {
				log.Warnf("删除数据库文件失败: %v", err)
			}
		}
	}
	return nil
}
