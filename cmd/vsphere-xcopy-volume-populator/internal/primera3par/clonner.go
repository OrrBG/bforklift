package primera3par

import (
	"context"
	"fmt"

	"github.com/kubev2v/forklift/cmd/vsphere-xcopy-volume-populator/internal/populator"
)

const PROVIDER_ID = "60002ac"

// Primera3ParClonner now uses the new StorageIdentifier for host mapping.
type Primera3ParClonner struct {
	client Primera3ParClient
}

func NewPrimera3ParClonner(storageHostname, storageUsername, storagePassword string, sslSkipVerify bool) (Primera3ParClonner, error) {
	clon := NewPrimera3ParClientWsImpl(storageHostname, storageUsername, storagePassword, sslSkipVerify)
	return Primera3ParClonner{
		client: &clon,
	}, nil
}

// EnsureClonnerIgroup creates or updates an initiator group using the host-level identifier.
// The identifier parameter can contain both IQN and WWN keys, but the primary ID (IDType) is used.
func (c *Primera3ParClonner) EnsureClonnerIgroup(initiatorGroup string, id populator.StorageIdentifier) (populator.MappingContext, error) {
	var hostName string
	var err error
	switch id.IDType {
	case "iqn":
		hostName, err = c.client.EnsureHostWithIqn(id.IDValue["iqn"])
	case "wwn":
		hostName, err = c.client.EnsureHostWithWWN(id.IDValue["wwn"])
	default:
		return nil, fmt.Errorf("unsupported storage identifier type: %s", id.IDType)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to ensure host with given identifier: %w", err)
	}

	// Ensure that the host set (initiator group) exists.
	err = c.client.EnsureHostSetExists(initiatorGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure host set: %w", err)
	}

	// Add the host (determined by the identifier) to the host set.
	err = c.client.AddHostToHostSet(initiatorGroup, hostName)
	if err != nil {
		return nil, fmt.Errorf("failed to add host to host set: %w", err)
	}

	return nil, nil
}

// Map delegates mapping the LUN to the underlying client.
func (c *Primera3ParClonner) Map(initiatorGroup string, targetLUN populator.LUN, mappingContext populator.MappingContext) (populator.LUN, error) {
	return c.client.EnsureLunMapped(initiatorGroup, targetLUN)
}

// UnMap delegates unmapping the LUN.
func (c *Primera3ParClonner) UnMap(initiatorGroup string, targetLUN populator.LUN, mappingContext populator.MappingContext) error {
	return c.client.LunUnmap(context.TODO(), initiatorGroup, targetLUN.Name)
}

// ResolveVolumeHandleToLUN resolves a volume handle to a LUN by calling the client.
func (c *Primera3ParClonner) ResolveVolumeHandleToLUN(volumeHandle string) (populator.LUN, error) {
	lun := populator.LUN{VolumeHandle: volumeHandle}
	lun, err := c.client.GetLunDetailsByVolumeName(volumeHandle, lun)
	if err != nil {
		return populator.LUN{}, err
	}
	return lun, nil
}

// CurrentMappedGroups returns the groups that the LUN is mapped to.
func (p *Primera3ParClonner) CurrentMappedGroups(targetLUN populator.LUN, mappingContext populator.MappingContext) ([]string, error) {
	res, err := p.client.CurrentMappedGroups(targetLUN.Name, nil)
	if err != nil {
		return []string{}, fmt.Errorf("failed to get current mapped groups: %w", err)
	}
	return res, nil
}
