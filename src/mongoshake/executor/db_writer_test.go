package executor

import (
	"testing"
	"fmt"

	"mongoshake/common"
	"mongoshake/oplog"
	"mongoshake/unit_test_common"

	"github.com/stretchr/testify/assert"
	"github.com/vinllen/mgo/bson"
	"github.com/vinllen/mgo"
)

const (
	testMongoAddress = unit_test_common.TestUrl
	testDb           = "writer_test"
	testCollection   = "a"
)

func mockOplogRecord(oId, oX int, o2Id int) *OplogRecord {
	or := &OplogRecord{
		original: &PartialLogWithCallbak {
			partialLog: &oplog.PartialLog {
				ParsedLog: oplog.ParsedLog {
					Object: bson.D{
						bson.DocElem{
							Name: "_id",
							Value: oId,
						},
						bson.DocElem{
							Name: "x",
							Value: oX,
						},
					},
				},
			},
		},
	}

	if o2Id != -1 {
		or.original.partialLog.ParsedLog.Query = bson.M {
			"_id": o2Id,
		}
	}

	return or
}

func TestSingleWriter(t *testing.T) {
	// test single writer

	var nr int

	// simple test
	{
		fmt.Printf("TestSingleWriter case %d.\n", nr)
		nr++

		conn, err := utils.NewMongoConn(testMongoAddress, "primary", true)
		assert.Equal(t, nil, err, "should be equal")

		// drop database
		err = conn.Session.DB(testDb).DropDatabase()
		assert.Equal(t, nil, err, "should be equal")

		writer := NewDbWriter(conn.Session, bson.M{}, false)

		inserts := []*OplogRecord{mockOplogRecord(1, 1, -1)}

		// write 1
		err = writer.doInsert(testDb, testCollection, bson.M{}, inserts, false)
		assert.Equal(t, nil, err, "should be equal")

		// update 1->2
		err = writer.doUpdate(testDb, testCollection, bson.M{}, []*OplogRecord{
			mockOplogRecord(1, 10, 1),
		}, false)
		assert.Equal(t, nil, err, "should be equal")

		// query
		result := make([]interface{}, 0)
		err = conn.Session.DB(testDb).C(testCollection).Find(bson.M{}).All(&result)
		assert.Equal(t, nil, err, "should be equal")
		assert.Equal(t, 1, len(result), "should be equal")
		assert.Equal(t, 10, result[0].(bson.M)["x"], "should be equal")

		// delete 2
		err = writer.doDelete(testDb, testCollection, bson.M{}, []*OplogRecord{
			mockOplogRecord(1, 1, 1),
		})
		assert.Equal(t, nil, err, "should be equal")

		// query
		result = make([]interface{}, 0)
		err = conn.Session.DB(testDb).C(testCollection).Find(bson.M{}).All(&result)
		assert.Equal(t, nil, err, "should be equal")
		assert.Equal(t, 0, len(result), "should be equal")
	}

	// test upsert, dupInsert
	{
		fmt.Printf("TestSingleWriter case %d.\n", nr)
		nr++

		conn, err := utils.NewMongoConn(testMongoAddress, "primary", true)
		assert.Equal(t, nil, err, "should be equal")

		writer := NewDbWriter(conn.Session, bson.M{}, false)

		// drop database
		err = conn.Session.DB(testDb).DropDatabase()
		assert.Equal(t, nil, err, "should be equal")

		// 1-5
		inserts := []*OplogRecord{
			mockOplogRecord(1, 1, -1),
			mockOplogRecord(2, 2, -1),
			mockOplogRecord(3, 3, -1),
			mockOplogRecord(4, 4, -1),
			mockOplogRecord(5, 5, -1),
		}

		// write 1
		err = writer.doInsert(testDb, testCollection, bson.M{}, inserts, false)
		assert.Equal(t, nil, err, "should be equal")

		// update 6->10
		err = writer.doUpdate(testDb, testCollection, bson.M{}, []*OplogRecord{
			mockOplogRecord(6, 10, 6),
		}, false)
		assert.NotEqual(t, nil, err, "should be equal")

		// upsert 6->10
		err = writer.doUpdate(testDb, testCollection, bson.M{}, []*OplogRecord{
			mockOplogRecord(6, 10, 6),
		}, true)
		assert.Equal(t, nil, err, "should be equal")

		// query
		result := make([]interface{}, 0)
		err = conn.Session.DB(testDb).C(testCollection).Find(bson.M{}).Sort("_id").All(&result)
		assert.Equal(t, nil, err, "should be equal")
		assert.Equal(t, 6, len(result), "should be equal")
		assert.Equal(t, 1, result[0].(bson.M)["x"], "should be equal")
		assert.Equal(t, 2, result[1].(bson.M)["x"], "should be equal")
		assert.Equal(t, 3, result[2].(bson.M)["x"], "should be equal")
		assert.Equal(t, 4, result[3].(bson.M)["x"], "should be equal")
		assert.Equal(t, 5, result[4].(bson.M)["x"], "should be equal")
		assert.Equal(t, 10, result[5].(bson.M)["x"], "should be equal")
		assert.Equal(t, 6, result[5].(bson.M)["_id"], "should be equal")

		// dupInsert but ignore
		err = writer.doInsert(testDb, testCollection, bson.M{}, []*OplogRecord{
			mockOplogRecord(1, 30, 1),
		}, false)
		assert.Equal(t, nil, err, "should be equal")

		// dupInsert -> update
		err = writer.doInsert(testDb, testCollection, bson.M{}, []*OplogRecord{
			mockOplogRecord(1, 30, 1),
		}, true)
		assert.Equal(t, nil, err, "should be equal")

		// query
		result = make([]interface{}, 0)
		err = conn.Session.DB(testDb).C(testCollection).Find(bson.M{}).Sort("_id").All(&result)
		assert.Equal(t, nil, err, "should be equal")
		assert.Equal(t, 6, len(result), "should be equal")
		assert.Equal(t, 30, result[0].(bson.M)["x"], "should be equal")

		// delete not found
		err = writer.doDelete(testDb, testCollection, bson.M{}, []*OplogRecord{
			mockOplogRecord(20, 20, 20),
		})
		assert.NotEqual(t, nil, err, "should be equal")
	}
}

func TestBulkWriter(t *testing.T) {
	// test bulk writer

	var nr int

	// basic test
	{
		fmt.Printf("TestBulkWriter case %d.\n", nr)
		nr++

		conn, err := utils.NewMongoConn(testMongoAddress, "primary", true)
		assert.Equal(t, nil, err, "should be equal")

		// drop database
		err = conn.Session.DB(testDb).DropDatabase()
		assert.Equal(t, nil, err, "should be equal")

		writer := NewDbWriter(conn.Session, bson.M{}, true)

		// 1-5
		inserts := []*OplogRecord{
			mockOplogRecord(1, 1, -1),
			mockOplogRecord(2, 2, -1),
			mockOplogRecord(3, 3, -1),
			mockOplogRecord(4, 4, -1),
			mockOplogRecord(5, 5, -1),
		}

		// write 1
		err = writer.doInsert(testDb, testCollection, bson.M{}, inserts, false)
		assert.Equal(t, nil, err, "should be equal")

		// 4-8
		inserts = []*OplogRecord{
			mockOplogRecord(4, 4, -1),
			mockOplogRecord(5, 5, -1),
			mockOplogRecord(6, 6, -1),
			mockOplogRecord(7, 7, -1),
			mockOplogRecord(8, 8, -1),
		}

		// write 1
		err = writer.doInsert(testDb, testCollection, bson.M{}, inserts, false)
		assert.Equal(t, nil, err, "should be equal")

		// query
		result := make([]interface{}, 0)
		err = conn.Session.DB(testDb).C(testCollection).Find(bson.M{}).Sort("_id").All(&result)
		assert.Equal(t, nil, err, "should be equal")
		assert.Equal(t, 8, len(result), "should be equal")
		assert.Equal(t, 1, result[0].(bson.M)["x"], "should be equal")
		assert.Equal(t, 2, result[1].(bson.M)["x"], "should be equal")
		assert.Equal(t, 3, result[2].(bson.M)["x"], "should be equal")
		assert.Equal(t, 4, result[3].(bson.M)["x"], "should be equal")
		assert.Equal(t, 5, result[4].(bson.M)["x"], "should be equal")
		assert.Equal(t, 6, result[5].(bson.M)["x"], "should be equal")
		assert.Equal(t, 7, result[6].(bson.M)["x"], "should be equal")
		assert.Equal(t, 8, result[7].(bson.M)["x"], "should be equal")

		// 8-10
		inserts = []*OplogRecord{
			mockOplogRecord(8, 80, -1),
			mockOplogRecord(9, 90, -1),
			mockOplogRecord(10, 100, -1),
		}

		// write 1
		err = writer.doInsert(testDb, testCollection, bson.M{}, inserts, true)
		assert.Equal(t, nil, err, "should be equal")

		// query
		result = make([]interface{}, 0)
		err = conn.Session.DB(testDb).C(testCollection).Find(bson.M{}).Sort("_id").All(&result)
		assert.Equal(t, nil, err, "should be equal")
		assert.Equal(t, 10, len(result), "should be equal")
		assert.Equal(t, 80, result[7].(bson.M)["x"], "should be equal")
		assert.Equal(t, 90, result[8].(bson.M)["x"], "should be equal")
		assert.Equal(t, 100, result[9].(bson.M)["x"], "should be equal")

		// delete 8-11
		deletes := []*OplogRecord{
			mockOplogRecord(8, 80, -1),
			mockOplogRecord(9, 90, -1),
			mockOplogRecord(10, 100, -1),
			mockOplogRecord(11, 110, -1), // not found
		}
		err = writer.doDelete(testDb, testCollection, bson.M{}, deletes)
		assert.Equal(t, nil, err, "should be equal") // won't throw error if not found

		result = make([]interface{}, 0)
		err = conn.Session.DB(testDb).C(testCollection).Find(bson.M{}).Sort("_id").All(&result)
		assert.Equal(t, nil, err, "should be equal")
		assert.Equal(t, 7, len(result), "should be equal")
	}

	// bulk update, delete
	{
		fmt.Printf("TestBulkWriter case %d.\n", nr)
		nr++

		conn, err := utils.NewMongoConn(testMongoAddress, "primary", true)
		assert.Equal(t, nil, err, "should be equal")

		// drop database
		err = conn.Session.DB(testDb).DropDatabase()
		assert.Equal(t, nil, err, "should be equal")

		writer := NewDbWriter(conn.Session, bson.M{}, true)

		// 1-5
		inserts := []*OplogRecord{
			mockOplogRecord(1, 1, -1),
			mockOplogRecord(2, 2, -1),
			mockOplogRecord(3, 3, -1),
			mockOplogRecord(4, 4, -1),
			mockOplogRecord(5, 5, -1),
		}

		// write 1
		err = writer.doInsert(testDb, testCollection, bson.M{}, inserts, false)
		assert.Equal(t, nil, err, "should be equal")

		// update not exist
		updates := []*OplogRecord{
			mockOplogRecord(5, 50, 5),
			mockOplogRecord(10, 100, 10),
			mockOplogRecord(11, 110, 11),
		}

		// not work
		err = writer.doUpdate(testDb, testCollection, bson.M{}, updates, false)
		assert.Equal(t, nil, err, "should be equal")

		result := make([]interface{}, 0)
		err = conn.Session.DB(testDb).C(testCollection).Find(bson.M{}).Sort("_id").All(&result)
		assert.Equal(t, nil, err, "should be equal")
		assert.Equal(t, 5, len(result), "should be equal")
		assert.Equal(t, 50, result[4].(bson.M)["x"], "should be equal")

		// updates
		updates = []*OplogRecord{
			mockOplogRecord(4, 40, 4),
			mockOplogRecord(10, 100, 10),
			mockOplogRecord(11, 110, 11),
		}

		// upsert
		err = writer.doUpdate(testDb, testCollection, bson.M{}, updates, true)
		assert.Equal(t, nil, err, "should be equal")

		result = make([]interface{}, 0)
		err = conn.Session.DB(testDb).C(testCollection).Find(bson.M{}).Sort("_id").All(&result)
		assert.Equal(t, nil, err, "should be equal")
		assert.Equal(t, 7, len(result), "should be equal")
		assert.Equal(t, 40, result[3].(bson.M)["x"], "should be equal")
		assert.Equal(t, 50, result[4].(bson.M)["x"], "should be equal")
		assert.Equal(t, 100, result[5].(bson.M)["x"], "should be equal")
		assert.Equal(t, 110, result[6].(bson.M)["x"], "should be equal")

		// deletes
		deletes := []*OplogRecord{
			mockOplogRecord(1, 1, -1),
			mockOplogRecord(2, 2, -1),
			mockOplogRecord(999, 999, -1), // not exist
		}

		err = writer.doDelete(testDb, testCollection, bson.M{}, deletes)
		result = make([]interface{}, 0)
		err = conn.Session.DB(testDb).C(testCollection).Find(bson.M{}).Sort("_id").All(&result)
		assert.Equal(t, nil, err, "should be equal")
		assert.Equal(t, 5, len(result), "should be equal")
	}

	// bulk update, delete
	{
		fmt.Printf("TestBulkWriter case %d.\n", nr)
		nr++

		conn, err := utils.NewMongoConn(testMongoAddress, "primary", true)
		assert.Equal(t, nil, err, "should be equal")

		// drop database
		err = conn.Session.DB(testDb).DropDatabase()
		assert.Equal(t, nil, err, "should be equal")

		writer := NewDbWriter(conn.Session, bson.M{}, true)

		// 1-5
		inserts := []*OplogRecord{
			mockOplogRecord(1, 1, -1),
			mockOplogRecord(2, 2, -1),
			mockOplogRecord(3, 3, -1),
			mockOplogRecord(4, 4, -1),
			mockOplogRecord(5, 5, -1),
		}

		// write 1
		err = writer.doInsert(testDb, testCollection, bson.M{}, inserts, false)
		assert.Equal(t, nil, err, "should be equal")

		// build index
		err = conn.Session.DB(testDb).C(testCollection).EnsureIndex(mgo.Index{
			Key: []string{"x"},
			Unique: true,
		})
		assert.Equal(t, nil, err, "should be equal")

		// updates
		updates := []*OplogRecord{
			mockOplogRecord(3, 5, 3), // dup
			mockOplogRecord(10, 100, 10),
			mockOplogRecord(11, 110, 11),
		}

		// upsert = false
		err = writer.doUpdate(testDb, testCollection, bson.M{}, updates, false)
		assert.NotEqual(t, nil, err, "should be equal")
		fmt.Println(err)

		result := make([]interface{}, 0)
		err = conn.Session.DB(testDb).C(testCollection).Find(bson.M{}).Sort("_id").All(&result)
		assert.Equal(t, nil, err, "should be equal")
		assert.Equal(t, 5, len(result), "should be equal")
		assert.Equal(t, 3, result[2].(bson.M)["x"], "should be equal")

		// upsert
		err = writer.doUpdate(testDb, testCollection, bson.M{}, updates, true)
		assert.Equal(t, nil, err, "should be equal")

		result = make([]interface{}, 0)
		err = conn.Session.DB(testDb).C(testCollection).Find(bson.M{}).Sort("_id").All(&result)
		assert.Equal(t, nil, err, "should be equal")
		assert.Equal(t, 7, len(result), "should be equal")
		assert.Equal(t, 3, result[2].(bson.M)["x"], "should be equal")
		assert.Equal(t, 100, result[5].(bson.M)["x"], "should be equal")
		assert.Equal(t, 110, result[6].(bson.M)["x"], "should be equal")
	}
}