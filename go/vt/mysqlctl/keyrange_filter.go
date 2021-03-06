// Copyright 2012, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mysqlctl

import (
	"bytes"
	"strconv"

	log "github.com/golang/glog"
	"github.com/youtube/vitess/go/vt/key"
	"github.com/youtube/vitess/go/vt/mysqlctl/proto"
)

var KEYSPACE_ID_COMMENT = []byte("/* EMD keyspace_id:")
var SPACE = []byte(" ")

// KeyRangeFilterFunc returns a function that calls sendReply only if statements
// in the transaction match the specified keyrange. The resulting function can be
// passed into the BinlogStreamer: bls.Stream(file, pos, sendTransaction) ->
// bls.Stream(file, pos, KeyRangeFilterFunc(sendTransaction))
func KeyRangeFilterFunc(keyrange key.KeyRange, sendReply sendTransactionFunc) sendTransactionFunc {
	return func(reply *proto.BinlogTransaction) error {
		matched := false
		filtered := make([]proto.Statement, 0, len(reply.Statements))
		for _, statement := range reply.Statements {
			switch statement.Category {
			case proto.BL_SET:
				filtered = append(filtered, statement)
			case proto.BL_DDL:
				filtered = append(filtered, statement)
				matched = true
			case proto.BL_DML:
				keyspaceIndex := bytes.LastIndex(statement.Sql, KEYSPACE_ID_COMMENT)
				if keyspaceIndex == -1 {
					updateStreamErrors.Add("KeyRangeStream", 1)
					log.Errorf("Error parsing keyspace id: %s", string(statement.Sql))
					continue
				}
				idstart := keyspaceIndex + len(KEYSPACE_ID_COMMENT)
				idend := bytes.Index(statement.Sql[idstart:], SPACE)
				if idend == -1 {
					updateStreamErrors.Add("KeyRangeStream", 1)
					log.Errorf("Error parsing keyspace id: %s", string(statement.Sql))
					continue
				}
				id, err := strconv.ParseUint(string(statement.Sql[idstart:idstart+idend]), 10, 64)
				if err != nil {
					updateStreamErrors.Add("KeyRangeStream", 1)
					log.Errorf("Error parsing keyspace id: %s", string(statement.Sql))
					continue
				}
				if !keyrange.Contains(key.Uint64Key(id).KeyspaceId()) {
					continue
				}
				filtered = append(filtered, statement)
				matched = true
			}
		}
		if matched {
			reply.Statements = filtered
		} else {
			reply.Statements = nil
		}
		return sendReply(reply)
	}
}
