// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rendering

import (
	"testing"
	texttemplate "text/template"

	sprig "github.com/go-task/slim-sprig/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/notification/internal/templatecache"
)

func TestRendering(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Rendering Suite")
}

func mustUnmarshal(raw string) map[string]interface{} {
	v, err := UnmarshalProperties([]byte(raw))
	if err != nil {
		panic(err)
	}
	return v
}

var _ = Describe("Template Rendering", func() {

	Describe("Custom template functions", func() {
		It("should render a template with a custom greeter function", func() {
			funcs := sprig.FuncMap()
			funcs["greeter"] = func(name string) string { return "Hello " + name }

			tpl := `This is a test template with a custom function {{ greeter .Name }}.`
			template, err := texttemplate.New("test").Funcs(funcs).Parse(tpl)
			Expect(err).NotTo(HaveOccurred())

			message, err := RenderMessage(template, mustUnmarshal(`{"Name":"John"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(message).To(Equal("This is a test template with a custom function Hello John."))
		})
	})

	Describe("icsTime", func() {
		var icsTimeFn func(string) (string, error)

		BeforeEach(func() {
			funcs := getCustomTemplateFunctions()
			icsTimeFn = funcs["icsTime"].(func(string) (string, error))
		})

		It("should convert a valid RFC3339 timestamp", func() {
			result, err := icsTimeFn("2026-04-25T09:00:00Z")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("20260425T090000Z"))
		})

		It("should convert a timestamp with timezone offset to UTC", func() {
			result, err := icsTimeFn("2026-04-25T11:00:00+02:00")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("20260425T090000Z"))
		})

		It("should return an error for invalid input", func() {
			_, err := icsTimeFn("not-a-time")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("icsTime: invalid RFC3339 time"))
		})

		It("should work within a Go template", func() {
			funcs := sprig.FuncMap()
			for k, v := range getCustomTemplateFunctions() {
				funcs[k] = v
			}

			tpl := `DTSTART:{{ icsTime .start }}`
			template, err := texttemplate.New("test").Funcs(funcs).Parse(tpl)
			Expect(err).NotTo(HaveOccurred())

			message, err := RenderMessage(template, mustUnmarshal(`{"start":"2026-04-25T09:00:00Z"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(message).To(Equal("DTSTART:20260425T090000Z"))
		})
	})

	Describe("RenderAttachments", func() {
		It("should return nil for empty attachments", func() {
			result, err := RenderAttachments(nil, map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should render a single attachment with icsTime", func() {
			funcs := sprig.FuncMap()
			for k, v := range getCustomTemplateFunctions() {
				funcs[k] = v
			}

			contentTpl, err := texttemplate.New("att").Funcs(funcs).Parse("BEGIN:VCALENDAR\nDTSTART:{{ icsTime .start }}\nEND:VCALENDAR")
			Expect(err).NotTo(HaveOccurred())

			attachments := []templatecache.ParsedAttachment{
				{
					Filename:        "invite.ics",
					ContentType:     "text/calendar",
					ContentTemplate: contentTpl,
				},
			}

			result, err := RenderAttachments(attachments, mustUnmarshal(`{"start":"2026-04-25T09:00:00Z"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0].Filename).To(Equal("invite.ics"))
			Expect(result[0].ContentType).To(Equal("text/calendar"))
			Expect(string(result[0].Content)).To(Equal("BEGIN:VCALENDAR\nDTSTART:20260425T090000Z\nEND:VCALENDAR"))
		})

		It("should return an error when template execution fails", func() {
			badTpl, _ := texttemplate.New("att-bad").Parse("{{ call .notAFunc }}")
			attachments := []templatecache.ParsedAttachment{
				{
					Filename:        "bad.ics",
					ContentType:     "text/calendar",
					ContentTemplate: badTpl,
				},
			}

			_, err := RenderAttachments(attachments, mustUnmarshal(`{"notAFunc":"string"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to render attachment"))
		})
	})
})
