package constants

type HeaderName string

var (
	HeaderNameChecksum            HeaderName = "X-File-Checksum"
	HeaderNameOriginalContentType HeaderName = "X-File-Content-Type"
)

func (h HeaderName) String() string {
	return string(h)
}
