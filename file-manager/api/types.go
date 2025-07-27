package api

type FileUploadResponse struct {
	MD5Hash     string
	FileId      string
	ContentType string
}

type FileDownloadResponse struct {
	MD5Hash     string
	ContentType string
	Content     []byte
}
