package hooks

import (
	log "github.com/sirupsen/logrus"
	"time"
	"uptimer/internal/seeking/dto"
)

var logger = log.WithFields(log.Fields{
	"package": "hooks",
})

// SeekResult represents the result of a seek operation.
type SeekResult struct {
	Host         dto.Host
	Online       bool
	StatusCode   int
	ResponseTime time.Duration
}

// HookHandler is an interface that defines a method to handle seek results.
type HookHandler interface {
	Handle(seekResult SeekResult) error
}
