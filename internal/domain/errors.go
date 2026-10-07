package domain

import "errors"

var (
	ErrInvalidInput              = errors.New("invalid input")
	ErrNotFound                  = errors.New("resource not found")
	ErrDuplicateAssetTag         = errors.New("duplicate asset tag")
	ErrActiveCaseExists          = errors.New("device already has an unfinished case")
	ErrStateConflict             = errors.New("resource state conflict")
	ErrSealAlreadyCreated        = errors.New("case already has an evidence seal")
	ErrDeviceNotCommissioned     = errors.New("device has not passed commissioning confirmation")
	ErrDeviceAlreadyCommissioned = errors.New("device has already passed commissioning confirmation")
)

func IsKnownError(err error) bool {
	return errors.Is(err, ErrInvalidInput) ||
		errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrDuplicateAssetTag) ||
		errors.Is(err, ErrActiveCaseExists) ||
		errors.Is(err, ErrStateConflict) ||
		errors.Is(err, ErrSealAlreadyCreated) ||
		errors.Is(err, ErrDeviceNotCommissioned) ||
		errors.Is(err, ErrDeviceAlreadyCommissioned)
}
