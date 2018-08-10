package lb

import "net/http"

//This struct is used when a http request is made.
//This custom implementation intents to postpone the response writing operation by storing the contents in variables and allowing the manual actual writing at any given time.
type CustomResponseWriter struct {
	http.ResponseWriter
	StatusCode int
	Body       []byte
}

//Instead of writing the data to the response body, this custom method stores it in a variable
func (crw *CustomResponseWriter) Write(data []byte) (int, error) {
	crw.Body = data
	return len(data), nil
}

//Instead of writing the code to the response header, this custom method stores it in a variable
func (crw *CustomResponseWriter) WriteHeader(code int) {
	crw.StatusCode = code
}

func (crw *CustomResponseWriter) WriteResponse() {
	crw.ResponseWriter.WriteHeader(crw.StatusCode)
	crw.ResponseWriter.Write(crw.Body)
}

func NewCustomResponseWriter(w http.ResponseWriter) *CustomResponseWriter {
	return &CustomResponseWriter{w, http.StatusOK, nil}
}
