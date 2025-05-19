package problems

import (
	"fmt"
	"net/http"
)

var _ error = &problem{}
var _ Problem = &problem{}

type Problem interface {
	error
	Code() int
}

// see https://www.rfc-editor.org/rfc/rfc9457.html#name-members-of-a-problem-detail
type problem struct {
	Type     string  `json:"type"`
	Status   int     `json:"status"`
	Title    string  `json:"title"`
	Detail   string  `json:"detail"`
	Instance string  `json:"instance"`
	Fields   []Field `json:"fields,omitempty"`
}

type Field struct {
	Field  string `json:"field"`
	Detail string `json:"detail"`
}

func (p *problem) Error() string {
	return fmt.Sprintf("%s: %s", p.Title, p.Detail)
}

func (p *problem) Code() int {
	return p.Status
}

type ProblemBuilder interface {
	Build() Problem
	Type(string) ProblemBuilder
	Status(int) ProblemBuilder
	Title(string) ProblemBuilder
	Detail(string) ProblemBuilder
	Instance(string) ProblemBuilder
	Fields(...Field) ProblemBuilder
}

type problemBuilder struct {
	problem *problem
}

func Builder() ProblemBuilder {
	return &problemBuilder{
		problem: new(problem),
	}
}

func (pb *problemBuilder) Build() Problem {
	return pb.problem
}

func (pb *problemBuilder) Type(t string) ProblemBuilder {
	pb.problem.Type = t
	return pb
}

func (pb *problemBuilder) Status(s int) ProblemBuilder {
	pb.problem.Status = s
	return pb
}

func (pb *problemBuilder) Title(t string) ProblemBuilder {
	pb.problem.Title = t
	return pb
}

func (pb *problemBuilder) Detail(d string) ProblemBuilder {

	pb.problem.Detail = d
	return pb
}

func (pb *problemBuilder) Instance(i string) ProblemBuilder {
	pb.problem.Instance = i
	return pb
}

func (pb *problemBuilder) Fields(fields ...Field) ProblemBuilder {
	pb.problem.Fields = append(pb.problem.Fields, fields...)
	return pb
}

func NewProblemOfError(err error) Problem {
	return &problem{
		Type:   "InternalError",
		Status: http.StatusInternalServerError,
		Title:  "Internal Error",
		Detail: err.Error(),
	}
}
