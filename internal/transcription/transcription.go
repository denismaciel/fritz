package transcription

import "context"

type AudioInput struct {
	Bytes    []byte
	MIMEType string
	FileName string
}

type Service interface {
	Transcribe(context.Context, AudioInput) (string, error)
}
