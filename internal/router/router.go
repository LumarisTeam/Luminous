package router

import (
	"fmt"
	"net/http"
	"strings"

	"luminous/internal/handler"
	"luminous/internal/mcpserver"
	"luminous/internal/middleware"
	"luminous/internal/response"

	"github.com/gin-gonic/gin"
)

func SetupRouter(
	schoolHandler *handler.SchoolHandler,
	adminHandler *handler.AdminHandler,
	appHandler *handler.AppHandler,
	adminToken string,
	corsOrigin string,
	rateLimitRate, rateLimitBurst int,
	trustedProxies string,
	mcpHandler http.Handler,
	mcpToken string,
) (*gin.Engine, error) {
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.CORSMiddleware(corsOrigin))
	r.Use(middleware.RateLimitMiddleware(rateLimitRate, rateLimitBurst))

	if trustedProxies != "" {
		proxies := strings.Split(trustedProxies, ",")
		for i, p := range proxies {
			proxies[i] = strings.TrimSpace(p)
		}
		if err := r.SetTrustedProxies(proxies); err != nil {
			return nil, fmt.Errorf("set trusted proxies: %w", err)
		}
	} else {
		if err := r.SetTrustedProxies(nil); err != nil {
			return nil, fmt.Errorf("set trusted proxies: %w", err)
		}
	}

	r.NoRoute(func(c *gin.Context) {
		response.Error(c, http.StatusNotFound, "route not found")
	})

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1")
	{
		v1.GET("/schools", schoolHandler.ListSchools)
		v1.GET("/schools/:code", schoolHandler.GetSchool)
		v1.GET("/app", appHandler.GetTagModel)
		v1.GET("/App", appHandler.GetTagModel)
	}

	admin := r.Group("/api/v1/admin")
	admin.Use(middleware.BodyLimitMiddleware(1 << 20)) // 1 MB
	admin.Use(middleware.AuthMiddleware(adminToken))
	{
		admin.GET("/schools", adminHandler.AdminListSchools)
		admin.POST("/schools", adminHandler.CreateSchool)
		admin.PUT("/schools/:code", adminHandler.UpdateSchool)
		admin.DELETE("/schools/:code", adminHandler.DeleteSchool)
	}

	if mcpHandler != nil {
		mcp := r.Group("/mcp")
		if mcpToken != "" {
			mcp.Use(mcpserver.TokenMiddleware(mcpToken))
		}
		mcp.Any("", gin.WrapH(mcpHandler))
		mcp.Any("/", gin.WrapH(mcpHandler))
	}

	return r, nil
}
