package release

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/weaveworks/flux"
	"github.com/weaveworks/flux/git"
	"github.com/weaveworks/flux/platform"
)

type ReleaseContext struct {
	Cluster    platform.Cluster
	Repo       git.Repo
	WorkingDir string
}

func NewReleaseContext(c platform.Cluster, repo git.Repo) *ReleaseContext {
	return &ReleaseContext{
		Cluster: c,
		Repo:    repo,
	}
}

// Repo operations

func (rc *ReleaseContext) CloneRepo() error {
	path, err := rc.Repo.Clone()
	if err != nil {
		return err
	}
	rc.WorkingDir = path
	return nil
}

func (rc *ReleaseContext) CommitAndPush(msg string) error {
	return rc.Repo.CommitAndPush(rc.WorkingDir, msg)
}

func (rc *ReleaseContext) RepoPath() string {
	return filepath.Join(rc.WorkingDir, rc.Repo.Path)
}

func (rc *ReleaseContext) PushChanges(updates []*ServiceUpdate, spec *flux.ReleaseSpec) error {
	err := writeUpdates(updates)
	if err != nil {
		return err
	}

	commitMsg := commitMessageFromReleaseSpec(spec)
	return rc.CommitAndPush(commitMsg)
}

func writeUpdates(updates []*ServiceUpdate) error {
	for _, update := range updates {
		fi, err := os.Stat(update.ManifestPath)
		if err != nil {
			return err
		}
		if err = ioutil.WriteFile(update.ManifestPath, update.ManifestBytes, fi.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func (rc *ReleaseContext) Clean() {
	if rc.WorkingDir != "" {
		os.RemoveAll(rc.WorkingDir)
	}
}

// Compiling lists of defined and running services. These need the
// release context because they look at files in the working
// directory.

// SelectServices finds the services that exist both in the definition
// files and the running platform. If given a list of `flux.ServiceID`
// as the first argument, it will include *only* those services, and
// treat missing services as *skipped*; otherwise, it will include all
// services, and *ignore* those that are defined by not
// running. Services in the locked and excluded sets are omitted (and
// recorded as so). The return value is a set of potentially
// updateable services.
func (rc *ReleaseContext) SelectServices(included []flux.ServiceID, locked flux.ServiceIDSet, excluded flux.ServiceIDSet, results flux.ReleaseResult) ([]*ServiceUpdate, error) {
	// Figure out all services that are defined in the repo and should
	// be selected for upgrading/applying.
	defined, err := rc.FindDefinedServices()
	if err != nil {
		return nil, err
	}

	updateMap := map[flux.ServiceID]*ServiceUpdate{}
	var ids []flux.ServiceID

	// The services to explicitly include; this is left as nil if only
	// was not given, since we treat missing services slightly
	// differently depending on whether an exact set was requested.
	var only flux.ServiceIDSet
	if included != nil {
		only = flux.ServiceIDSet{}
		only.Add(included)
	}

	for _, s := range defined {
		if only == nil || only.Contains(s.ServiceID) {
			var result flux.ServiceResult
			switch {
			case excluded.Contains(s.ServiceID):
				result = flux.ServiceResult{
					Status: flux.ReleaseStatusSkipped,
					Error:  "excluded",
				}
			case locked.Contains(s.ServiceID):
				result = flux.ServiceResult{
					Status: flux.ReleaseStatusSkipped,
					Error:  "locked",
				}
			default:
				result = flux.ServiceResult{
					Status: flux.ReleaseStatusPending,
				}
				updateMap[s.ServiceID] = s
				ids = append(ids, s.ServiceID)
			}
			results[s.ServiceID] = result
		}
	}

	// Correlate with services in running system.
	services, err := rc.Cluster.SomeServices(ids)
	if err != nil {
		return nil, err
	}

	var updates []*ServiceUpdate
	for _, service := range services {
		update := updateMap[service.ID]
		update.Service = service
		updates = append(updates, update)
		delete(updateMap, service.ID)
	}
	// Mark anything left over as skipped
	for id, _ := range updateMap {
		status := flux.ReleaseStatusIgnored
		if included != nil { // i.e., this service was specifically requested
			status = flux.ReleaseStatusSkipped
		}
		results[id] = flux.ServiceResult{
			Status: status,
			Error:  "not in running system",
		}
	}

	return updates, nil
}

func (rc *ReleaseContext) FindDefinedServices() ([]*ServiceUpdate, error) {
	services, err := rc.Cluster.FindDefinedServices(rc.RepoPath())
	if err != nil {
		return nil, err
	}

	var defined []*ServiceUpdate
	for id, paths := range services {
		switch len(paths) {
		case 1:
			def, err := ioutil.ReadFile(paths[0])
			if err != nil {
				return nil, err
			}
			defined = append(defined, &ServiceUpdate{
				ServiceID:     id,
				ManifestPath:  paths[0],
				ManifestBytes: def,
			})
		default:
			return nil, fmt.Errorf("multiple resource files found for service %s: %s", id, strings.Join(paths, ", "))
		}
	}
	return defined, nil
}

func (rc *ReleaseContext) LockedServices() flux.ServiceIDSet {
	// FIXME look at the annotations, if that's what we do
	return flux.ServiceIDSet{}
}
