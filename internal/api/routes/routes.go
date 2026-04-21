package routes

import mw "clinic-api/internal/api/middleware"

var scope = mw.RequireScope
var cache = mw.CacheMiddleware
