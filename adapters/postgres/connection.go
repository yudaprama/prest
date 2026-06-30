package postgres

import (
	"github.com/prest/prest/v2/adapters/postgres/internal/connection"

	"github.com/jmoiron/sqlx"
)

// GetURI postgres connection URI
func GetURI(DBName string) string {
	return connection.GetURI(DBName)
}

// Get get postgres connection
func Get() (*sqlx.DB, error) {
	return connection.Get()
}

// GetByName returns the connection registered under a logical name (e.g. one
// registered via [[pg.urls]] at startup, which AddURI stores under a name key).
// Unlike Get(), it does not depend on the shared "current database" global, so
// it is safe for handlers mounted outside the per-CRUD middleware chain (which
// never calls SetDatabase). Returns an error if the name is not in the pool.
func GetByName(name string) (*sqlx.DB, error) {
	return connection.GetFromPool(name)
}

// GetPool of connection
func GetPool() *connection.Pool {
	return connection.GetPool()
}

// AddDatabaseToPool add connection to pool
func AddDatabaseToPool(name string) (*sqlx.DB, error) {
	return connection.AddDatabaseToPool(name)
}

// SetRealName maps a logical/alias name to the actual database name that
// the connection is bound to. Required when a connection's pool name does
// not match the underlying PostgreSQL database name, so the SQL builder
// qualifies identifiers correctly.
func SetRealName(logical, actual string) {
	connection.SetRealName(logical, actual)
}

// AddURI registers a database connection built from a raw DSN and stores
// it in the pool keyed by the DSN itself. This exposes connection.AddURI
// so that packages outside adapters (e.g. cmd/prestd) can register
// multiple independently-configured connection strings at startup.
func AddURI(name, dsn string) (*sqlx.DB, error) {
	return connection.AddURI(name, dsn)
}

// MustGet get postgres connection
func MustGet() *sqlx.DB {
	return connection.MustGet()
}

// SetDatabase set current database in use
// todo: remove when ctx is fully implemented
func SetDatabase(name string) {
	connection.SetDatabase(name)
}

// GetDatabase get current database in use
func GetDatabase() string {
	return connection.GetDatabase()
}
