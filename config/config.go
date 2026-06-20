package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/prest/prest/v2/adapters"
	"github.com/prest/prest/v2/cache"
	"github.com/structy/log"

	"log/slog"

	"github.com/joho/godotenv"
	"github.com/lestrrat-go/jwx/v2/jwk"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

const (
	jsonAggDefault = "jsonb_agg"
	jsonAgg        = "json_agg"
)

// TablesConf informations

// UserFilterConfig declares a tenant filter that pREST auto-injects
// into every read against `{database}/{schema}/{table}`. The
// `column` is the table column that holds the per-user identifier
// (typically `user_id`); pREST appends `WHERE <column> = <id>` to
// the query, where `<id>` is read from the request context key
// `prest/context.UserIDKey`.
//
// The auth middleware is responsible for setting that context value
// (see the kratos integration for the Ory Kratos example). When no
// matching entry is found, or when the context value is empty, the
// filter is silently skipped — callers are expected to enforce
// authentication upstream.
type UserFilterConfig struct {
	Database string `mapstructure:"database"`
	Schema   string `mapstructure:"schema"`
	Table    string `mapstructure:"table"`
	Column   string `mapstructure:"column"`
}

// WorkspaceFilterConfig declares a tenant filter for workspace tables.
// pREST auto-injects `WHERE <column> IN (<user_workspace_list>)` for
// every read/write against the configured table. The list comes from
// the request context under `prest/context.WorkspaceIDsKey`, which is
// populated by the WorkspaceMembershipResolver middleware.
type WorkspaceFilterConfig struct {
	Database string `mapstructure:"database"`
	Schema   string `mapstructure:"schema"`
	Table    string `mapstructure:"table"`
	Column   string `mapstructure:"column"`
}

// WorkspaceCompatConfig declares the active-workspace ("compat") filter for
// a workspace-capable content table. It mirrors LobeHub's buildWorkspaceWhere
// exactly, replacing the plain user_id filter on that table:
//   - active workspace present (WorkspaceIDActiveKey non-empty, from the
//     X-Workspace-Id header set by the BFF after its Keto Check):
//     WHERE <workspace_column> = $ws
//   - personal mode (no active workspace):
//     WHERE <user_column> = $uid AND <workspace_column> IS NULL
//
// A table MUST NOT appear in both [[auth.user_id_filters]] and
// [[auth.workspace_compat_filters]]; Parse() rejects the overlap so each
// table gets exactly one filter. Unlike the union-membership workspace
// filter, this mode makes NO Keto calls on the read path — the active
// workspace is a trusted, pre-authorized header (same trust model as
// X-User-Id).
type WorkspaceCompatConfig struct {
	Database        string `mapstructure:"database"`
	Schema          string `mapstructure:"schema"`
	Table           string `mapstructure:"table"`
	UserColumn      string `mapstructure:"user_column"`
	WorkspaceColumn string `mapstructure:"workspace_column"`
}

type TablesConf struct {
	Name        string   `mapstructure:"name"`
	Permissions []string `mapstructure:"permissions"`
	Fields      []string `mapstructure:"fields"`
}

type UsersConf struct {
	Name   string `mapstructure:"name"`
	Tables []TablesConf
}

// AccessConf informations
type AccessConf struct {
	Restrict    bool
	IgnoreTable []string
	Tables      []TablesConf
	Users       []UsersConf
}

// ExposeConf (expose data) information
type ExposeConf struct {
	Enabled         bool
	DatabaseListing bool
	SchemaListing   bool
	TableListing    bool
}

type PluginMiddleware struct {
	File string
	Func string
}

// PGURLConfig holds a named database URL for multi-connection setups.
type PGURLConfig struct {
	Name string `mapstructure:"name"`
	URL  string `mapstructure:"url"`
}

// Prest basic config
type Prest struct {
	AuthEnabled          bool
	AuthSchema           string
	AuthTable            string
	AuthUsername         string
	AuthPassword         string
	AuthEncrypt          string
	AuthMetadata         []string
	AuthType             string
	UserIDHeader         string
	UserIDFilters         []UserFilterConfig
	WorkspaceIDFilters    []WorkspaceFilterConfig
	HTTPHost              string // HTTPHost Declare which http address the PREST used
	HTTPPort             int    // HTTPPort Declare which http port the PREST used
	HTTPTimeout          int
	PGHost               string
	PGPort               int
	PGUser               string
	PGPass               string
	PGDatabase           string
	PGURL                string
	PGURLs               []string
	PGNamedURLs          []PGURLConfig
	PGSSLMode            string
	PGSSLCert            string
	PGSSLKey             string
	PGSSLRootCert        string
	ContextPath          string
	PGMaxIdleConn        int
	PGMaxOpenConn        int
	PGConnTimeout        int
	PGCache              bool
	JWTKey               string
	JWTAlgo              string
	JWTWellKnownURL      string
	JWTJWKS              string
	JWTWhiteList         []string
	JSONAggType          string
	MigrationsPath       string
	QueriesPath          string
	AccessConf           AccessConf
	ExposeConf           ExposeConf
	CORSAllowOrigin      []string
	CORSAllowHeaders     []string
	CORSAllowMethods     []string
	CORSAllowCredentials bool
	Debug                bool
	Adapter              adapters.Adapter
	EnableDefaultJWT     bool
	SingleDB             bool
	HTTPSMode            bool
	HTTPSCert            string
	HTTPSKey             string
	Cache                cache.Config
	PluginPath           string
	PluginMiddlewareList []PluginMiddleware
	Logger               *slog.Logger
	// KetoReadURL is the Ory Keto Read API endpoint (default http://localhost:4466).
	KetoReadURL  string
	KetoWriteURL string
	KetoEnabled  bool
	// WorkspaceFiltersEnabled controls whether the workspace membership
	// resolver and IN-clause filters are active. Default false (Phase 1
	// only). When true, the four workspace tables are auto-scoped by
	// WorkspaceIDsKey.
	WorkspaceFiltersEnabled bool
	// WorkspaceCompatFilters are the active-workspace ("compat") entries.
	// Each listed table gets buildWorkspaceWhere semantics instead of the
	// plain user_id filter. Inert until the list is non-empty.
	WorkspaceCompatFilters []WorkspaceCompatConfig
	// WorkspaceActiveHeader is the request header carrying the single
	// active workspace id (default "X-Workspace-Id"), set by the BFF.
	WorkspaceActiveHeader string
}

const defaultCacheDir = "./"

var (
	// PrestConf config variable
	PrestConf      *Prest
	configFile     string
	defaultCfgFile = "./prest.toml"
)

// loadDotEnv populates os.Environ from a .env file in the current working
// directory. It is a no-op (and silent) when the file is absent, so
// production deployments that inject secrets via the orchestrator keep
// working unchanged. Variables already set in the environment take
// precedence — the .env file only fills in the gaps.
func loadDotEnv() {
	if err := godotenv.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Debug("could not load .env", "err", err)
		}
	}
}

// Load configuration
func Load() {
	loadDotEnv()
	viperCfg()
	PrestConf = &Prest{}
	Parse(PrestConf)
	if _, err := os.Stat(PrestConf.QueriesPath); os.IsNotExist(err) {
		if err = os.MkdirAll(PrestConf.QueriesPath, 0700); err != nil {
			slog.Error("Queries directory was not created", "path", PrestConf.QueriesPath, "err", err)
		}
	}

	// ignore cache if disabled
	if !PrestConf.Cache.Enabled {
		return
	}

	if _, err := os.Stat(PrestConf.Cache.StoragePath); os.IsNotExist(err) {
		if err = os.MkdirAll(PrestConf.Cache.StoragePath, 0700); err != nil {
			slog.Error("Cache directory was not created, falling back to default './'", "path", PrestConf.Cache.StoragePath, "err", err)
			PrestConf.Cache.StoragePath = defaultCacheDir
		}
	}

	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	if logLevel := os.Getenv("PREST_LOG_LEVEL"); logLevel != "" {
		var l slog.Level
		if err := l.UnmarshalText([]byte(logLevel)); err == nil {
			opts.Level = l
		}
	}
	PrestdHandler := slog.NewJSONHandler(os.Stdout, opts)
	PrestConf.Logger = slog.New(PrestdHandler)
	slog.SetDefault(PrestConf.Logger)
}

func viperCfg() {
	configFile = getPrestConfFile(os.Getenv("PREST_CONF"))

	dir, file := filepath.Split(configFile)
	file = strings.TrimSuffix(file, filepath.Ext(file))
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvPrefix("PREST")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(replacer)
	viper.AddConfigPath(dir)
	viper.SetConfigName(file)
	viper.SetConfigType("toml")

	viper.SetDefault("auth.enabled", false)
	viper.SetDefault("auth.username", "username")
	viper.SetDefault("auth.password", "password")
	viper.SetDefault("auth.schema", "public")
	viper.SetDefault("auth.table", "prest_users")
	viper.SetDefault("auth.encrypt", "MD5")
	viper.SetDefault("auth.type", "body")

	viper.SetDefault("http.host", "0.0.0.0")
	viper.SetDefault("http.port", 3000)
	viper.SetDefault("http.timeout", 60)

	viper.SetDefault("pg.host", "127.0.0.1")
	viper.SetDefault("pg.port", 5432)
	viper.SetDefault("pg.database", "prest")
	viper.SetDefault("pg.user", "postgres")
	viper.SetDefault("pg.pass", "postgres")
	viper.SetDefault("pg.maxidleconn", 0) // avoids db memory leak on req timeout
	viper.SetDefault("pg.maxopenconn", 10)
	viper.SetDefault("pg.conntimeout", 10)
	viper.SetDefault("pg.single", true)
	viper.SetDefault("pg.cache", true)
	// todo: replace this with prefer, will need to replace lib/pq
	// https://github.com/jackc/pgx/blob/47d631e34be7128997a0aa89b75885cc4ad4c82e/pgconn/config.go#L218
	viper.SetDefault("pg.ssl.mode", "disable")

	viper.SetDefault("jwt.default", true)
	viper.SetDefault("jwt.algo", "HS256")
	viper.SetDefault("jwt.wellknownurl", "")
	viper.SetDefault("jwt.jwks", "")
	viper.SetDefault("jwt.whitelist", []string{`^\/auth$`})

	viper.SetDefault("json.agg.type", "jsonb_agg")

	viper.SetDefault("cors.allowheaders", []string{"Content-Type"})
	viper.SetDefault("cors.allowmethods", []string{"GET", "HEAD", "POST", "PUT", "DELETE", "OPTIONS"})
	viper.SetDefault("cors.alloworigin", []string{"*"})
	viper.SetDefault("cors.allowcredentials", true)

	viper.SetDefault("https.mode", false)
	viper.SetDefault("https.cert", "/etc/certs/cert.crt")
	viper.SetDefault("https.key", "/etc/certs/cert.key")

	viper.SetDefault("cache.enabled", false)
	viper.SetDefault("cache.time", 10)
	viper.SetDefault("cache.storagepath", "./")
	viper.SetDefault("cache.sufixfile", ".cache.prestd.db")

	viper.SetDefault("version", 1)
	viper.SetDefault("debug", false)
	viper.SetDefault("context", "/")
	viper.SetDefault("pluginpath", "./lib")
	viper.SetDefault("pluginmiddlewarelist", []PluginMiddleware{})
	viper.SetDefault("keto.readurl", "http://localhost:4466")
	viper.SetDefault("keto.writeurl", "http://localhost:4467")
	viper.SetDefault("keto.enabled", false)
	viper.SetDefault("auth.workspace_filters_enabled", false)
	viper.SetDefault("expose.enabled", false)
	viper.SetDefault("expose.tables", true)
	viper.SetDefault("expose.schemas", true)
	viper.SetDefault("expose.databases", true)

	hDir, err := homedir.Dir()
	if err != nil {
		slog.Error("could not find homedir", "err", err)
		os.Exit(1)
	}
	viper.SetDefault("queries.location", filepath.Join(hDir, "queries"))
}

func getPrestConfFile(prestConf string) string {
	if prestConf != "" {
		return prestConf
	}
	return defaultCfgFile
}

// Parse pREST config
// todo: split config onto methods to simplify this
func Parse(cfg *Prest) {
	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			slog.Warn("file not found, falling back to default settings", "file", configFile)
			cfg.PGSSLMode = "disable"
		}
		slog.Warn("read env config error", "err", err)
	}

	parseAuthConfig(cfg)
	parseHTTPConfig(cfg)
	portFromEnv(cfg)
	parseDBConfig(cfg)

	cfg.JWTKey = viper.GetString("jwt.key")
	cfg.JWTAlgo = viper.GetString("jwt.algo")
	cfg.JWTWellKnownURL = viper.GetString("jwt.wellknownurl")
	cfg.JWTJWKS = viper.GetString("jwt.jwks")
	cfg.JWTWhiteList = viper.GetStringSlice("jwt.whitelist")
	fetchJWKS(cfg)

	cfg.JSONAggType = getJSONAgg()

	cfg.MigrationsPath = viper.GetString("migrations")

	cfg.AccessConf.Restrict = viper.GetBool("access.restrict")
	cfg.AccessConf.IgnoreTable = viper.GetStringSlice("access.ignore_table")
	cfg.QueriesPath = viper.GetString("queries.location")

	cfg.CORSAllowOrigin = viper.GetStringSlice("cors.alloworigin")
	cfg.CORSAllowHeaders = viper.GetStringSlice("cors.allowheaders")
	cfg.CORSAllowMethods = viper.GetStringSlice("cors.allowmethods")
	cfg.CORSAllowCredentials = viper.GetBool("cors.allowcredentials")

	cfg.Debug = viper.GetBool("debug")
	cfg.EnableDefaultJWT = viper.GetBool("jwt.default")
	cfg.ContextPath = viper.GetString("context")

	cfg.PluginPath = viper.GetString("pluginpath")

	loadCacheConfig(cfg)

	cfg.ExposeConf.Enabled = viper.GetBool("expose.enabled")
	cfg.ExposeConf.TableListing = viper.GetBool("expose.tables")
	cfg.ExposeConf.SchemaListing = viper.GetBool("expose.schemas")
	cfg.ExposeConf.DatabaseListing = viper.GetBool("expose.databases")

	// table access config
	var tablesconf []TablesConf
	err = viper.UnmarshalKey("access.tables", &tablesconf)
	if err != nil {
		slog.Error("could not unmarshal access tables", "err", err)
	}
	cfg.AccessConf.Tables = tablesconf

	var usersconf []UsersConf
	err = viper.UnmarshalKey("access.users", &usersconf)
	if err != nil {
		slog.Error("could not unmarshal access users", "err", err)
	}
	cfg.AccessConf.Users = usersconf

	// plugin middleware list config
	var pluginMiddlewareConfig []PluginMiddleware
	err = viper.UnmarshalKey("pluginmiddlewarelist", &pluginMiddlewareConfig)
	if err != nil {
		slog.Error("could not unmarshal access plugin middleware list", "err", err)
	}
	cfg.PluginMiddlewareList = pluginMiddlewareConfig
	cfg.KetoReadURL = viper.GetString("keto.readurl")
	cfg.KetoWriteURL = viper.GetString("keto.writeurl")
	cfg.KetoEnabled = viper.GetBool("keto.enabled")
}

// parseDatabaseURL tries to get from URL the DB configs
func parseDatabaseURL(cfg *Prest) {
	if cfg.PGURL == "" {
		slog.Debug("no db url found, skipping")
		return
	}
	// Parser PG URL, get database connection via string URL
	u, err := url.Parse(cfg.PGURL)
	if err != nil {
		slog.Error("cannot parse db url", "err", err)
		return
	}
	cfg.PGHost = u.Hostname()
	if u.Port() != "" {
		pgPort, err := strconv.Atoi(u.Port())
		if err != nil {
			slog.Error("cannot parse db url port, falling back to default values", "port", u.Port(), "err", err)
			return
		}
		cfg.PGPort = pgPort
	}
	cfg.PGUser = u.User.Username()
	pgPass, pgPassExist := u.User.Password()
	if pgPassExist {
		cfg.PGPass = pgPass
	}
	cfg.PGDatabase = strings.Replace(u.Path, "/", "", -1)
	if u.Query().Get("sslmode") != "" {
		cfg.PGSSLMode = u.Query().Get("sslmode")
	}
}

// ErrJWTDefaultEnabledNoKey is returned when the default JWT middleware is
// enabled but no verification material (HMAC key, JWKS or .well-known URL) was
// provided. This guards against accidentally serving requests with an empty
// HMAC key, which would let any client forge bearer tokens. See GHSA-fj7v-859r-2fm4.
var ErrJWTDefaultEnabledNoKey = errors.New(
	"jwt.default is enabled but no verification material was provided " +
		"(set jwt.key, jwt.jwks or jwt.wellknownurl, or disable jwt.default)")

// ErrAuthEnabledNoJWTKey is returned when basic auth is enabled but jwt.key
// is empty. AuthMiddleware uses the same []byte(JWTKey) to verify HS256
// tokens, so an empty key opens the same auth-bypass as the default JWT
// middleware. See GHSA-fj7v-859r-2fm4.
var ErrAuthEnabledNoJWTKey = errors.New(
	"auth.enabled is true but jwt.key is empty (required to verify HS256 tokens)")

// ValidateJWTConfig fails fast when either of the JWT-validating middlewares
// would be installed without any verification material:
//
//   - The default JWT middleware (jwt.default = true) requires jwt.key, a
//     JWKS, or a .well-known URL.
//   - AuthMiddleware (auth.enabled = true) verifies HS256 tokens with
//     jwt.key, so an empty key is unsafe.
//
// The default JWT path also bypasses when Debug is true, so we mirror that
// rule here to avoid blocking debug-mode startups.
//
// Call this from binary entrypoints before serving requests; tests that
// exercise Load() without setting JWT material rely on the middleware-level
// guards (middlewares.JwtMiddleware, middlewares.AuthMiddleware) to fail
// closed at request time.
func ValidateJWTConfig(cfg *Prest) error {
	if cfg.AuthEnabled && cfg.JWTKey == "" {
		return ErrAuthEnabledNoJWTKey
	}
	if !cfg.EnableDefaultJWT {
		return nil
	}
	if cfg.Debug {
		return nil
	}
	if cfg.JWTKey != "" || cfg.JWTJWKS != "" || cfg.JWTWellKnownURL != "" {
		return nil
	}
	return ErrJWTDefaultEnabledNoKey
}

// fetchJWKS tries to get the JWKS from the URL in the config
func fetchJWKS(cfg *Prest) {
	if cfg.JWTWellKnownURL == "" {
		slog.Debug("no JWT WellKnown url found, skipping")
		return
	}
	if cfg.JWTJWKS != "" {
		slog.Debug("JWKS already set, skipping")
		return
	}

	// Call provider to obtain .well-known config
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	r, err := client.Get(cfg.JWTWellKnownURL)
	if err != nil {
		slog.Error("Cannot get .well-known configuration", "url", cfg.JWTWellKnownURL, "err", err)
		return
	}
	defer r.Body.Close()

	var wellKnown map[string]interface{}
	err = json.NewDecoder(r.Body).Decode(&wellKnown)
	if err != nil {
		slog.Error("Failed to decode JSON", "err", err)
		return
	}

	//Retrieve the JWKS from the endpoint
	uri, ok := wellKnown["jwks_uri"].(string)
	if !ok {
		slog.Error("Unable to convert .WellKnown configuration of jwks_uri to a string")
		return
	}

	JWKSet, err := jwk.Fetch(context.Background(), uri)
	if err != nil {
		err := fmt.Errorf("failed to parse JWK: %s", err)
		log.Errorf("Failed to fetch JWK: %v\n", err)
		return
	}

	//Convert set to json string
	jwkSetJSON, err := json.Marshal(JWKSet)
	if err != nil {
		slog.Error("Failed to marshal JWKSet to JSON", "err", err)
		return
	}

	cfg.JWTJWKS = string(jwkSetJSON)
}

func portFromEnv(cfg *Prest) {
	if os.Getenv("PORT") == "" {
		slog.Debug("could not find PORT in env")
		return
	}
	// cloud factor support: https://help.heroku.com/PPBPA231/how-do-i-use-the-port-environment-variable-in-container-based-apps
	HTTPPort, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		slog.Debug("could not find PORT in env")
		return
	}
	cfg.HTTPPort = HTTPPort
}

// getJSONAgg identifies which json aggregation function will be used,
// support `jsonb` and `json`; `jsonb` is the default value
//
// https://www.postgresql.org/docs/9.5/functions-aggregate.html
func getJSONAgg() (config string) {
	config = viper.GetString("json.agg.type")
	if config == jsonAgg {
		return jsonAgg
	}
	if config != jsonAggDefault {
		slog.Warn("JSON Agg type can only be 'json_agg' or 'jsonb_agg', using the later as default")
	}
	return jsonAggDefault
}

func parseDBConfig(cfg *Prest) {
	cfg.PGURL = viper.GetString("pg.url")
	cfg.PGHost = viper.GetString("pg.host")
	cfg.PGPort = viper.GetInt("pg.port")
	cfg.PGUser = viper.GetString("pg.user")
	cfg.PGPass = viper.GetString("pg.pass")
	cfg.PGDatabase = viper.GetString("pg.database")
	cfg.PGSSLMode = viper.GetString("pg.ssl.mode")
	cfg.PGSSLKey = viper.GetString("pg.ssl.key")
	cfg.PGSSLCert = viper.GetString("pg.ssl.cert")
	cfg.PGSSLRootCert = viper.GetString("pg.ssl.rootcert")

	if os.Getenv("DATABASE_URL") != "" {
		// cloud factor support: https://devcenter.heroku.com/changelog-items/438
		cfg.PGURL = os.Getenv("DATABASE_URL")
	}
	parseDatabaseURL(cfg)

	cfg.PGMaxIdleConn = viper.GetInt("pg.maxidleconn")
	cfg.PGMaxOpenConn = viper.GetInt("pg.maxopenconn")
	cfg.PGConnTimeout = viper.GetInt("pg.conntimeout")
	cfg.PGCache = viper.GetBool("pg.cache")
	cfg.SingleDB = viper.GetBool("pg.single")

	// Parse pg.urls: try named format ([[pg.urls]] with name+url) first,
	// fall back to plain string array for backward compatibility.
	var namedURLs []PGURLConfig
	if err := viper.UnmarshalKey("pg.urls", &namedURLs); err == nil {
		for i := range namedURLs {
			if namedURLs[i].URL != "" && namedURLs[i].Name == "" {
				namedURLs[i].Name = DBNameFromURL(namedURLs[i].URL)
			}
			// Allow PREST_PG_URL_<NAME> to override the URL inline.
			// Use this for keeping credentials out of prest.toml: leave
			// the URL blank in the file and supply it via env / .env.
			if v := os.Getenv("PREST_PG_URL_" + pgURLEnvKey(namedURLs[i].Name)); v != "" {
				namedURLs[i].URL = v
				if namedURLs[i].Name == "" {
					namedURLs[i].Name = DBNameFromURL(v)
				}
			}
		}
	}
	if hasNamedURLs(namedURLs) {
		cfg.PGNamedURLs = namedURLs
	} else {
		// Legacy string array: also honour PREST_PG_URL_<N> for each entry.
		urls := viper.GetStringSlice("pg.urls")
		for i := range urls {
			if v := os.Getenv("PREST_PG_URL_" + strconv.Itoa(i)); v != "" {
				urls[i] = v
			}
		}
		cfg.PGURLs = urls
	}
}

// pgURLEnvKey normalises a pg.urls entry name into an env-var-friendly
// suffix: uppercase, with dashes and spaces replaced by underscores.
// An empty name yields an empty suffix, which produces "PREST_PG_URL_"
// — the caller can still set it but the lookup is order-dependent.
func pgURLEnvKey(name string) string {
	r := strings.NewReplacer("-", "_", " ", "_", ".", "_")
	return strings.ToUpper(r.Replace(name))
}

func hasNamedURLs(urls []PGURLConfig) bool {
	for _, u := range urls {
		if u.URL != "" {
			return true
		}
	}
	return false
}

// DBNameFromURL extracts the database name from a PostgreSQL connection URL.
func DBNameFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	name := strings.TrimPrefix(u.Path, "/")
	return name
}

func loadCacheConfig(cfg *Prest) {
	cfg.Cache.Enabled = viper.GetBool("cache.enabled")
	cfg.Cache.Time = viper.GetInt("cache.time")
	cfg.Cache.StoragePath = viper.GetString("cache.storagepath")
	cfg.Cache.SufixFile = viper.GetString("cache.sufixfile")

	// cache endpoints config
	var cacheendpoints = []cache.Endpoint{}
	err := viper.UnmarshalKey("cache.endpoints", &cacheendpoints)
	if err != nil {
		slog.Error("could not unmarshal cache endpoints", "err", err)
	}
	cfg.Cache.Endpoints = cacheendpoints
}

func parseAuthConfig(cfg *Prest) {
	cfg.AuthEnabled = viper.GetBool("auth.enabled")
	cfg.AuthSchema = viper.GetString("auth.schema")
	cfg.AuthTable = viper.GetString("auth.table")
	cfg.AuthUsername = viper.GetString("auth.username")
	cfg.AuthPassword = viper.GetString("auth.password")
	cfg.AuthEncrypt = viper.GetString("auth.encrypt")
	cfg.AuthMetadata = viper.GetStringSlice("auth.metadata")
	cfg.AuthType = viper.GetString("auth.type")
	cfg.UserIDHeader = viper.GetString("auth.user_id_header")

	var userFilters []UserFilterConfig
	if err := viper.UnmarshalKey("auth.user_id_filters", &userFilters); err == nil {
		cfg.UserIDFilters = userFilters
	}

	var workspaceFilters []WorkspaceFilterConfig
	if err := viper.UnmarshalKey("auth.workspace_id_filters", &workspaceFilters); err == nil {
		cfg.WorkspaceIDFilters = workspaceFilters
	}
	cfg.WorkspaceFiltersEnabled = viper.GetBool("auth.workspace_filters_enabled")

	var compatFilters []WorkspaceCompatConfig
	if err := viper.UnmarshalKey("auth.workspace_compat_filters", &compatFilters); err == nil {
		cfg.WorkspaceCompatFilters = compatFilters
	}
	cfg.WorkspaceActiveHeader = viper.GetString("auth.workspace_active_header")

	if err := ValidateWorkspaceCompat(cfg); err != nil {
		slog.Error("invalid auth config: workspace_compat overlap", "err", err)
		os.Exit(1)
	}
}

// ValidateWorkspaceCompat rejects a table listed in both user_id_filters
// and workspace_compat_filters — each table must receive exactly one
// filter (compat takes runtime precedence, but the overlap is always a
// config mistake). Returns nil when the two sets are disjoint.
func ValidateWorkspaceCompat(cfg *Prest) error {
	seen := make(map[[3]string]bool, len(cfg.UserIDFilters))
	for _, f := range cfg.UserIDFilters {
		seen[[3]string{f.Database, f.Schema, f.Table}] = true
	}
	for _, f := range cfg.WorkspaceCompatFilters {
		key := [3]string{f.Database, f.Schema, f.Table}
		if seen[key] {
			return fmt.Errorf("table %s/%s/%s appears in both user_id_filters and workspace_compat_filters — list it in exactly one", f.Database, f.Schema, f.Table)
		}
	}
	return nil
}

func parseHTTPConfig(cfg *Prest) {
	cfg.HTTPHost = viper.GetString("http.host")
	cfg.HTTPPort = viper.GetInt("http.port")
	cfg.HTTPTimeout = viper.GetInt("http.timeout")

	cfg.HTTPSMode = viper.GetBool("https.mode")
	cfg.HTTPSCert = viper.GetString("https.cert")
	cfg.HTTPSKey = viper.GetString("https.key")
}
