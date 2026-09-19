package router

import (
	"github.com/gin-gonic/gin"
	"pki-certificate-rollover-impact/backend/internal/constants"
	"pki-certificate-rollover-impact/backend/internal/handler"
	"pki-certificate-rollover-impact/backend/internal/middleware"
)

func RegisterImpactSignoffRoutes(api *gin.RouterGroup, h *handler.ImpactSignoffHandler) {
	group := api.Group("/rollover-scenarios/:id/signoffs")
	group.GET("", middleware.RequirePermission(constants.PermissionRead), h.List)
	group.POST("", middleware.RequireAnyPermission(constants.PermissionScenarioVerify, constants.PermissionScenarioWrite), h.Register)
}
