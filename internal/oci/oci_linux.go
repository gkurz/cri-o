package oci

import (
	"os"
	"syscall"
	"time"

	"github.com/cri-o/cri-o/server/cri/types"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/sys/unix"
)

func (r *runtimeOCI) createContainerPlatform(c *Container, cgroupParent string, pid int) error {
	if c.Spoofed() {
		return nil
	}
	g := &generate.Generator{
		Config: &rspec.Spec{
			Linux: &rspec.Linux{
				Resources: &rspec.LinuxResources{},
			},
		},
	}
	// Mutate our newly created spec to find the customizations that are needed for conmon
	if err := r.config.Workloads.MutateSpecGivenAnnotations(types.InfraContainerName, g, c.Annotations()); err != nil {
		return err
	}

	// Move conmon to specified cgroup
	conmonCgroupfsPath, err := r.config.CgroupManager().MoveConmonToCgroup(c.ID(), cgroupParent, r.config.ConmonCgroup, pid, g.Config.Linux.Resources)
	if err != nil {
		return err
	}
	c.conmonCgroupfsPath = conmonCgroupfsPath
	return nil
}

func sysProcAttrPlatform() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// newPipe creates a unix socket pair for communication
func newPipe() (parent, child *os.File, _ error) {
	fds, err := unix.Socketpair(unix.AF_LOCAL, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(fds[1]), "parent"), os.NewFile(uintptr(fds[0]), "child"), nil
}

func (r *runtimeOCI) containerStats(ctr *Container, cgroup string) (*types.ContainerStats, error) {
	stats := &types.ContainerStats{
		Attributes: ctr.CRIAttributes(),
	}

	if ctr.Spoofed() {
		return stats, nil
	}

	// technically, the CRI does not mandate a CgroupParent is given to a pod
	// this situation should never happen in production, but some test suites
	// (such as critest) assume we can call stats on a cgroupless container
	if cgroup == "" {
		systemNano := time.Now().UnixNano()
		stats.CPU = &types.CPUUsage{
			Timestamp: systemNano,
		}
		stats.Memory = &types.MemoryUsage{
			Timestamp: systemNano,
		}
		stats.WritableLayer = &types.FilesystemUsage{
			Timestamp: systemNano,
		}
		return stats, nil
	}
	// update the stats object with information from the cgroup
	if err := r.config.CgroupManager().PopulateContainerCgroupStats(cgroup, ctr.ID(), stats); err != nil {
		return nil, err
	}
	return stats, nil
}
