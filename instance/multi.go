package instance

import (
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"

	"github.com/weaveworks/flux"
	"github.com/weaveworks/flux/git"
	"github.com/weaveworks/flux/history"
	"github.com/weaveworks/flux/platform"
	"github.com/weaveworks/flux/registry"
)

type MultitenantInstancer struct {
	DB                  DB
	Connecter           platform.Connecter
	Logger              log.Logger
	History             history.DB
	MemcacheClient      registry.MemcacheClient
	RegistryCacheExpiry time.Duration
}

func (m *MultitenantInstancer) Get(instanceID flux.InstanceID) (*Instance, error) {
	c, err := m.DB.GetConfig(instanceID)
	if err != nil {
		return nil, errors.Wrap(err, "getting instance config from DB")
	}

	// Platform interface for this instance
	platform, err := m.Connecter.Connect(instanceID)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to platform")
	}

	// Logger specialised to this instance
	instanceLogger := log.NewContext(m.Logger).With("instanceID", instanceID)

	// Events for this instance
	eventRW := EventReadWriter{instanceID, m.History}

	// Configuration for this instance
	config := configurer{instanceID, m.DB}

	return New(
		platform,
		config,
		instanceLogger,
		eventRW,
		eventRW,
	), nil
}
