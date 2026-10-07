package contracts

import "example.com/solo-0021-metrology-assurance-suite/internal/domain"

type RegisterDeviceRequest struct {
	AssetTag string `json:"asset_tag"`
	Model    string `json:"model"`
	Room     string `json:"room"`
	Steward  string `json:"steward"`
}

type OpenCaseRequest struct {
	DeviceID  string  `json:"device_id"`
	Nominal   float64 `json:"nominal"`
	Tolerance float64 `json:"tolerance"`
	Steward   string  `json:"steward"`
}

type RecordMeasurementRequest struct {
	Observed   float64 `json:"observed"`
	Technician string  `json:"technician"`
}

type ReopenCaseRequest struct {
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

type SealCaseRequest struct {
	Reviewer string `json:"reviewer"`
	Summary  string `json:"summary"`
}

// ConfirmDeviceCommissioningRequest is the site confirmation submitted before a
// device may enter formal verification work.
type ConfirmDeviceCommissioningRequest struct {
	ResponsibleParty string `json:"responsible_party"`
	InstallLocation  string `json:"install_location"`
	FoundationState  string `json:"foundation_state"`
	Basis            string `json:"basis"`
	ConfirmedBy      string `json:"confirmed_by"`
}

type DeviceResponse struct {
	Device        domain.Device                      `json:"device"`
	Confirmations []domain.CommissioningConfirmation `json:"confirmations"`
}

type ConfirmationsResponse struct {
	Confirmations []domain.CommissioningConfirmation `json:"confirmations"`
}

type CaseResponse struct {
	Case         domain.VerificationCase `json:"case"`
	Device       domain.Device           `json:"device"`
	Measurements []domain.Measurement    `json:"measurements"`
	Seal         *domain.EvidenceSeal    `json:"seal,omitempty"`
	Events       []domain.AuditEvent     `json:"events"`
}

type EventsResponse struct {
	Events []domain.AuditEvent `json:"events"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
