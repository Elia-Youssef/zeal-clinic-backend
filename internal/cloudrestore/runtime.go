package cloudrestore

import (
	"database/sql"
	"sync"
)

// Runtime controls the database-backed background services. The server binds
// its sync engine and monitor here so a restore can stop them before closing
// pools and recreate them against the reopened database.
type Runtime interface {
	Pause()
	Resume(*sql.DB)
}

var runtimeBinding struct {
	sync.RWMutex
	runtime Runtime
}

func SetRuntime(r Runtime) {
	runtimeBinding.Lock()
	runtimeBinding.runtime = r
	runtimeBinding.Unlock()
}

func currentRuntime() Runtime {
	runtimeBinding.RLock()
	defer runtimeBinding.RUnlock()
	return runtimeBinding.runtime
}
