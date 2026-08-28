package server

import "github.com/gin-gonic/gin"

// registerSystemRoutes keeps process probes separate from versioned product
// APIs so their availability contract cannot accidentally inherit auth or
// optional dependency middleware.
func (s *Server) registerSystemRoutes(router *gin.Engine) {
	router.GET("/healthz", gin.WrapF(s.health))
	router.GET("/readyz", gin.WrapF(s.ready))
	router.GET("/health/dependencies", gin.WrapF(s.dependencyHealth))
	router.GET("/metrics", gin.WrapF(s.metrics))
}

func (s *Server) registerPublicAPIRoutes(router *gin.Engine) {
	router.POST("/api/v1/auth/register", gin.WrapF(s.register))
	router.POST("/api/v1/auth/login", gin.WrapF(s.login))
	router.POST("/api/v1/auth/token", gin.WrapF(s.issueToken))
	router.POST("/api/v1/ai-events/:eventID/claims", s.preAuthClaimIPLimit(), s.authenticate(), s.requireActiveTenant(), gin.WrapF(s.claimAIEvent))
}
