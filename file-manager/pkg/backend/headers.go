package backend

const (
	// XFileContentType is the header for the content type of the file
	XFileContentType = "X-File-Content-Type"
	// XFileChecksum is the header for the checksum of the file
	XFileChecksum = "X-File-Checksum"
	// XFileDetectedContentType is the header for the detected content type
	XFileDetectedContentType = "X-File-Detected-Content-Type"
	// XFileContentTypeSource indicates the source of content type detection
	XFileContentTypeSource = "X-File-Content-Type-Source"
	DefaultContentType     = "application/octet-stream" // Default content type if not specified or detected
)
