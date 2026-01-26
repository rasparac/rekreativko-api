package api

import (
	"encoding/json"
	"net/http"
)

// DecodeJSONBody is used as generic decoding method to get request body in JSON format back as an struct
func DecodeJSONBody(r *http.Request, body any) error {
	return json.NewDecoder(r.Body).Decode(body)
}
