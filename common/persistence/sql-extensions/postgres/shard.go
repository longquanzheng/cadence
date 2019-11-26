// Copyright (c) 2019 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package postgres

import (
	"database/sql"

	"github.com/uber/cadence/common/persistence/sql/storage/sqldb"
)

const (
	createShardQry = `INSERT INTO
 shards (shard_id, range_id, data, data_encoding) VALUES ($1, $2, $3, $4)`

	getShardQry = `SELECT
 shard_id, range_id, data, data_encoding
 FROM shards WHERE shard_id = $1`

	updateShardQry = `UPDATE shards 
 SET range_id = $1, data = $2, data_encoding = $3 
 WHERE shard_id = $4`

	lockShardQry     = `SELECT range_id FROM shards WHERE shard_id = $1 FOR UPDATE`
	readLockShardQry = `SELECT range_id FROM shards WHERE shard_id = $1 FOR SHARE`
)

// InsertIntoShards inserts one or more rows into shards table
func (mdb *db) InsertIntoShards(row *sqldb.ShardsRow) (sql.Result, error) {
	return mdb.conn.Exec(createShardQry, row.ShardID, row.RangeID, row.Data, row.DataEncoding)
}

// UpdateShards updates one or more rows into shards table
func (mdb *db) UpdateShards(row *sqldb.ShardsRow) (sql.Result, error) {
	return mdb.conn.Exec(updateShardQry, row.RangeID, row.Data, row.DataEncoding, row.ShardID)
}

// SelectFromShards reads one or more rows from shards table
func (mdb *db) SelectFromShards(filter *sqldb.ShardsFilter) (*sqldb.ShardsRow, error) {
	var row sqldb.ShardsRow
	err := mdb.conn.Get(&row, getShardQry, filter.ShardID)
	if err != nil {
		return nil, err
	}
	return &row, err
}

// ReadLockShards acquires a read lock on a single row in shards table
func (mdb *db) ReadLockShards(filter *sqldb.ShardsFilter) (int, error) {
	var rangeID int
	err := mdb.conn.Get(&rangeID, readLockShardQry, filter.ShardID)
	return rangeID, err
}

// WriteLockShards acquires a write lock on a single row in shards table
func (mdb *db) WriteLockShards(filter *sqldb.ShardsFilter) (int, error) {
	var rangeID int
	err := mdb.conn.Get(&rangeID, lockShardQry, filter.ShardID)
	return rangeID, err
}
