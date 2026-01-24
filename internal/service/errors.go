package service

import "errors"

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSpaceNotFound   = errors.New("space not found")
	ErrFileNotFound    = errors.New("file not found")
	ErrFileNotUploaded = errors.New("file not uploaded")
	ErrMessageNotFound = errors.New("message not found")
)
