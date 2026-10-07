package domain

import (
	"fmt"
	"strings"
	"time"
)

type DeviceStatus string

const (
	// DevicePendingCommissioning devices are registered but have not passed the
	// site confirmation of install location, foundation state, and owner. They
	// cannot open verification cases.
	DevicePendingCommissioning DeviceStatus = "pending_commissioning"
	DeviceActive               DeviceStatus = "active"
	DeviceRetired              DeviceStatus = "retired"
)

type Device struct {
	ID         string       `json:"id"`
	AssetTag   string       `json:"asset_tag"`
	Model      string       `json:"model"`
	Room       string       `json:"room"`
	Steward    string       `json:"steward"`
	Status     DeviceStatus `json:"status"`
	EnrolledAt time.Time    `json:"enrolled_at"`
	// CommissionedAt is set exactly once, when the first successful commissioning
	// confirmation moves the device into active service.
	CommissionedAt *time.Time `json:"commissioned_at,omitempty"`
	RetiredAt      *time.Time `json:"retired_at,omitempty"`
}

func NewDevice(id, assetTag, model, room, steward string, at time.Time) (Device, error) {
	device := Device{
		ID:         strings.TrimSpace(id),
		AssetTag:   NormalizeAssetTag(assetTag),
		Model:      strings.TrimSpace(model),
		Room:       strings.TrimSpace(room),
		Steward:    strings.TrimSpace(steward),
		Status:     DevicePendingCommissioning,
		EnrolledAt: at.UTC(),
	}
	if err := device.Validate(); err != nil {
		return Device{}, err
	}
	return device, nil
}

func NormalizeAssetTag(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func (d Device) Validate() error {
	if d.ID == "" || d.AssetTag == "" || d.Model == "" || d.Room == "" || d.Steward == "" {
		return fmt.Errorf("%w: device id, asset tag, model, room, and steward are required", ErrInvalidInput)
	}
	if len(d.AssetTag) > 64 {
		return fmt.Errorf("%w: asset tag is too long", ErrInvalidInput)
	}
	if d.EnrolledAt.IsZero() {
		return fmt.Errorf("%w: enrollment time is required", ErrInvalidInput)
	}
	switch d.Status {
	case DevicePendingCommissioning, DeviceActive, DeviceRetired:
		return nil
	default:
		return fmt.Errorf("%w: unsupported device state %q", ErrInvalidInput, d.Status)
	}
}

// IsActive reports whether the device is in service and may open cases.
func (d Device) IsActive() bool {
	return d.Status == DeviceActive
}

// IsCommissioned reports whether the device has ever passed commissioning.
// Retired devices remain commissioned; the lifecycle fact is not erased.
func (d Device) IsCommissioned() bool {
	return d.Status == DeviceActive || d.Status == DeviceRetired
}

// Commission applies the only successful commissioning confirmation. It moves a
// pending device to active service and stamps the commissioning time exactly
// once. A repeat confirmation cannot rewrite device facts.
func (d *Device) Commission(at time.Time) error {
	if d.Status == DeviceActive || d.Status == DeviceRetired {
		return fmt.Errorf("%w: device %s is already commissioned", ErrDeviceAlreadyCommissioned, d.ID)
	}
	if d.Status != DevicePendingCommissioning {
		return fmt.Errorf("%w: device %s is %s and cannot be commissioned", ErrStateConflict, d.ID, d.Status)
	}
	stamp := at.UTC()
	d.Status = DeviceActive
	d.CommissionedAt = &stamp
	return nil
}

func (d *Device) Retire(at time.Time) error {
	if d.Status != DeviceActive {
		return fmt.Errorf("%w: only an active device can be retired", ErrStateConflict)
	}
	stamp := at.UTC()
	d.Status = DeviceRetired
	d.RetiredAt = &stamp
	return nil
}
