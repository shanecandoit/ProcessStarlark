package workspace

import "errors"

var (
	ErrDraftNotFound         = errors.New("draft not found")
	ErrDraftExists           = errors.New("draft id already exists")
	ErrDraftLimit            = errors.New("this local service supports at most 16 drafts")
	ErrDraftInput            = errors.New("invalid draft operation")
	ErrSourceVersionNotFound = errors.New("source version not found")
)
