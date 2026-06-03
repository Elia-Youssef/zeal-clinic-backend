package routes

import mw "clinic-api/internal/api/middleware"

var scope = mw.RequireScope
var scopeAny = mw.RequireAnyScope
var scopeAll = mw.RequireAllScopes
var scopeOrSelf = mw.RequireScopeOrSelf
var selfUser = mw.SelfUserID
var selfEmployee = mw.SelfEmployeeID
var cache = mw.CacheMiddleware
var cacheF = mw.CacheMiddlewareForce
