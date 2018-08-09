package lb

import "net/http"

type CustomResponseWriter struct {
	http.ResponseWriter
	StatusCode int
	Body       []byte
}

func (crw *CustomResponseWriter) Write(data []byte) (int, error) {
	crw.Body = data
	return len(data), nil
}

func (crw *CustomResponseWriter) WriteHeader(code int) {
	crw.StatusCode = code
}

func NewCustomResponseWriter(w http.ResponseWriter) *CustomResponseWriter {
	return &CustomResponseWriter{w, http.StatusOK, nil}
}

