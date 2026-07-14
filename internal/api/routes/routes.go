package routes

import (
	mw "clinic-api/internal/api/middleware"
	syncpkg "clinic-api/internal/sync"
)

var scope = mw.RequireScope
var scopeAny = mw.RequireAnyScope
var scopeAll = mw.RequireAllScopes
var scopeOrSelf = mw.RequireScopeOrSelf
var selfUser = mw.SelfUserID
var selfEmployee = mw.SelfEmployeeID
var cache = mw.CacheMiddleware
var cacheF = mw.CacheMiddlewareForce
var criticalSync = syncpkg.RequireCriticalSync
