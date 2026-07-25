package context

type Key int

const (
	_ Key = iota
	DBNameKey
	HTTPTimeoutKey
	UserInfoKey
	UserIDKey
	// TenantIDKey holds the single tenant ID for a request, exposed
	// to SQL templates via the `tenantId` var. NOTE: its only setter
	// (the TenantAuthzGate middleware, which read `?tenantId=`)
	// has been removed — authentication and tenant authorization now
	// live in Ory Oathkeeper. This key is therefore vestigial (always
	// empty) and kept only because controllers/sql.go still reads it; it
	// is harmlessly empty unless repopulated from the X-Tenant-Id
	// header in a future change. Active-tenant scoping now flows via
	// TenantIDActiveKey.
	TenantIDKey
	// TenantIDActiveKey holds the single active tenant id for the
	// request, sourced from the X-Tenant-Id header. Oathkeeper sets
	// this header authoritatively; empty =
	// personal mode. Used exclusively by the "compat" filter mode
	// (buildTenantWhere semantics) on tenant-capable content
	// tables — distinct from TenantIDsKey (union membership) and
	// TenantIDKey (vestigial single-value template var).
	TenantIDActiveKey
)
