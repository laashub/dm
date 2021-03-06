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
	. "github.com/pingcap/check"
)

func (t *testForEtcd) TestDeleteInfosOperations(c *C) {
	defer clearTestInfoOperation(c)

	var (
		task     = "test"
		source   = "mysql-replica-1"
		upSchema = "foo-1"
		upTable  = "bar-1"
		DDLs     = []string{"ALTER TABLE bar ADD COLUMN c1 INT"}
		info     = NewInfo(task, source, upSchema, upTable, "foo", "bar", DDLs, nil, nil)
		op       = NewOperation("test-ID", task, source, upSchema, upTable, DDLs, ConflictResolved, false)
	)

	// put info.
	_, err := PutInfo(etcdTestCli, info)
	c.Assert(err, IsNil)
	ifm, _, err := GetAllInfo(etcdTestCli)
	c.Assert(err, IsNil)
	c.Assert(ifm, HasLen, 1)
	c.Assert(ifm[task][source][upSchema][upTable], DeepEquals, info)

	// put operation.
	_, _, err = PutOperation(etcdTestCli, false, op)
	c.Assert(err, IsNil)
	opm, _, err := GetAllOperations(etcdTestCli)
	c.Assert(err, IsNil)
	c.Assert(opm, HasLen, 1)
	c.Assert(opm[task][source][upSchema][upTable], DeepEquals, op)

	// DELETE info and operation.
	_, err = DeleteInfosOperations(etcdTestCli, []Info{info}, []Operation{op})
	c.Assert(err, IsNil)

	// verify no info & operation exist.
	ifm, _, err = GetAllInfo(etcdTestCli)
	c.Assert(err, IsNil)
	c.Assert(ifm, HasLen, 0)
	opm, _, err = GetAllOperations(etcdTestCli)
	c.Assert(err, IsNil)
	c.Assert(opm, HasLen, 0)
}

func (t *testForEtcd) TestSourceTablesInfo(c *C) {
	defer clearTestInfoOperation(c)

	var (
		task     = "task"
		source   = "mysql-replica-1"
		upSchema = "foo-1"
		upTable  = "bar-1"
		st1      = NewSourceTables(task, source, map[string]map[string]struct{}{
			"db": {"tbl-1": struct{}{}, "tbl-2": struct{}{}},
		})
		st2 = NewSourceTables(task, source, map[string]map[string]struct{}{
			"db": {"tbl-2": struct{}{}, "tbl-3": struct{}{}},
		})
		i11 = NewInfo(task, source, upSchema, upTable, "foo", "bar",
			[]string{"ALTER TABLE bar ADD COLUMN c1 INT"}, nil, nil)
	)

	// put source tables and info.
	rev1, err := PutSourceTablesInfo(etcdTestCli, st1, i11)
	c.Assert(err, IsNil)
	c.Assert(rev1, Greater, int64(0))

	stm, rev2, err := GetAllSourceTables(etcdTestCli)
	c.Assert(err, IsNil)
	c.Assert(rev2, Equals, rev1)
	c.Assert(stm, HasLen, 1)
	c.Assert(stm[task], HasLen, 1)
	c.Assert(stm[task][source], DeepEquals, st1)

	ifm, rev3, err := GetAllInfo(etcdTestCli)
	c.Assert(err, IsNil)
	c.Assert(rev3, Equals, rev1)
	c.Assert(ifm, HasLen, 1)
	c.Assert(ifm[task], HasLen, 1)
	c.Assert(ifm[task][source], HasLen, 1)
	c.Assert(ifm[task][source][upSchema], HasLen, 1)
	c.Assert(ifm[task][source][upSchema][upTable], DeepEquals, i11)

	// put/update source tables and delete info.
	rev4, err := PutSourceTablesDeleteInfo(etcdTestCli, st2, i11)
	c.Assert(err, IsNil)
	c.Assert(rev4, Greater, rev1)

	stm, rev5, err := GetAllSourceTables(etcdTestCli)
	c.Assert(err, IsNil)
	c.Assert(rev5, Equals, rev4)
	c.Assert(stm, HasLen, 1)
	c.Assert(stm[task], HasLen, 1)
	c.Assert(stm[task][source], DeepEquals, st2)

	ifm, rev6, err := GetAllInfo(etcdTestCli)
	c.Assert(err, IsNil)
	c.Assert(rev6, Equals, rev4)
	c.Assert(ifm, HasLen, 0)
}
