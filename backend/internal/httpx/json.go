package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
)

const MaxRequestBytes = 1 << 20

const unknownFieldPrefix = "json: unknown field "

type ErrorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"requestId,omitempty"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(payload)
}

func Error(w http.ResponseWriter, status int, code, message string) {
	ErrorWithDetails(w, status, code, message, nil)
}

func ErrorWithDetails(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	WriteJSON(w, status, ErrorResponse{Error: ErrorBody{
		Code:      code,
		Message:   message,
		Details:   details,
		RequestID: w.Header().Get(RequestIDHeader),
	}})
}

func DecodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		status := http.StatusBadRequest
		code := CodeBadRequest
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			status = http.StatusRequestEntityTooLarge
			code = CodePayloadTooLarge
		}
		Error(w, status, code, decodeErrorMessage(err))
		return false
	}
	return true
}

func decodeErrorMessage(err error) string {
	var syntaxError *json.SyntaxError
	var typeError *json.UnmarshalTypeError
	var maxBytesError *http.MaxBytesError

	switch {
	case errors.As(err, &syntaxError):
		return fmt.Sprintf("request body contains malformed JSON at position %d", syntaxError.Offset)
	case errors.As(err, &typeError):
		if typeError.Field == "" {
			return "request body must be a JSON object"
		}
		if expected := exposableTypeName(typeError.Type); expected != "" {
			return fmt.Sprintf("field %q must be of type %s", typeError.Field, expected)
		}
		return fmt.Sprintf("field %q has an invalid value", typeError.Field)
	case errors.As(err, &maxBytesError):
		return fmt.Sprintf("request body must not exceed %d bytes", maxBytesError.Limit)
	case errors.Is(err, io.EOF):
		return "request body is empty"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "request body contains malformed JSON"
	case strings.HasPrefix(err.Error(), unknownFieldPrefix):
		return "request body contains unknown field " + strings.TrimPrefix(err.Error(), unknownFieldPrefix)
	default:
		return "request body is invalid"
	}
}

func exposableTypeName(target reflect.Type) string {
	if target == nil {
		return ""
	}
	name := target.String()
	if strings.Contains(name, ".") {
		return ""
	}
	return name
}
