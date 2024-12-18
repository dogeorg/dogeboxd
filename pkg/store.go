package dogeboxd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

type StoreManager struct {
	DBPath  string
	DB      *sql.DB
	WriteMu sync.Mutex
}

func NewStoreManager(dbPath string) (*StoreManager, error) {
	store := &StoreManager{DBPath: dbPath}
	if err := store.OpenDB(); err != nil {
		return nil, err
	}
	return store, nil
}

func (sm *StoreManager) CloseDB() error {
	sm.WriteMu.Lock()
	defer sm.WriteMu.Unlock()

	if sm.DB != nil {
		if err := sm.DB.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (sm *StoreManager) OpenDB() error {
	db, err := sql.Open("sqlite3", sm.DBPath)
	if err != nil {
		return err
	}
	sm.DB = db
	return nil
}

func (sm *StoreManager) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		started <- true
		<-stop
		// wait for writes to finish..
		sm.WriteMu.Lock()
		defer sm.WriteMu.Unlock()
		sm.DB.Close()
		stopped <- true
	}()
	return nil
}

func (sm *StoreManager) ensureTableExists(tableName string) {
	sm.WriteMu.Lock()
	defer sm.WriteMu.Unlock()

	_, err := sm.DB.Exec(fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS %s (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            key TEXT UNIQUE NOT NULL,
            value JSON NOT NULL
        )
    `, tableName))
	if err != nil {
		fmt.Println("Error creating table:", err)
	}
}

func GetTypeStore[T any](sm *StoreManager) *TypeStore[T] {
	typeName := reflect.TypeOf((*T)(nil)).Elem().Name()
	tableName := strings.ToLower(strings.ReplaceAll(typeName, "_", ""))
	sm.ensureTableExists(tableName)
	return &TypeStore[T]{sm: sm, mu: &sm.WriteMu, Table: tableName}
}

type TypeStore[T any] struct {
	sm    *StoreManager
	mu    *sync.Mutex
	Table string
}

func (ts *TypeStore[T]) Set(key string, value T) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	valueBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}

	_, err = ts.sm.DB.Exec(fmt.Sprintf("INSERT OR REPLACE INTO %s (key, value) VALUES (?, ?)", ts.Table), key, valueBytes)
	return err
}

func (ts *TypeStore[T]) Get(key string) (T, error) {
	var valueBytes []byte
	err := ts.sm.DB.QueryRow(fmt.Sprintf("SELECT value FROM %s WHERE key = ?", ts.Table), key).Scan(&valueBytes)
	if err != nil {
		return *new(T), err
	}

	var value T
	err = json.Unmarshal(valueBytes, &value)
	return value, err
}

func (ts *TypeStore[T]) Del(key string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	_, err := ts.sm.DB.Exec(fmt.Sprintf("DELETE FROM %s WHERE key = ?", ts.Table), key)
	return err
}

// This should not be used to update/insert, it doesn't lock
func (ts *TypeStore[T]) Exec(query string, args ...interface{}) ([]T, error) {
	rows, err := ts.sm.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []T
	for rows.Next() {
		var valueBytes []byte
		if err := rows.Scan(&valueBytes); err != nil {
			return nil, err
		}

		var value T
		if err := json.Unmarshal(valueBytes, &value); err != nil {
			return nil, err
		}
		results = append(results, value)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}
