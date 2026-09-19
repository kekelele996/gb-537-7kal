package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"pki-certificate-rollover-impact/backend/internal/constants"
	"pki-certificate-rollover-impact/backend/internal/dto"
	"pki-certificate-rollover-impact/backend/internal/service"
	"pki-certificate-rollover-impact/backend/internal/util"
)

type ImpactSignoffHandler struct {
	service *service.ImpactSignoffService
}

func NewImpactSignoffHandler(value *service.ImpactSignoffService) *ImpactSignoffHandler {
	return &ImpactSignoffHandler{service: value}
}
func (h *ImpactSignoffHandler) List(c *gin.Context) {
	id, err := util.ParseUintParam(c, "id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	result, serviceErr := h.service.GateStatus(c.Request.Context(), id)
	respond(c, http.StatusOK, result, serviceErr)
}
func (h *ImpactSignoffHandler) Register(c *gin.Context) {
	id, err := util.ParseUintParam(c, "id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	actor := mustActor(c)
	if !constants.HasPermission(constants.Role(actor.Role), constants.PermissionScenarioVerify) {
		if creator, loadErr := h.service.CreatedBy(c.Request.Context(), id); loadErr == nil && creator == actor.UserID {
			util.Fail(c, util.NewError(http.StatusConflict, util.CodeReviewerConflict, "scenario creator cannot sign off impact dispositions"))
			return
		}
		util.Fail(c, util.NewError(http.StatusForbidden, util.CodeForbidden, "role cannot register impact sign-offs"))
		return
	}
	var request dto.CreateImpactSignoffRequest
	if !bindJSON(c, &request) {
		return
	}
	result, serviceErr := h.service.Register(c.Request.Context(), id, request, actor, util.RequestID(c))
	respond(c, http.StatusCreated, result, serviceErr)
}
