package domain

import (
	"fmt"
	"strings"
	"time"
)

type CommissioningOutcome string

const (
	CommissioningAccepted CommissioningOutcome = "accepted"
	CommissioningRejected CommissioningOutcome = "rejected"
)

// CommissioningConfirmation is an immutable on-site commissioning fact for a
// device. Rejected confirmations are retained as history but never change the
// device, while exactly one accepted confirmation enables the device.
type CommissioningConfirmation struct {
	ID            string               `json:"id"`
	DeviceID      string               `json:"device_id"`
	Responsible   string               `json:"responsible"`
	InstalledRoom string               `json:"installed_room"`
	Foundation    string               `json:"foundation"`
	Evidence      string               `json:"evidence"`
	Outcome       CommissioningOutcome `json:"outcome"`
	ConfirmedAt   time.Time            `json:"confirmed_at"`
}

func NewCommissioningConfirmation(id, deviceID, responsible, installedRoom, foundation, evidence string, outcome CommissioningOutcome, at time.Time) (CommissioningConfirmation, error) {
	item := CommissioningConfirmation{
		ID:            strings.TrimSpace(id),
		DeviceID:      strings.TrimSpace(deviceID),
		Responsible:   strings.TrimSpace(responsible),
		InstalledRoom: strings.TrimSpace(installedRoom),
		Foundation:    strings.TrimSpace(foundation),
		Evidence:      strings.TrimSpace(evidence),
		Outcome:       outcome,
		ConfirmedAt:   at.UTC(),
	}
	if err := item.Validate(); err != nil {
		return CommissioningConfirmation{}, err
	}
	return item, nil
}

func (c CommissioningConfirmation) Validate() error {
	if c.ID == "" || c.DeviceID == "" {
		return fmt.Errorf("%w: confirmation id and device id are required", ErrInvalidInput)
	}
	if c.Responsible == "" || c.InstalledRoom == "" || c.Foundation == "" || c.Evidence == "" {
		return fmt.Errorf("%w: responsible party, installed location, foundation status, and evidence are required", ErrInvalidInput)
	}
	if c.ConfirmedAt.IsZero() {
		return fmt.Errorf("%w: confirmation time is required", ErrInvalidInput)
	}
	switch c.Outcome {
	case CommissioningAccepted, CommissioningRejected:
		return nil
	default:
		return fmt.Errorf("%w: unsupported commissioning outcome %q", ErrInvalidInput, c.Outcome)
	}
}
