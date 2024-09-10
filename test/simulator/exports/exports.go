package exports

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
)

type NetworkInit = accumulated.NetworkInit

type LogLevel = config.LogLevel

var DefaultLogLevels = LogLevel{}.
	Parse(config.DefaultLogLevels).
	SetModule("sim", "info").
	SetModule("executor", "info").
	String()

type Config = config.Logging

// Database stuff
type Database = database.Database
type Batch = database.Batch
type Transaction = database.Transaction
type Account = database.Account

type TxnQuery = api.TxnQuery
