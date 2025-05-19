package problems

import (
	"fmt"
	"net/http"

	"github.com/pkg/errors"
)

func NotFound(ref ...string) Problem {
	detail := "No resource found"
	if len(ref) > 0 {
		detail = fmt.Sprintf("Resource %s not found", ref[0])
	}

	return Builder().
		Status(http.StatusNotFound).
		Type("NotFound").
		Title("Not found").
		Detail(detail).
		Build()
}

func MethodNotAllowed(method string) Problem {
	return Builder().
		Status(http.StatusMethodNotAllowed).
		Type("MethodNotAllowed").
		Title("Method Not Allowed").
		Detail(fmt.Sprintf("Method %s not allowed", method)).
		Build()
}

func IsNotFound(err error) bool {
	var p Problem
	if errors.As(err, &p) {
		return p.Code() == http.StatusNotFound
	}
	return false
}

func ValidationError(field, detail string) Problem {
	return Builder().
		Status(http.StatusBadRequest).
		Type("ValidationError").
		Title("Invalid Request").
		Detail(fmt.Sprintf("%s: %s", field, detail)).
		Build()
}

func IsValidationError(err error) bool {
	var p *problem
	if errors.As(err, &p) {
		return p.Type == "ValidationError"
	}
	return false
}

func ValidationErrors(fieldsMap map[string]string, detail ...string) Problem {
	fields := make([]Field, 0, len(fieldsMap))
	for field, detail := range fieldsMap {
		fields = append(fields, Field{
			Field:  field,
			Detail: detail,
		})
	}
	d := "One or more fields failed validation"
	if len(detail) > 0 {
		d = detail[0]
	}

	return Builder().
		Status(http.StatusBadRequest).
		Type("ValidationError").
		Title("Invalid Request").
		Detail(d).
		Fields(fields...).
		Build()
}

func BadRequest(detail string) Problem {
	return Builder().
		Status(http.StatusBadRequest).
		Type("BadRequest").
		Title("Bad Request").
		Detail(detail).
		Build()
}

func InternalServerError(title, detail string) Problem {
	return Builder().
		Status(http.StatusInternalServerError).
		Type("InternalServerError").
		Title(title).
		Detail(detail).
		Build()
}

func Conflict(detail string) Problem {
	return Builder().
		Status(http.StatusConflict).
		Type("Conflict").
		Title("Conflict").
		Detail(detail).
		Build()
}

func Forbidden(title, detail string) Problem {
	return Builder().
		Status(http.StatusForbidden).
		Type("Forbidden").
		Title(title).
		Detail(detail).
		Build()
}

func Unauthorized(title, detail string) Problem {
	return Builder().
		Status(http.StatusUnauthorized).
		Type("Unauthorized").
		Title(title).
		Detail(detail).
		Build()
}
