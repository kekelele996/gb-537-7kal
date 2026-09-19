package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"pki-certificate-rollover-impact/backend/internal/dto"
	"pki-certificate-rollover-impact/backend/internal/service"
	"pki-certificate-rollover-impact/backend/internal/util"
)

type RolloverSignoffHandler struct {
	service *service.RolloverScenarioService
}

func NewRolloverSignoffHandler(value *service.RolloverScenarioService) *RolloverSignoffHandler {
	return &RolloverSignoffHandler{service: value}
}
func (h *RolloverSignoffHandler) ReviewGate(c *gin.Context) {
	id, err := util.ParseUintParam(c, "id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	result, serviceErr := h.service.ReviewGate(c.Request.Context(), id)
	respond(c, http.StatusOK, result, serviceErr)
}
func (h *RolloverSignoffHandler) Register(c *gin.Context) {
	id, err := util.ParseUintParam(c, "id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	serviceID, err := util.ParseUintParam(c, "service_id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	var request dto.UpsertRolloverSignoffRequest
	if !bindJSON(c, &request) {
		return
	}
	result, serviceErr := h.service.RegisterSignoff(c.Request.Context(), id, serviceID, request, mustActor(c), util.RequestID(c))
	respond(c, http.StatusOK, result, serviceErr)
}
