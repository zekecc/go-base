package gdopool

import (
	"database/sql"
	"errors"
	"sync"

	base "github.com/jameschz/go-base/lib/base"
	gdobase "github.com/jameschz/go-base/lib/gdo/base"
	gdodriver "github.com/jameschz/go-base/lib/gdo/driver"
	gutil "github.com/jameschz/go-base/lib/gutil"

	// import mysql lib
	_ "github.com/go-sql-driver/mysql"
)

var (
	_debugStatus  bool
	_dbPoolInit   bool
	_dbPoolIdle   map[string]*base.Stack
	_dbPoolActive map[string]*base.Hmap
	_dbPoolLock   sync.Mutex
)

// private
func debugPrint(vals ...interface{}) {
	if _debugStatus == true {
		gutil.Dump(vals...)
	}
}

// private
func createDataSource(driver *gdodriver.Driver) *gdobase.DataSource {
	ds := &gdobase.DataSource{}
	ds.Name = driver.DbName
	ds.ID = gutil.UUID()
	// open db connection
	dsn := driver.User + ":" +
		driver.Pass + "@tcp(" +
		driver.Host + ":" +
		driver.Port + ")/" +
		driver.DbName + "?charset=" +
		driver.Charset
	switch driver.Type {
	case "mysql":
		dbc, err := sql.Open("mysql", dsn)
		if err != nil {
			panic("gdo> open db error")
		}
		ds.Conn = dbc
	}
	// for debug
	debugPrint("gdopool.createDataSource", ds)
	return ds
}

// private
func releaseDataSource(ds *gdobase.DataSource) {
	if ds != nil {
		ds.Conn.Close()
		ds = nil
	}
	// for debug
	debugPrint("gdopool.releaseDataSource", ds)
}

// SetDebug : public
func SetDebug(status bool) {
	_debugStatus = status
}

// Init : public
func Init() (err error) {
	// init once
	if _dbPoolInit == true {
		return nil
	}
	// init drivers
	gdodriver.Init()
	// init pool by drivers
	dbDrivers := gdodriver.GetDrivers()
	_dbPoolIdle = make(map[string]*base.Stack, 0)
	_dbPoolActive = make(map[string]*base.Hmap, 0)
	for dbName, dbDriver := range dbDrivers {
		_dbPoolIdle[dbName] = base.NewStack()
		_dbPoolActive[dbName] = base.NewHmap()
		for i := 0; i < dbDriver.PoolInitSize; i++ {
			_dbPoolIdle[dbName].Push(createDataSource(dbDriver))
		}
	}
	// for debug
	debugPrint("gdopool.Init", _dbPoolIdle, _dbPoolActive)
	// init ok status
	if err == nil {
		_dbPoolInit = true
	}
	return err
}

// Fetch : public
func Fetch(dbName string) (ds *gdobase.DataSource, err error) {
	// get driver by name
	dbDriver := gdodriver.GetDriver(dbName)
	// fetch start >>> lock
	_dbPoolLock.Lock()
	// reach to max active size
	activeSize := _dbPoolActive[dbName].Len()
	if dbDriver.PoolMaxActive <= activeSize {
		return nil, errors.New("gdopool : max active limit")
	}
	// add if not enough
	idleSize := _dbPoolIdle[dbName].Len()
	if dbDriver.PoolMinIdle >= idleSize {
		idleSizeAdd := dbDriver.PoolMaxIdle - idleSize
		for i := 0; i < idleSizeAdd; i++ {
			_dbPoolIdle[dbName].Push(createDataSource(dbDriver))
		}
		// for debug
		debugPrint("gdopool.Fetch Add", _dbPoolIdle[dbName].Len(), _dbPoolActive[dbName].Len())
	}
	// fetch from front
	if _dbPoolIdle[dbName].Len() >= 1 {
		ds = _dbPoolIdle[dbName].Pop().(*gdobase.DataSource)
		_dbPoolActive[dbName].Set(ds.ID, ds)
	} else {
		return nil, errors.New("gdopool : no enough ds")
	}
	// for debug
	debugPrint("gdopool.Fetch", _dbPoolIdle[dbName].Len(), _dbPoolActive[dbName].Len())
	// fetch end >>> unlock
	_dbPoolLock.Unlock()
	// return ds 0
	return ds, err
}

// Return : public
func Return(ds *gdobase.DataSource) (err error) {
	// get driver by name
	dbName := ds.Name
	dbDriver := gdodriver.GetDriver(dbName)
	// return start >>> lock
	_dbPoolLock.Lock()
	// delete from active list
	_dbPoolActive[dbName].Delete(ds.ID)
	// return or release
	idleSize := _dbPoolIdle[dbName].Len()
	if dbDriver.PoolMaxIdle <= idleSize {
		releaseDataSource(ds)
	} else {
		_dbPoolIdle[dbName].Push(ds)
	}

	// return end >>> unlock
	_dbPoolLock.Unlock()
	// for debug
	debugPrint("gdopool.Return", _dbPoolIdle[dbName].Len(), _dbPoolActive[dbName].Len())
	return err
}
