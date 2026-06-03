package routes

import (
	"net/http"
	"time"

	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/httpx"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

// ~10 login attempts/min per IP; 429 on exceed.
func loginRateLimiter() echo.MiddlewareFunc {
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
			Rate:      rate.Limit(10.0 / 60.0),
			Burst:     10,
			ExpiresIn: 3 * time.Minute,
		}),
		DenyHandler: func(c echo.Context, _ string, _ error) error {
			return c.JSON(http.StatusTooManyRequests, httpx.Response{Error: "Too many login attempts, please try again later"})
		},
	})
}

func SetupAuthRoutes(public *echo.Group, protected *echo.Group) {
	public.POST("/auth/login", handlers.Login, loginRateLimiter())
	public.POST("/auth/logout", handlers.Logout)
	protected.GET("/auth/verify", handlers.Verify)
	protected.GET("/auth/me", handlers.Me)
}
