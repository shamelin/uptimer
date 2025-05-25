package hooks

import (
	log "github.com/sirupsen/logrus"
	"time"
	"uptimer/internal/seeking/dto"
)

var logger = log.WithFields(log.Fields{
	"package": "hooks",
})

type SeekResult struct {
	Host         dto.Host
	Online       bool
	StatusCode   int
	ResponseTime time.Duration
}

type HookHandler interface {
	Handle(seekResult SeekResult) error
}
