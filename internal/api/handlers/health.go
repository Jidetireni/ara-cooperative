package handlers

import "net/http"

func (h *Handlers) HealthCheckHandler(w http.ResponseWriter, r *http.Request) {

	resp := envelope{
		"status": "available",
		"system_info": map[string]string{
			"environment": h.config.Server.Env,
			"version":     "1.0.0",
		},
	}

	if err := h.writeJSON(w, http.StatusOK, resp, nil); err != nil {
		h.errorResponse(w, r, err)
		return
	}

}
