package router

import (
	"github.com/gin-gonic/gin"
	"pki-certificate-rollover-impact/backend/internal/constants"
	"pki-certificate-rollover-impact/backend/internal/handler"
	"pki-certificate-rollover-impact/backend/internal/middleware"
)

func RegisterRolloverSignoffRoutes(api *gin.RouterGroup, h *handler.RolloverSignoffHandler) {
	group := api.Group("/rollover-scenarios")
	group.GET("/:id/signoffs", middleware.RequirePermission(constants.PermissionRead), h.ReviewGate)
	group.PUT("/:id/signoffs/:service_id", middleware.RequirePermission(constants.PermissionScenarioVerify), h.Register)
}
