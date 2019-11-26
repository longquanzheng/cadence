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
	"errors"
	"fmt"
	"github.com/uber/cadence/common/persistence/sql/storage/sqldb"
	"strings"
)

const (
	templateCreateWorkflowExecutionStarted = `INSERT INTO executions_visibility (` +
		`domain_id, workflow_id, run_id, start_time, execution_time, workflow_type_name, memo, encoding) ` +
		`VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
         ON CONFLICT (domain_id, run_id) DO NOTHING`

	templateCreateWorkflowExecutionClosed = `INSERT INTO executions_visibility (` +
		`domain_id, workflow_id, run_id, start_time, execution_time, workflow_type_name, close_time, close_status, history_length, memo, encoding) ` +
		`VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (domain_id, run_id) DO UPDATE 
		  SET workflow_id = excluded.workflow_id,
		      start_time = excluded.start_time,
		      execution_time = excluded.execution_time,
              workflow_type_name = excluded.workflow_type_name,
			  close_time = excluded.close_time,
			  close_status = excluded.close_status,
			  history_length = excluded.history_length,
			  memo = excluded.memo,
			  encoding = excluded.encoding`

	// RunID condition is needed for correct pagination
	templateConditions1 = ` AND domain_id = $1
		 AND start_time >= $2
		 AND start_time <= $3
 		 AND (run_id > $4 OR start_time < $5)
         ORDER BY start_time DESC, run_id
         LIMIT $6`

	templateConditions2 = ` AND domain_id = $2
		 AND start_time >= $3
		 AND start_time <= $4
 		 AND (run_id > $5 OR start_time < $6)
         ORDER BY start_time DESC, run_id
         LIMIT $7`

	templateOpenFieldNames = `workflow_id, run_id, start_time, execution_time, workflow_type_name, memo, encoding`
	templateOpenSelect     = `SELECT ` + templateOpenFieldNames + ` FROM executions_visibility WHERE close_status IS NULL `

	templateClosedSelect = `SELECT ` + templateOpenFieldNames + `, close_time, close_status, history_length
		 FROM executions_visibility WHERE close_status IS NOT NULL `

	templateGetOpenWorkflowExecutions = templateOpenSelect + templateConditions1

	templateGetClosedWorkflowExecutions = templateClosedSelect + templateConditions1

	templateGetOpenWorkflowExecutionsByType = templateOpenSelect + `AND workflow_type_name = $1` + templateConditions2

	templateGetClosedWorkflowExecutionsByType = templateClosedSelect + `AND workflow_type_name = $1` + templateConditions2

	templateGetOpenWorkflowExecutionsByID = templateOpenSelect + `AND workflow_id = $1` + templateConditions2

	templateGetClosedWorkflowExecutionsByID = templateClosedSelect + `AND workflow_id = $1` + templateConditions2

	templateGetClosedWorkflowExecutionsByStatus = templateClosedSelect + `AND close_status = $1` + templateConditions2

	templateGetClosedWorkflowExecution = `SELECT workflow_id, run_id, start_time, execution_time, memo, encoding, close_time, workflow_type_name, close_status, history_length 
		 FROM executions_visibility
		 WHERE domain_id = $1 AND close_status IS NOT NULL
		 AND run_id = $2`

	templateDeleteWorkflowExecution = "DELETE FROM executions_visibility WHERE domain_id=$1 AND run_id=$2"
)

var errCloseParams = errors.New("missing one of {closeStatus, closeTime, historyLength} params")

// InsertIntoVisibility inserts a row into visibility table. If an row already exist,
// its left as such and no update will be made
func (mdb *db) InsertIntoVisibility(row *sqldb.VisibilityRow) (sql.Result, error) {
	row.StartTime = mdb.converter.ToMySQLDateTime(row.StartTime)
	return mdb.conn.Exec(templateCreateWorkflowExecutionStarted,
		row.DomainID,
		row.WorkflowID,
		row.RunID,
		row.StartTime,
		row.ExecutionTime,
		row.WorkflowTypeName,
		row.Memo,
		row.Encoding)
}

// ReplaceIntoVisibility replaces an existing row if it exist or creates a new row in visibility table
func (mdb *db) ReplaceIntoVisibility(row *sqldb.VisibilityRow) (sql.Result, error) {
	switch {
	case row.CloseStatus != nil && row.CloseTime != nil && row.HistoryLength != nil:
		row.StartTime = mdb.converter.ToMySQLDateTime(row.StartTime)
		closeTime := mdb.converter.ToMySQLDateTime(*row.CloseTime)
		return mdb.conn.Exec(templateCreateWorkflowExecutionClosed,
			row.DomainID,
			row.WorkflowID,
			row.RunID,
			row.StartTime,
			row.ExecutionTime,
			row.WorkflowTypeName,
			closeTime,
			*row.CloseStatus,
			*row.HistoryLength,
			row.Memo,
			row.Encoding)
	default:
		return nil, errCloseParams
	}
}

// DeleteFromVisibility deletes a row from visibility table if it exist
func (mdb *db) DeleteFromVisibility(filter *sqldb.VisibilityFilter) (sql.Result, error) {
	return mdb.conn.Exec(templateDeleteWorkflowExecution, filter.DomainID, filter.RunID)
}

// SelectFromVisibility reads one or more rows from visibility table
func (mdb *db) SelectFromVisibility(filter *sqldb.VisibilityFilter) ([]sqldb.VisibilityRow, error) {
	var err error
	var rows []sqldb.VisibilityRow
	if filter.MinStartTime != nil {
		*filter.MinStartTime = mdb.converter.ToMySQLDateTime(*filter.MinStartTime)
	}
	if filter.MaxStartTime != nil {
		*filter.MaxStartTime = mdb.converter.ToMySQLDateTime(*filter.MaxStartTime)
	}
	switch {
	case filter.MinStartTime == nil && filter.RunID != nil && filter.Closed:
		var row sqldb.VisibilityRow
		err = mdb.conn.Get(&row, templateGetClosedWorkflowExecution, filter.DomainID, *filter.RunID)
		if err == nil {
			rows = append(rows, row)
		}
	case filter.MinStartTime != nil && filter.WorkflowID != nil:
		qry := templateGetOpenWorkflowExecutionsByID
		if filter.Closed {
			qry = templateGetClosedWorkflowExecutionsByID
		}
		err = mdb.conn.Select(&rows,
			qry,
			*filter.WorkflowID,
			filter.DomainID,
			mdb.converter.ToMySQLDateTime(*filter.MinStartTime),
			mdb.converter.ToMySQLDateTime(*filter.MaxStartTime),
			*filter.RunID,
			*filter.MinStartTime,
			*filter.PageSize)
	case filter.MinStartTime != nil && filter.WorkflowTypeName != nil:
		qry := templateGetOpenWorkflowExecutionsByType
		if filter.Closed {
			qry = templateGetClosedWorkflowExecutionsByType
		}
		err = mdb.conn.Select(&rows,
			qry,
			*filter.WorkflowTypeName,
			filter.DomainID,
			mdb.converter.ToMySQLDateTime(*filter.MinStartTime),
			mdb.converter.ToMySQLDateTime(*filter.MaxStartTime),
			*filter.RunID,
			*filter.MaxStartTime,
			*filter.PageSize)
	case filter.MinStartTime != nil && filter.CloseStatus != nil:
		err = mdb.conn.Select(&rows,
			templateGetClosedWorkflowExecutionsByStatus,
			*filter.CloseStatus,
			filter.DomainID,
			mdb.converter.ToMySQLDateTime(*filter.MinStartTime),
			mdb.converter.ToMySQLDateTime(*filter.MaxStartTime),
			*filter.RunID,
			mdb.converter.ToMySQLDateTime(*filter.MaxStartTime),
			*filter.PageSize)
	case filter.MinStartTime != nil:
		qry := templateGetOpenWorkflowExecutions
		if filter.Closed {
			qry = templateGetClosedWorkflowExecutions
		}
		err = mdb.conn.Select(&rows,
			qry,
			filter.DomainID,
			mdb.converter.ToMySQLDateTime(*filter.MinStartTime),
			mdb.converter.ToMySQLDateTime(*filter.MaxStartTime),
			*filter.RunID,
			mdb.converter.ToMySQLDateTime(*filter.MaxStartTime),
			*filter.PageSize)
	default:
		return nil, fmt.Errorf("invalid query filter")
	}
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].StartTime = mdb.converter.FromMySQLDateTime(rows[i].StartTime)
		rows[i].ExecutionTime = mdb.converter.FromMySQLDateTime(rows[i].ExecutionTime)
		if rows[i].CloseTime != nil {
			closeTime := mdb.converter.FromMySQLDateTime(*rows[i].CloseTime)
			rows[i].CloseTime = &closeTime
		}
		rows[i].RunID = strings.TrimSpace(rows[i].RunID)
		rows[i].WorkflowID = strings.TrimSpace(rows[i].WorkflowID)
	}
	return rows, err
}
