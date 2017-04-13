package daemon

import (
	"time"

	"github.com/pkg/errors"

	"github.com/weaveworks/flux"
	"github.com/weaveworks/flux/git"
	"github.com/weaveworks/flux/platform"
	"github.com/weaveworks/flux/registry"
	"github.com/weaveworks/flux/release"
)

// Combine these things to form Devasta^Wan implementation of
// Platform.
type Daemon struct {
	V          string
	Cluster    platform.Cluster
	Registry   registry.Registry
	Repo       git.Repo
	WorkingDir string
}

// Invariant.
var _ platform.Platform = &Daemon{}

func (d *Daemon) Version() (string, error) {
	return d.V, nil
}

func (d *Daemon) Ping() error {
	return d.Cluster.Ping()
}

func (d *Daemon) Export() ([]byte, error) {
	return d.Cluster.Export()
}

func (d *Daemon) ListServices(namespace string) ([]flux.ServiceStatus, error) {
	var res []flux.ServiceStatus
	services, err := d.Cluster.AllServices(namespace)
	if err != nil {
		return nil, errors.Wrap(err, "getting services from cluster")
	}
	for _, service := range services {
		res = append(res, flux.ServiceStatus{
			ID:         service.ID,
			Containers: containers2containers(service.ContainersOrNil()),
			Status:     service.Status,
		})
	}
	return res, nil
}

// List the images available for set of services
func (d *Daemon) ListImages(spec flux.ServiceSpec) ([]flux.ImageStatus, error) {
	var services []platform.Service
	var err error
	if spec == flux.ServiceSpecAll {
		services, err = d.Cluster.AllServices("")
	} else {
		id, err := spec.AsID()
		if err != nil {
			return nil, errors.Wrap(err, "treating service spec as ID")
		}
		services, err = d.Cluster.SomeServices([]flux.ServiceID{id})
	}

	images, err := release.CollectAvailableImages(d.Registry, services)
	if err != nil {
		return nil, errors.Wrap(err, "getting images for services")
	}

	var res []flux.ImageStatus
	for _, service := range services {
		containers := containersWithAvailable(service, images)
		res = append(res, flux.ImageStatus{
			ID:         service.ID,
			Containers: containers,
		})
	}

	return res, nil
}

// Apply the desired changes to the config files
func (d *Daemon) UpdateImages(spec flux.ReleaseSpec) (flux.ReleaseResult, error) {
	started := time.Now()
	rc := release.NewReleaseContext(d.Cluster, d.Registry, d.Repo, d.WorkingDir)
	// FIXME update the repo?
	results, err := release.Release(rc, spec)

	status := flux.ReleaseStatusSuccess
	if err != nil {
		status = flux.ReleaseStatusFailed
	}

	release := flux.Release{
		StartedAt: started,
		// TODO: fetch the job and look this up so it matches
		// (which must be done after completing the job)
		EndedAt: time.Now().UTC(),
		Done:    true,
		Status:  status,
		// %%%FIXME reinstate the log, if it's useful
		//		Log:      logged,

		// %%% FIXME where does this come from? Redesign
		//		Cause:  job.Params.(jobs.ReleaseJobParams).Cause,
		Spec:   spec,
		Result: results,
	}
	err = d.logRelease(err, release)
	return results, err
}

// Tell the daemon to synchronise the cluster with the manifests in
// the git repo.
func (d *Daemon) SyncCluster() error {
	return errors.New("FIXME")
}

// Ask the daemon how far it's got applying things; in particular, is it
// past the supplied release? Return the list of commits between where
// we have applied and the ref given, inclusive. E.g., if you send HEAD,
// you'll get all the commits yet to be applied. If you send a hash
// and it's applied _past_ it, you'll get an empty list.
func (d *Daemon) SyncStatus(commitRef string) ([]string, error) {
	return nil, errors.New("FIXME")
}

// Non-platform.Platform methods

// `logEvent` expects the result of applying updates, and records an event in
// the history about the release taking place. It returns the origin error if
// that was non-nil, otherwise the result of the attempted logging.
func (d *Daemon) logRelease(executeErr error, release flux.Release) error {
	errorMessage := ""
	logLevel := flux.LogLevelInfo
	if executeErr != nil {
		errorMessage = executeErr.Error()
		logLevel = flux.LogLevelError
	}

	var serviceIDs []flux.ServiceID
	for _, id := range release.Result.ServiceIDs() {
		serviceIDs = append(serviceIDs, flux.ServiceID(id))
	}

	err := d.LogEvent(flux.Event{
		ServiceIDs: serviceIDs,
		Type:       flux.EventRelease,
		StartedAt:  release.StartedAt,
		EndedAt:    release.EndedAt,
		LogLevel:   logLevel,
		Metadata: flux.ReleaseEventMetadata{
			Release: release,
			Error:   errorMessage,
		},
	})
	if err != nil {
		if executeErr == nil {
			return errors.Wrap(err, "logging event")
		}
	}
	return executeErr
}

func (d *Daemon) LogEvent(ev flux.Event) error {
	// FIXME FIX FIXMEEEEEEE
	return nil
}

// vvv helpers vvv

func containers2containers(cs []platform.Container) []flux.Container {
	res := make([]flux.Container, len(cs))
	for i, c := range cs {
		id, _ := flux.ParseImageID(c.Image)
		res[i] = flux.Container{
			Name: c.Name,
			Current: flux.ImageDescription{
				ID: id,
			},
		}
	}
	return res
}

func containersWithAvailable(service platform.Service, images release.ImageMap) (res []flux.Container) {
	for _, c := range service.ContainersOrNil() {
		id, _ := flux.ParseImageID(c.Image)
		repo := id.Repository()
		available := images[repo]
		res = append(res, flux.Container{
			Name: c.Name,
			Current: flux.ImageDescription{
				ID: id,
			},
			Available: available,
		})
	}
	return res
}
