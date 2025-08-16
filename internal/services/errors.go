package services

type ApiError struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

func (a *ApiError) Error() string {
	return a.Message
}
