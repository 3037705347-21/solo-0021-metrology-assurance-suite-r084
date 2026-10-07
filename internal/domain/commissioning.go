package domain

import (
	"fmt"
	"strings"
	"time"
)

// CommissioningConfirmation is the immutable site record produced when a
// responsible person confirms the install location, foundation state, the
// accountable party, and the evidence basis before a device enters service.
type CommissioningConfirmation struct {
	ID               string    `json:"id"`
	DeviceID         string    `json:"device_id"`
	ResponsibleParty string    `json:"responsible_party"`
	InstallLocation  string    `json:"install_location"`
	FoundationState  string    `json:"foundation_state"`
	Basis            string    `json:"basis"`
	ConfirmedBy      string    `json:"confirmed_by"`
	ConfirmedAt      time.Time `json:"confirmed_at"`
}

func NewCommissioningConfirmation(id, deviceID, responsibleParty, installLocation, foundationState, basis, confirmedBy string, at time.Time) (CommissioningConfirmation, error) {
	item := CommissioningConfirmation{
		ID:               strings.TrimSpace(id),
		DeviceID:         strings.TrimSpace(deviceID),
		ResponsibleParty: strings.TrimSpace(responsibleParty),
		InstallLocation:  strings.TrimSpace(installLocation),
		FoundationState:  strings.TrimSpace(foundationState),
		Basis:            strings.TrimSpace(basis),
		ConfirmedBy:      strings.TrimSpace(confirmedBy),
		ConfirmedAt:      at.UTC(),
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
	if c.ResponsibleParty == "" || c.InstallLocation == "" || c.FoundationState == "" || c.Basis == "" || c.ConfirmedBy == "" {
		return fmt.Errorf("%w: responsible party, install location, foundation state, basis, and confirmer are required", ErrInvalidInput)
	}
	if len(c.ResponsibleParty) > 200 || len(c.InstallLocation) > 200 || len(c.FoundationState) > 200 || len(c.ConfirmedBy) > 200 {
		return fmt.Errorf("%w: commissioning text fields are too long", ErrInvalidInput)
	}
	if len(c.Basis) > 1024 {
		return fmt.Errorf("%w: confirmation basis is too long", ErrInvalidInput)
	}
	if c.ConfirmedAt.IsZero() {
		return fmt.Errorf("%w: confirmation time is required", ErrInvalidInput)
	}
	return nil
}
