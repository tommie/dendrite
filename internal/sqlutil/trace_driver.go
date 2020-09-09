// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build !wasm

package sqlutil

import (
	"database/sql"
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/lib/pq"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/ngrok/sqlmw"
)

func registerDrivers() {
	if !tracingEnabled {
		return
	}
	// install the wrapped drivers
	sql.Register("postgres-trace", sqlmw.Driver(&pq.Driver{}, new(traceInterceptor)))

	//sql.Register("sqlite3-trace", sqlmw.Driver(&sqlite.SQLiteDriver{}, new(traceInterceptor)))

	eventMask := sqlite3.TraceStmt | sqlite3.TraceProfile | sqlite3.TraceRow | sqlite3.TraceClose

	sql.Register("sqlite3-trace",
		&sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				err := conn.SetTrace(&sqlite3.TraceConfig{
					Callback:        traceCallback,
					EventMask:       eventMask,
					WantExpandedSQL: true,
				})
				return err
			},
		})

}

func traceCallback(info sqlite3.TraceInfo) int {
	// Not very readable but may be useful; uncomment next line in case of doubt:
	//fmt.Printf("Trace: %#v\n", info)

	var dbErrText string
	if info.DBError.Code != 0 || info.DBError.ExtendedCode != 0 {
		dbErrText = fmt.Sprintf("; DB error: %#v", info.DBError)
	} else {
		dbErrText = "."
	}

	// Show the Statement-or-Trigger text in curly braces ('{', '}')
	// since from the *paired* ASCII characters they are
	// the least used in SQL syntax, therefore better visual delimiters.
	// Maybe show 'ExpandedSQL' the same way as 'StmtOrTrigger'.
	//
	// A known use of curly braces (outside strings) is
	// for ODBC escape sequences. Not likely to appear here.
	//
	// Template languages, etc. don't matter, we should see their *result*
	// at *this* level.
	// Strange curly braces in SQL code that reached the database driver
	// suggest that there is a bug in the application.
	// The braces are likely to be either template syntax or
	// a programming language's string interpolation syntax.

	var expandedText string
	if info.ExpandedSQL != "" {
		if info.ExpandedSQL == info.StmtOrTrigger {
			expandedText = " = exp"
		} else {
			expandedText = fmt.Sprintf(" expanded {%q}", info.ExpandedSQL)
		}
	} else {
		expandedText = ""
	}

	// SQLite docs as of September 6, 2016: Tracing and Profiling Functions
	// https://www.sqlite.org/c3ref/profile.html
	//
	// The profile callback time is in units of nanoseconds, however
	// the current implementation is only capable of millisecond resolution
	// so the six least significant digits in the time are meaningless.
	// Future versions of SQLite might provide greater resolution on the profiler callback.

	var runTimeText string
	if info.RunTimeNanosec == 0 {
		if info.EventCode == sqlite3.TraceProfile {
			//runTimeText = "; no time" // seems confusing
			runTimeText = "; time 0" // no measurement unit
		} else {
			//runTimeText = "; no time" // seems useless and confusing
		}
	} else {
		const nanosPerMillisec = 1000000
		if info.RunTimeNanosec%nanosPerMillisec == 0 {
			runTimeText = fmt.Sprintf("; time %d ms", info.RunTimeNanosec/nanosPerMillisec)
		} else {
			// unexpected: better than millisecond resolution
			runTimeText = fmt.Sprintf("; time %d ns!!!", info.RunTimeNanosec)
		}
	}

	var modeText string
	if info.AutoCommit {
		modeText = "-AC-"
	} else {
		modeText = "+Tx+"
	}

	log.Infof("Trace: ev %d %s conn 0x%x, stmt 0x%x {%q}%s%s%s\n",
		info.EventCode, modeText, info.ConnHandle, info.StmtHandle,
		info.StmtOrTrigger, expandedText,
		runTimeText,
		dbErrText)
	return 0
}