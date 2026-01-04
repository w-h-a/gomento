package v1space

import (
	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type ListSpacesOutput struct {
	Items []v1.Space `json:"items"`
}
