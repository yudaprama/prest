package connection

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/prest/prest/v2/config"

	"github.com/jmoiron/sqlx"
	// Used pg drive on sqlx
	_ "github.com/lib/pq"
)

var (
	pool         *Pool
	currDatabase string
)

// Pool struct
type Pool struct {
	Mtx      *sync.Mutex
	DB       map[string]*sqlx.DB
	RealName map[string]string // logical name → actual database name for SQL qualification
}

// poolKey returns a stable, safe map key for a DSN. The raw DSN (which
// may contain a password) is hashed so the password is not kept as a
// second copy in memory via the map key.
func poolKey(dsn string) string {
	h := sha256.Sum256([]byte(dsn))
	return hex.EncodeToString(h[:])
}

// nameKey returns a pool key for named connections (registered via AddURI).
// The "n:" prefix prevents collisions with poolKey hashes.
func nameKey(name string) string {
	return "n:" + name
}

// GetURI postgres connection URI
func GetURI(DBName string) string {
	if DBName == "" {
		DBName = config.PrestConf.PGDatabase
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(config.PrestConf.PGUser, config.PrestConf.PGPass),
		Host:   config.PrestConf.PGHost + ":" + strconv.Itoa(config.PrestConf.PGPort),
		Path:   DBName,
	}
	q := u.Query()
	q.Set("sslmode", config.PrestConf.PGSSLMode)
	q.Set("connect_timeout", strconv.Itoa(config.PrestConf.PGConnTimeout))
	if config.PrestConf.PGSSLCert != "" {
		q.Set("sslcert", config.PrestConf.PGSSLCert)
	}
	if config.PrestConf.PGSSLKey != "" {
		q.Set("sslkey", config.PrestConf.PGSSLKey)
	}
	if config.PrestConf.PGSSLRootCert != "" {
		q.Set("sslrootcert", config.PrestConf.PGSSLRootCert)
	}
	u.RawQuery = q.Encode()

	return u.String()
}

// Get get Postgres connection adding it to the pool if needed
func Get() (*sqlx.DB, error) {
	DB := getDatabaseFromPool(GetDatabase())
	// Connection is already in the pool
	if DB != nil {
		return DB, nil
	}

	// Connection is not in the pool, add it
	DB, err := AddDatabaseToPool(GetDatabase())

	return DB, err
}

// GetFromPool tries to get the db name from the db pool
// will return error if not found
func GetFromPool(dbName string) (*sqlx.DB, error) {
	DB := getDatabaseFromPool(dbName)
	if DB == nil {
		return nil, errors.New("db not found in pool")
	}
	return DB, nil
}

// GetPool of connection
func GetPool() *Pool {
	if pool == nil {
		pool = &Pool{
			Mtx:      &sync.Mutex{},
			DB:       make(map[string]*sqlx.DB),
			RealName: make(map[string]string),
		}
	}
	return pool
}

// ResolveDBName returns the actual database name for a logical/alias name.
// The SQL builder qualifies identifiers as "database"."schema"."table", so a
// logical alias (e.g. "plano") must be translated to the real database name
// (e.g. "postgres") that the connection is actually bound to. The mapping is
// resolved in this order:
//  1. An explicit registration in the pool (RealName).
//  2. The default PGDatabase from the active configuration.
//  3. The input name unchanged.
func ResolveDBName(name string) string {
	if name == "" {
		return name
	}
	p := GetPool()
	p.Mtx.Lock()
	if real, ok := p.RealName[name]; ok && real != "" {
		p.Mtx.Unlock()
		return real
	}
	p.Mtx.Unlock()
	if cfg := config.PrestConf; cfg != nil && cfg.PGDatabase != "" {
		return cfg.PGDatabase
	}
	return name
}

func getDatabaseFromPool(name string) *sqlx.DB {
	p := GetPool()
	p.Mtx.Lock()
	DB := p.DB[poolKey(GetURI(name))]
	if DB == nil {
		DB = p.DB[nameKey(name)]
	}
	p.Mtx.Unlock()
	return DB
}

// AddDatabaseToPool create and add connection to the pool
func AddDatabaseToPool(name string) (*sqlx.DB, error) {
	dsn := GetURI(name)
	key := poolKey(dsn)
	p := GetPool()

	// Fast path: already in pool
	p.Mtx.Lock()
	if existing, ok := p.DB[key]; ok {
		p.Mtx.Unlock()
		return existing, nil
	}
	p.Mtx.Unlock()

	// Slow path: connect without holding the lock
	DB, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, err
	}
	DB.SetMaxIdleConns(config.PrestConf.PGMaxIdleConn)
	DB.SetMaxOpenConns(config.PrestConf.PGMaxOpenConn)

	// Re-lock to store; close duplicate if another goroutine won the race
	p.Mtx.Lock()
	if existing, ok := p.DB[key]; ok {
		p.Mtx.Unlock()
		DB.Close()
		return existing, nil
	}
	p.DB[key] = DB
	p.Mtx.Unlock()
	return DB, nil
}

// MustGet get postgres connection
func MustGet() *sqlx.DB {
	var err error
	var DB *sqlx.DB

	DB, err = Get()
	if err != nil {
		slog.Error("Unable to connect to database", "error", err)
		panic(err)
	}
	return DB
}

// AddURI registers a database connection built from a raw DSN (e.g.
// "postgres://user:pass@host:5432/db?sslmode=disable") and stores it in the
// pool. Unlike AddDatabaseToPool, this does not derive the connection string
// from the global config and therefore supports fully independent connection
// strings supplied by the caller (multiple databases on different hosts,
// separate credentials, etc.).
func AddURI(name, dsn string) (*sqlx.DB, error) {
	if name == "" {
		name = dsn
	}
	if dsn == "" {
		return nil, errors.New("empty dsn")
	}

	key := poolKey(dsn)
	p := GetPool()

	// Fast path: already in pool
	nk := nameKey(name)
	p.Mtx.Lock()
	if existing, ok := p.DB[key]; ok {
		p.Mtx.Unlock()
		return existing, nil
	}
	if existing, ok := p.DB[nk]; ok {
		p.Mtx.Unlock()
		return existing, nil
	}
	p.Mtx.Unlock()

	// Slow path: connect without holding the lock
	DB, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("connect %q: %w", name, err)
	}
	DB.SetMaxIdleConns(config.PrestConf.PGMaxIdleConn)
	DB.SetMaxOpenConns(config.PrestConf.PGMaxOpenConn)

	// Re-lock to store; close duplicate if another goroutine won the race
	p.Mtx.Lock()
	if existing, ok := p.DB[key]; ok {
		p.Mtx.Unlock()
		DB.Close()
		return existing, nil
	}
	p.DB[key] = DB
	p.DB[nk] = DB
	p.Mtx.Unlock()

	slog.Info("registered extra database connection", "name", name, "dsn", redact(dsn))
	return DB, nil
}

// SetRealName registers a mapping from a logical/alias name to the actual
// database name that the connection is bound to. The SQL builder qualifies
// identifiers with the actual database name, so this mapping is required
// when a caller uses an alias to refer to a connection whose underlying
// database name is different.
func SetRealName(logical, actual string) {
	if logical == "" || actual == "" {
		return
	}
	p := GetPool()
	p.Mtx.Lock()
	p.RealName[logical] = actual
	p.Mtx.Unlock()
}

// getFromPoolByDSN looks up a connection by its raw DSN string.
func getFromPoolByDSN(dsn string) (*sqlx.DB, error) {
	p := GetPool()
	p.Mtx.Lock()
	DB := p.DB[poolKey(dsn)]
	p.Mtx.Unlock()
	if DB == nil {
		return nil, errors.New("db not found in pool")
	}
	return DB, nil
}

func redact(dsn string) string {
	// Mask the password segment between "://" and "@" so secrets are not
	// printed in logs.
	scheme := strings.Index(dsn, "://")
	if scheme < 0 {
		return dsn
	}
	at := strings.Index(dsn[scheme+3:], "@")
	if at < 0 {
		return dsn
	}
	at += scheme + 3
	user := dsn[scheme+3 : at]
	colon := strings.LastIndex(user, ":")
	if colon < 0 {
		return dsn
	}
	return dsn[:scheme+3] + user[:colon+1] + "****" + dsn[at:]
}

// SetDatabase set current database in use
func SetDatabase(name string) {
	currDatabase = name
}

// GetDatabase get current database in use
func GetDatabase() string {
	return currDatabase
}
