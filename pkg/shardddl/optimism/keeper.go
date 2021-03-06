// Copyright 2020 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package optimism

import (
	"fmt"
	"sort"
	"sync"

	"github.com/pingcap/tidb-tools/pkg/dbutil"
)

// LockKeeper used to keep and handle DDL lock conveniently.
// The lock information do not need to be persistent, and can be re-constructed from the shard DDL info.
type LockKeeper struct {
	mu    sync.RWMutex
	locks map[string]*Lock // lockID -> Lock
}

// NewLockKeeper creates a new LockKeeper instance.
func NewLockKeeper() *LockKeeper {
	return &LockKeeper{
		locks: make(map[string]*Lock),
	}
}

// TrySync tries to sync the lock.
func (lk *LockKeeper) TrySync(info Info, sts []SourceTables) (string, []string, error) {
	var (
		lockID = genDDLLockID(info)
		l      *Lock
		ok     bool
	)

	lk.mu.Lock()
	defer lk.mu.Unlock()

	if l, ok = lk.locks[lockID]; !ok {
		lk.locks[lockID] = NewLock(lockID, info.Task, info.TableInfoBefore, sts)
		l = lk.locks[lockID]
	}

	newDDLs, err := l.TrySync(info.Source, info.UpSchema, info.UpTable, info.DDLs, info.TableInfoAfter, sts)
	return lockID, newDDLs, err
}

// RemoveLock removes a lock.
func (lk *LockKeeper) RemoveLock(lockID string) bool {
	lk.mu.Lock()
	defer lk.mu.Unlock()

	_, ok := lk.locks[lockID]
	delete(lk.locks, lockID)
	return ok
}

// FindLock finds a lock.
func (lk *LockKeeper) FindLock(lockID string) *Lock {
	lk.mu.RLock()
	defer lk.mu.RUnlock()

	return lk.locks[lockID]
}

// FindLockByInfo finds a lock with a shard DDL info.
func (lk *LockKeeper) FindLockByInfo(info Info) *Lock {
	return lk.FindLock(genDDLLockID(info))
}

// Locks return a copy of all Locks.
func (lk *LockKeeper) Locks() map[string]*Lock {
	lk.mu.RLock()
	defer lk.mu.RUnlock()

	locks := make(map[string]*Lock, len(lk.locks))
	for k, v := range lk.locks {
		locks[k] = v
	}
	return locks
}

// Clear clears all Locks.
func (lk *LockKeeper) Clear() {
	lk.mu.Lock()
	defer lk.mu.Unlock()

	lk.locks = make(map[string]*Lock)
}

// genDDLLockID generates DDL lock ID from its info.
func genDDLLockID(info Info) string {
	return fmt.Sprintf("%s-%s", info.Task, dbutil.TableName(info.DownSchema, info.DownTable))
}

// TableKeeper used to keep initial tables for a task in optimism mode.
type TableKeeper struct {
	mu     sync.RWMutex
	tables map[string]map[string]SourceTables // task-name -> source-ID -> tables.
}

// NewTableKeeper creates a new TableKeeper instance.
func NewTableKeeper() *TableKeeper {
	return &TableKeeper{
		tables: make(map[string]map[string]SourceTables),
	}
}

// Init (re-)initializes the keeper with initial source tables.
func (tk *TableKeeper) Init(stm map[string]map[string]SourceTables) {
	tk.mu.Lock()
	defer tk.mu.Unlock()

	tk.tables = make(map[string]map[string]SourceTables)
	for task, sts := range stm {
		if _, ok := tk.tables[task]; !ok {
			tk.tables[task] = make(map[string]SourceTables)
		}
		for source, st := range sts {
			tk.tables[task][source] = st
		}
	}
}

// Update adds/updates tables into the keeper or removes tables from the keeper.
// it returns whether added/updated or removed.
func (tk *TableKeeper) Update(st SourceTables) bool {
	tk.mu.Lock()
	defer tk.mu.Unlock()

	if st.IsDeleted {
		if _, ok := tk.tables[st.Task]; !ok {
			return false
		}
		if _, ok := tk.tables[st.Task][st.Source]; !ok {
			return false
		}
		delete(tk.tables[st.Task], st.Source)
		return true
	}

	if _, ok := tk.tables[st.Task]; !ok {
		tk.tables[st.Task] = make(map[string]SourceTables)
	}
	tk.tables[st.Task][st.Source] = st
	return true
}

// AddTable adds a table into the source tables.
// it returns whether added (not exist before).
// NOTE: we only add for existing task now.
func (tk *TableKeeper) AddTable(task, source, schema, table string) bool {
	tk.mu.Lock()
	defer tk.mu.Unlock()

	if _, ok := tk.tables[task]; !ok {
		return false
	}
	if _, ok := tk.tables[task][source]; !ok {
		tk.tables[task][source] = NewSourceTables(task, source, map[string]map[string]struct{}{})
	}
	st := tk.tables[task][source]
	added := st.AddTable(schema, table)
	tk.tables[task][source] = st // assign the modified SourceTables.
	return added
}

// RemoveTable removes a table from the source tables.
// it returns whether removed (exit before).
func (tk *TableKeeper) RemoveTable(task, source, schema, table string) bool {
	tk.mu.Lock()
	defer tk.mu.Unlock()

	if _, ok := tk.tables[task]; !ok {
		return false
	}
	if _, ok := tk.tables[task][source]; !ok {
		return false
	}
	st := tk.tables[task][source]
	removed := st.RemoveTable(schema, table)
	tk.tables[task][source] = st // assign the modified SourceTables.
	return removed
}

// FindTables finds source tables by task name.
func (tk *TableKeeper) FindTables(task string) []SourceTables {
	tk.mu.RLock()
	defer tk.mu.RUnlock()

	stm, ok := tk.tables[task]
	if !ok {
		return nil
	}
	return SourceTablesMapToSlice(stm)
}

// SourceTablesMapToSlice converts a map[string]SourceTables to []SourceTables.
func SourceTablesMapToSlice(stm map[string]SourceTables) []SourceTables {
	var ret SourceTablesSlice
	for _, st := range stm {
		ret = append(ret, st)
	}
	sort.Sort(ret)
	return ret
}

// SourceTablesSlice attaches the methods of Interface to []SourceTables,
// sorting in increasing order according to `Source` field.
type SourceTablesSlice []SourceTables

// Len implements Sorter.Len.
func (t SourceTablesSlice) Len() int {
	return len(t)
}

// Less implements Sorter.Less.
func (t SourceTablesSlice) Less(i, j int) bool {
	return t[i].Source < t[j].Source
}

// Swap implements Sorter.Swap.
func (t SourceTablesSlice) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}
