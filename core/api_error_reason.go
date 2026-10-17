package core

import (
	"encoding/json"
	"net/http"
)

// reason is the API's explanation of an error response, which APIError.Error prints after
// the status code: the body's detail, else its title, else its message (the gateway's 401),
// else the status text.
func reason(statusCode int, body string) string {
	var fields struct{ Detail, Title, Message string }
	// A body that is not a JSON object leaves every field empty.
	_ = json.Unmarshal([]byte(body), &fields)
	for _, text := range []string{fields.Detail, fields.Title, fields.Message} {
		if text != "" {
			return text
		}
	}
	return http.StatusText(statusCode)
}
