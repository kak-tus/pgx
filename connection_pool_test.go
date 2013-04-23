package pgx

import (
	"fmt"
	"testing"
)

func createConnectionPool(maxConnections int) *ConnectionPool {
	connectionOptions := ConnectionParameters{socket: "/private/tmp/.s.PGSQL.5432", user: "pgx_none", database: "pgx_test"}
	pool, err := NewConnectionPool(connectionOptions, maxConnections)
	if err != nil {
		panic("Unable to create connection pool")
	}
	return pool
}

func TestNewConnectionPool(t *testing.T) {
	connectionOptions := ConnectionParameters{socket: "/private/tmp/.s.PGSQL.5432", user: "pgx_none", database: "pgx_test"}
	pool, err := NewConnectionPool(connectionOptions, 5)
	if err != nil {
		t.Fatal("Unable to establish connection pool")
	}
	defer pool.Close()

	if pool.MaxConnections != 5 {
		t.Error("Wrong maxConnections")
	}
}

func TestPoolAcquireAndReleaseCycle(t *testing.T) {
	maxConnections := 2
	incrementCount := int32(100)
	completeSync := make(chan int)
	pool := createConnectionPool(maxConnections)
	defer pool.Close()

	acquireAll := func() (connections []*Connection) {
		connections = make([]*Connection, maxConnections)
		for i := 0; i < maxConnections; i++ {
			connections[i] = pool.Acquire()
		}
		return
	}

	allConnections := acquireAll()

	for _, c := range allConnections {
		var err error
		if _, err = c.Execute("create temporary table t(counter integer not null)"); err != nil {
			t.Fatal("Unable to create temp table:" + err.Error())
		}
		if _, err = c.Execute("insert into t(counter) values(0);"); err != nil {
			t.Fatal("Unable to insert initial counter row: " + err.Error())
		}
	}

	for _, c := range allConnections {
		pool.Release(c)
	}

	f := func() {
		var err error
		conn := pool.Acquire()
		if err != nil {
			t.Fatal("Unable to acquire connection")
		}
		defer pool.Release(conn)

		// Increment counter...
		_, err = conn.Execute("update t set counter = counter + 1")
		if err != nil {
			t.Fatal("Unable to update counter: " + err.Error())
		}
		completeSync <- 0
	}

	for i := int32(0); i < incrementCount; i++ {
		go f()
	}

	// Wait for all f() to complete
	for i := int32(0); i < incrementCount; i++ {
		<-completeSync
	}

	// Check that temp table in each connection has been incremented some number of times
	actualCount := int32(0)
	allConnections = acquireAll()

	for _, c := range allConnections {
		n, err := c.SelectInt32("select counter from t")
		if err != nil {
			t.Fatal("Unable to read back execution counter: " + err.Error())
		}

		if n == 0 {
			t.Error("A connection was never used")
		}

		actualCount += n
	}

	if actualCount != incrementCount {
		fmt.Println(actualCount)
		t.Error("Wrong number of increments")
	}

	for _, c := range allConnections {
		pool.Release(c)
	}
}
