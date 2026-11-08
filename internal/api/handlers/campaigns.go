package handlers

import (
	"net/http"
	"strings"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	var input dto.CampaignInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	campaign, err := h.factory.Services.Campaigns.Create(r.Context(), input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusCreated, campaign, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) ListCampaigns(w http.ResponseWriter, r *http.Request) {
	var filters dto.CampaignFilters
	if value := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status"))); value != "" {
		filters.Status = &value
	}
	if value := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("participation"))); value != "" {
		filters.Participation = &value
	}
	result, err := h.factory.Services.Campaigns.List(r.Context(), filters, h.getPaginationParams(r))
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, result, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) GetCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.errorResponse(w, r, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid campaign id"})
		return
	}
	campaign, err := h.factory.Services.Campaigns.Get(r.Context(), id)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, campaign, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) CampaignContributions(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.errorResponse(w, r, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid campaign id"})
		return
	}
	result, err := h.factory.Services.Campaigns.Contributions(r.Context(), id, h.getPaginationParams(r))
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, result, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) UpdateCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.errorResponse(w, r, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid campaign id"})
		return
	}
	var input dto.CampaignInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	campaign, err := h.factory.Services.Campaigns.Update(r.Context(), id, input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, campaign, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) ChangeCampaignStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.errorResponse(w, r, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid campaign id"})
		return
	}
	var input dto.CampaignStatusInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	campaign, err := h.factory.Services.Campaigns.ChangeStatus(r.Context(), id, input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, campaign, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) ResolveCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.errorResponse(w, r, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid campaign id"})
		return
	}
	var input dto.CampaignResolutionInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	campaign, err := h.factory.Services.Campaigns.Resolve(r.Context(), id, input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, campaign, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) SetCampaignRefundStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.errorResponse(w, r, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid campaign id"})
		return
	}
	var input dto.CampaignRefundStatusInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	campaign, err := h.factory.Services.Campaigns.SetRefundStatus(r.Context(), id, input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, campaign, nil); err != nil {
		h.logError(r, err)
	}
}
