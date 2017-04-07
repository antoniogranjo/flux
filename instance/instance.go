package instance

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"

	"github.com/weaveworks/flux"
	"github.com/weaveworks/flux/git"
	"github.com/weaveworks/flux/history"
	fluxmetrics "github.com/weaveworks/flux/metrics"
	"github.com/weaveworks/flux/platform"
	"github.com/weaveworks/flux/registry"
)

type Instancer interface {
	Get(inst flux.InstanceID) (*Instance, error)
}

type Instance struct {
	Platform platform.Platform
	Config   Configurer
	Repo     git.Repo

	log.Logger
	history.EventReader
	history.EventWriter
}

func New(
	platform platform.Platform,
	config Configurer,
	logger log.Logger,
	events history.EventReader,
	eventlog history.EventWriter,
) *Instance {
	return &Instance{
		Platform:    platform,
		Config:      config,
		Logger:      logger,
		EventReader: events,
		EventWriter: eventlog,
	}
}
