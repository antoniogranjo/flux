package platform

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/weaveworks/flux"
	"github.com/weaveworks/flux/git"
	"github.com/weaveworks/flux/registry"
)

// Combine these things to form Devasta^Wan implementation of
// Platform.
type Daemon struct {
	V        string
	Cluster  Cluster
	Registry registry.Registry
	Repo     git.Repo
}

type DaemonV6 interface {
	Version() (string, error)
	ListServices(namespace string) ([]flux.ServiceStatus, error)
	ListImages(flux.ServiceSpec) ([]flux.ImageStatus, error)
	UpdateImages(flux.ReleaseSpec) error
	SyncStatus(string) ([]string, error)
	DumpConfig() ([]byte, error)
}

// Invariant
var _ DaemonV6 = &Daemon{}

// The things we can get from the running cluster. These used to form
// the Platform interface; but now we do more in the daemon so they
// are distinct interfaces.
type Cluster interface {
	AllServices(maybeNamespace string, ignored flux.ServiceIDSet) ([]Service, error)
	SomeServices([]flux.ServiceID) ([]Service, error)
	Ping() error
	Export() ([]byte, error)
	Sync(SyncDef) error
}

func (d *Daemon) Version() (string, error) {
	return d.V, nil
}

func (d *Daemon) DumpConfig() ([]byte, error) {
	return d.Cluster.Export()
}

func (d *Daemon) ListServices(namespace string) ([]flux.ServiceStatus, error) {
	var res []flux.ServiceStatus
	services, err := d.getAllServices(namespace)
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
	var services []Service
	var err error
	if spec == flux.ServiceSpecAll {
		services, err = d.getAllServices("")
	} else {
		id, err := spec.AsID()
		if err != nil {
			return nil, errors.Wrap(err, "treating service spec as ID")
		}
		services, err = d.getServices([]flux.ServiceID{id})
	}

	images, err := d.collectAvailableImages(services)
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
func (d *Daemon) UpdateImages(flux.ReleaseSpec) error {
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

//

// vvv helpers vvv

// Get the services in `namespace` along with their containers (if
// there are any) from the platform; if namespace is blank, just get
// all the services, in any namespace.
func (d *Daemon) getAllServices(maybeNamespace string) ([]Service, error) {
	return d.getAllServicesExcept(maybeNamespace, flux.ServiceIDSet{})
}

// Get all services except those with an ID in the set given
func (d *Daemon) getAllServicesExcept(maybeNamespace string, ignored flux.ServiceIDSet) (res []Service, err error) {
	return d.Cluster.AllServices(maybeNamespace, ignored)
}

// Get the services mentioned, along with their containers.
func (d *Daemon) getServices(ids []flux.ServiceID) ([]Service, error) {
	return d.Cluster.SomeServices(ids)
}

func containers2containers(cs []Container) []flux.Container {
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

// For keeping track of which images are available
type ImageMap map[string][]flux.ImageDescription

// Get the images available for the services given. An image may be
// mentioned more than once in the services, but will only be fetched
// once.
func (d *Daemon) collectAvailableImages(services []Service) (ImageMap, error) {
	images := ImageMap{}
	for _, service := range services {
		for _, container := range service.ContainersOrNil() {
			id, err := flux.ParseImageID(container.Image)
			if err != nil {
				// container is running an invalid image id? what?
				return nil, err
			}
			images[id.Repository()] = nil
		}
	}
	for repo := range images {
		r, err := registry.ParseRepository(repo)
		if err != nil {
			return nil, errors.Wrapf(err, "parsing repository %s", repo)
		}
		imageRepo, err := d.Registry.GetRepository(r)
		if err != nil {
			return nil, errors.Wrapf(err, "fetching image metadata for %s", repo)
		}
		res := make([]flux.ImageDescription, len(imageRepo))
		for i, im := range imageRepo {
			id, err := flux.ParseImageID(im.String())
			if err != nil {
				// registry returned an invalid image id
				return nil, err
			}
			res[i] = flux.ImageDescription{
				ID:        id,
				CreatedAt: im.CreatedAt,
			}
		}
		images[repo] = res
	}
	return images, nil
}

// LatestImage returns the latest releasable image for a repository.
// A releasable image is one that is not tagged "latest". (Assumes the
// available images are in descending order of latestness.) If no such
// image exists, returns nil, and the caller can decide whether that's
// an error or not.
func (m ImageMap) latestImage(repo string) *flux.ImageDescription {
	for _, image := range m[repo] {
		_, _, tag := image.ID.Components()
		if strings.EqualFold(tag, "latest") {
			continue
		}
		return &image
	}
	return nil
}

func containersWithAvailable(service Service, images ImageMap) (res []flux.Container) {
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
