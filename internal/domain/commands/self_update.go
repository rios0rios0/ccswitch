package commands

import (
	"fmt"

	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// SelfUpdateCommand replaces the running ccswitch binary with the latest release.
type SelfUpdateCommand struct {
	updates repositories.SelfUpdateRepository
}

// NewSelfUpdateCommand creates a SelfUpdateCommand.
func NewSelfUpdateCommand(updates repositories.SelfUpdateRepository) *SelfUpdateCommand {
	return &SelfUpdateCommand{updates: updates}
}

// Execute installs the latest release when it is newer than the running binary,
// and reports whether it did. dryRun only reports what would be installed, and
// force skips the confirmation prompt.
func (c *SelfUpdateCommand) Execute(dryRun, force bool) (bool, error) {
	installed, err := c.updates.Update(dryRun, force)
	if err != nil {
		return false, fmt.Errorf("failed to update ccswitch: %w", err)
	}
	return installed, nil
}
