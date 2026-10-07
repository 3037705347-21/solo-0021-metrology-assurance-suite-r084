package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	"example.com/solo-0021-metrology-assurance-suite/internal/bootstrap"
)

type apiResponse struct {
	Status int
	Body   map[string]any
	Raw    string
}

type harness struct {
	baseURL string
	client  *http.Client
	server  *httptest.Server
}

func main() {
	workflow := flag.String("workflow", "", "workflow id to verify")
	flag.Parse()

	checks := map[string]func() error{
		"register-device":          checkRegisterDevice,
		"commission-device":        checkCommissionDevice,
		"open-verification-case":   checkOpenVerificationCase,
		"record-measurement":       checkRecordMeasurement,
		"reopen-verification-case": checkReopenVerificationCase,
		"seal-verification":        checkSealVerification,
	}
	run, ok := checks[*workflow]
	if !ok {
		fmt.Fprintf(flag.CommandLine.Output(), "unknown workflow %q\n", *workflow)
		flag.Usage()
		os.Exit(2)
	}
	if err := run(); err != nil {
		fmt.Printf("workflow %s: failed: %v\n", *workflow, err)
		os.Exit(1)
	}
	fmt.Printf("workflow %s: ok\n", *workflow)
}

func checkRegisterDevice() error {
	h := newHarness()
	defer h.close()

	response, err := h.request(http.MethodPost, "/devices", map[string]any{
		"asset_tag": " met-1001 ",
		"model":     "Bench Meter",
		"room":      "Room A",
		"steward":   "Lin",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(response, http.StatusCreated); err != nil {
		return err
	}
	id, err := nestedString(response.Body, "device", "id")
	if err != nil {
		return err
	}
	tag, err := nestedString(response.Body, "device", "asset_tag")
	if err != nil {
		return err
	}
	if tag != "MET-1001" {
		return fmt.Errorf("asset tag was not normalized: %q", tag)
	}

	read, err := h.request(http.MethodGet, "/devices/"+id, nil)
	if err != nil {
		return err
	}
	if err := expectStatus(read, http.StatusOK); err != nil {
		return err
	}
	readID, err := nestedString(read.Body, "device", "id")
	if err != nil {
		return err
	}
	if readID != id {
		return fmt.Errorf("read returned device %q, expected %q", readID, id)
	}

	duplicate, err := h.request(http.MethodPost, "/devices", map[string]any{
		"asset_tag": "MET-1001",
		"model":     "Other Meter",
		"room":      "Room B",
		"steward":   "Mira",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(duplicate, http.StatusConflict); err != nil {
		return err
	}
	return expectErrorCode(duplicate, "duplicate_asset_tag")
}

func checkCommissionDevice() error {
	h := newHarness()
	defer h.close()

	deviceID, err := h.createDevice("MET-1501")
	if err != nil {
		return err
	}

	// A freshly registered device is pending and must not yet be able to open a case.
	pending, err := h.request(http.MethodGet, "/devices/"+deviceID, nil)
	if err != nil {
		return err
	}
	if err := expectStatus(pending, http.StatusOK); err != nil {
		return err
	}
	pendingStatus, err := nestedString(pending.Body, "device", "status")
	if err != nil {
		return err
	}
	if pendingStatus != "pending_commissioning" {
		return fmt.Errorf("new device status = %q, expected pending_commissioning", pendingStatus)
	}
	if _, err := nestedString(pending.Body, "device", "commissioned_at"); err == nil {
		return errors.New("pending device already carries a commissioned_at timestamp")
	}
	pendingConfirmations, err := nestedSlice(pending.Body, "confirmations")
	if err != nil {
		return err
	}
	if len(pendingConfirmations) != 0 {
		return fmt.Errorf("pending device already has %d confirmations", len(pendingConfirmations))
	}

	blocked, err := h.request(http.MethodPost, "/cases", map[string]any{
		"device_id": deviceID,
		"nominal":   25.0,
		"tolerance": 0.5,
		"steward":   "Qiao",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(blocked, http.StatusConflict); err != nil {
		return err
	}
	if err := expectErrorCode(blocked, "device_not_commissioned"); err != nil {
		return err
	}

	// A malformed confirmation fails validation and must not change device facts.
	invalid, err := h.request(http.MethodPost, "/devices/"+deviceID+"/confirmations", map[string]any{
		"responsible_party": "Lin",
		"install_location":  "Bench 3, Room A",
		"foundation_state":  "",
		"basis":             "site checklist #42",
		"confirmed_by":      "Sora",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(invalid, http.StatusBadRequest); err != nil {
		return err
	}
	if err := expectErrorCode(invalid, "invalid_input"); err != nil {
		return err
	}
	afterInvalid, err := h.request(http.MethodGet, "/devices/"+deviceID, nil)
	if err != nil {
		return err
	}
	if err := expectStatus(afterInvalid, http.StatusOK); err != nil {
		return err
	}
	statusAfterInvalid, err := nestedString(afterInvalid.Body, "device", "status")
	if err != nil {
		return err
	}
	if statusAfterInvalid != "pending_commissioning" {
		return fmt.Errorf("failed confirmation rewrote device status to %q", statusAfterInvalid)
	}
	historyAfterInvalid, err := nestedSlice(afterInvalid.Body, "confirmations")
	if err != nil {
		return err
	}
	if len(historyAfterInvalid) != 0 {
		return fmt.Errorf("failed confirmation appended %d history records", len(historyAfterInvalid))
	}

	// A valid confirmation enables the device in the same atomic commit.
	confirmed, err := h.commissionDevice(deviceID, map[string]any{
		"responsible_party": "Lin",
		"install_location":  "Bench 3, Room A",
		"foundation_state":  "leveled and bolted down",
		"basis":             "site checklist #42 and install report",
		"confirmed_by":      "Sora",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(confirmed, http.StatusCreated); err != nil {
		return err
	}
	activeStatus, err := nestedString(confirmed.Body, "device", "status")
	if err != nil {
		return err
	}
	if activeStatus != "active" {
		return fmt.Errorf("confirmed device status = %q, expected active", activeStatus)
	}
	commissionedAt, err := nestedString(confirmed.Body, "device", "commissioned_at")
	if err != nil {
		return err
	}
	if commissionedAt == "" {
		return errors.New("commissioned device is missing commissioned_at")
	}
	confirmations, err := nestedSlice(confirmed.Body, "confirmations")
	if err != nil {
		return err
	}
	if len(confirmations) != 1 {
		return fmt.Errorf("device view has %d confirmations after success", len(confirmations))
	}
	first, ok := confirmations[0].(map[string]any)
	if !ok {
		return errors.New("confirmation entry is not an object")
	}
	if first["responsible_party"] != "Lin" || first["install_location"] != "Bench 3, Room A" ||
		first["foundation_state"] != "leveled and bolted down" || first["confirmed_by"] != "Sora" {
		return fmt.Errorf("confirmation record does not preserve submitted facts: %v", first)
	}

	// Once enabled, a case can open; the case view carries the same lifecycle fact.
	opened, err := h.openCase(deviceID)
	if err != nil {
		return err
	}
	caseDeviceStatus, err := nestedString(opened.Body, "device", "status")
	if err != nil {
		return err
	}
	if caseDeviceStatus != "active" {
		return fmt.Errorf("case view device status = %q, expected active", caseDeviceStatus)
	}

	// Two confirmations racing a fresh pending device must enable it exactly once.
	raceID, err := h.createDevice("MET-1502")
	if err != nil {
		return err
	}
	type raceOutcome struct {
		response apiResponse
		err      error
	}
	raceResults := make(chan raceOutcome, 2)
	var raceWait sync.WaitGroup
	for index := 0; index < 2; index++ {
		raceWait.Add(1)
		go func(index int) {
			defer raceWait.Done()
			response, err := h.commissionDevice(raceID, map[string]any{
				"responsible_party": fmt.Sprintf("Party-%d", index+1),
				"install_location":  "Bench 4, Room A",
				"foundation_state":  "leveled",
				"basis":             fmt.Sprintf("checklist-%d", index+1),
				"confirmed_by":      "Sora",
			})
			raceResults <- raceOutcome{response: response, err: err}
		}(index)
	}
	raceWait.Wait()
	close(raceResults)
	raceCreated := 0
	raceConflict := 0
	for item := range raceResults {
		if item.err != nil {
			return item.err
		}
		switch item.response.Status {
		case http.StatusCreated:
			raceCreated++
		case http.StatusConflict:
			raceConflict++
		default:
			return fmt.Errorf("unexpected concurrent confirmation status %d: %s", item.response.Status, item.response.Raw)
		}
	}
	if raceCreated != 1 || raceConflict != 1 {
		return fmt.Errorf("concurrent confirmation outcomes: %d created, %d conflict", raceCreated, raceConflict)
	}
	raceHistory, err := h.request(http.MethodGet, "/devices/"+raceID+"/confirmations", nil)
	if err != nil {
		return err
	}
	if err := expectStatus(raceHistory, http.StatusOK); err != nil {
		return err
	}
	raceItems, err := nestedSlice(raceHistory.Body, "confirmations")
	if err != nil {
		return err
	}
	if len(raceItems) != 1 {
		return fmt.Errorf("racing confirmations left %d records, expected 1", len(raceItems))
	}

	// A repeat confirmation is rejected and must not rewrite history or the timestamp.
	duplicate, err := h.commissionDevice(deviceID, map[string]any{
		"responsible_party": "Someone Else",
		"install_location":  "Other Bench",
		"foundation_state":  "unchecked",
		"basis":             "duplicate submission",
		"confirmed_by":      "Other",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(duplicate, http.StatusConflict); err != nil {
		return err
	}
	if err := expectErrorCode(duplicate, "device_already_commissioned"); err != nil {
		return err
	}

	history, err := h.request(http.MethodGet, "/devices/"+deviceID+"/confirmations", nil)
	if err != nil {
		return err
	}
	if err := expectStatus(history, http.StatusOK); err != nil {
		return err
	}
	items, err := nestedSlice(history.Body, "confirmations")
	if err != nil {
		return err
	}
	if len(items) != 1 {
		return fmt.Errorf("repeat confirmation rewrote history to %d records", len(items))
	}
	survivor, ok := items[0].(map[string]any)
	if !ok {
		return errors.New("surviving confirmation is not an object")
	}
	if survivor["responsible_party"] != "Lin" || survivor["confirmed_by"] != "Sora" {
		return fmt.Errorf("repeat confirmation rewrote the device facts: %v", survivor)
	}

	final, err := h.request(http.MethodGet, "/devices/"+deviceID, nil)
	if err != nil {
		return err
	}
	finalCommissionedAt, err := nestedString(final.Body, "device", "commissioned_at")
	if err != nil {
		return err
	}
	if finalCommissionedAt != commissionedAt {
		return fmt.Errorf("commissioned_at changed from %q to %q", commissionedAt, finalCommissionedAt)
	}

	// Confirming a missing device is a stable not-found.
	missing, err := h.commissionDevice("device-9999", map[string]any{
		"responsible_party": "Lin",
		"install_location":  "Bench 3",
		"foundation_state":  "ok",
		"basis":             "checklist",
		"confirmed_by":      "Sora",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(missing, http.StatusNotFound); err != nil {
		return err
	}
	return expectErrorCode(missing, "not_found")
}

func checkOpenVerificationCase() error {
	h := newHarness()
	defer h.close()

	deviceID, err := h.createDevice("MET-1101")
	if err != nil {
		return err
	}
	blocked, err := h.request(http.MethodPost, "/cases", map[string]any{
		"device_id": deviceID,
		"nominal":   25.0,
		"tolerance": 0.5,
		"steward":   "Qiao",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(blocked, http.StatusConflict); err != nil {
		return err
	}
	if err := expectErrorCode(blocked, "device_not_commissioned"); err != nil {
		return err
	}

	if _, err := h.commissionDevice(deviceID, defaultCommissionBody()); err != nil {
		return err
	}
	opened, err := h.openCase(deviceID)
	if err != nil {
		return err
	}
	caseID, err := nestedString(opened.Body, "case", "id")
	if err != nil {
		return err
	}
	state, err := nestedString(opened.Body, "case", "state")
	if err != nil {
		return err
	}
	if state != "awaiting_measurement" {
		return fmt.Errorf("opened case state = %q", state)
	}

	read, err := h.request(http.MethodGet, "/cases/"+caseID, nil)
	if err != nil {
		return err
	}
	if err := expectStatus(read, http.StatusOK); err != nil {
		return err
	}

	second, err := h.request(http.MethodPost, "/cases", map[string]any{
		"device_id": deviceID,
		"nominal":   25.0,
		"tolerance": 0.5,
		"steward":   "Qiao",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(second, http.StatusConflict); err != nil {
		return err
	}
	return expectErrorCode(second, "active_case_exists")
}

func checkRecordMeasurement() error {
	h := newHarness()
	defer h.close()

	deviceID, err := h.createDevice("MET-1201")
	if err != nil {
		return err
	}
	if _, err := h.commissionDevice(deviceID, defaultCommissionBody()); err != nil {
		return err
	}
	opened, err := h.openCase(deviceID)
	if err != nil {
		return err
	}
	caseID, err := nestedString(opened.Body, "case", "id")
	if err != nil {
		return err
	}
	accepted, err := h.recordMeasurement(caseID, 25.2, "Rui")
	if err != nil {
		return err
	}
	if err := expectStatus(accepted, http.StatusCreated); err != nil {
		return err
	}
	state, err := nestedString(accepted.Body, "case", "state")
	if err != nil {
		return err
	}
	if state != "review_ready" {
		return fmt.Errorf("accepted measurement state = %q", state)
	}
	acceptedValue, err := nestedBool(accepted.Body, "measurements", 0, "accepted")
	if err != nil {
		return err
	}
	if !acceptedValue {
		return errors.New("in-tolerance measurement was not accepted")
	}

	second, err := h.recordMeasurement(caseID, 25.1, "Rui")
	if err != nil {
		return err
	}
	if err := expectStatus(second, http.StatusConflict); err != nil {
		return err
	}
	if err := expectErrorCode(second, "state_conflict"); err != nil {
		return err
	}

	rejectDeviceID, err := h.createDevice("MET-1202")
	if err != nil {
		return err
	}
	if _, err := h.commissionDevice(rejectDeviceID, defaultCommissionBody()); err != nil {
		return err
	}
	rejectCase, err := h.openCase(rejectDeviceID)
	if err != nil {
		return err
	}
	rejectCaseID, err := nestedString(rejectCase.Body, "case", "id")
	if err != nil {
		return err
	}
	rejected, err := h.recordMeasurement(rejectCaseID, 26.5, "Rui")
	if err != nil {
		return err
	}
	if err := expectStatus(rejected, http.StatusCreated); err != nil {
		return err
	}
	rejectedState, err := nestedString(rejected.Body, "case", "state")
	if err != nil {
		return err
	}
	if rejectedState != "recheck_required" {
		return fmt.Errorf("out-of-tolerance state = %q", rejectedState)
	}
	rejectedValue, err := nestedBool(rejected.Body, "measurements", 0, "accepted")
	if err != nil {
		return err
	}
	if rejectedValue {
		return errors.New("out-of-tolerance measurement was accepted")
	}
	return nil
}

func checkReopenVerificationCase() error {
	h := newHarness()
	defer h.close()

	caseID, err := h.rejectedCase("MET-1301")
	if err != nil {
		return err
	}
	reopened, err := h.request(http.MethodPost, "/cases/"+caseID+"/reopen", map[string]any{
		"actor":  "Lin",
		"reason": "measurement setup was replaced",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(reopened, http.StatusOK); err != nil {
		return err
	}
	state, err := nestedString(reopened.Body, "case", "state")
	if err != nil {
		return err
	}
	if state != "awaiting_measurement" {
		return fmt.Errorf("reopened case state = %q", state)
	}
	measurements, err := nestedSlice(reopened.Body, "measurements")
	if err != nil {
		return err
	}
	if len(measurements) != 1 {
		return fmt.Errorf("reopen changed measurement history: %d entries", len(measurements))
	}

	second, err := h.request(http.MethodPost, "/cases/"+caseID+"/reopen", map[string]any{
		"actor":  "Lin",
		"reason": "duplicate request",
	})
	if err != nil {
		return err
	}
	if err := expectStatus(second, http.StatusConflict); err != nil {
		return err
	}
	if err := expectErrorCode(second, "state_conflict"); err != nil {
		return err
	}

	events, err := h.request(http.MethodGet, "/cases/"+caseID+"/events", nil)
	if err != nil {
		return err
	}
	if err := expectStatus(events, http.StatusOK); err != nil {
		return err
	}
	if !hasEvent(events.Body, "case_reopened") {
		return errors.New("reopen audit event is missing")
	}
	return nil
}

func checkSealVerification() error {
	h := newHarness()
	defer h.close()

	caseID, err := h.readyCase("MET-1401")
	if err != nil {
		return err
	}
	type outcome struct {
		response apiResponse
		err      error
	}
	results := make(chan outcome, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			response, err := h.request(http.MethodPost, "/cases/"+caseID+"/seal", map[string]any{
				"reviewer": fmt.Sprintf("Reviewer-%d", index+1),
				"summary":  "accepted measurement sealed for review",
			})
			results <- outcome{response: response, err: err}
		}(index)
	}
	wait.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for item := range results {
		if item.err != nil {
			return item.err
		}
		switch item.response.Status {
		case http.StatusCreated:
			successes++
		case http.StatusConflict:
			conflicts++
		default:
			return fmt.Errorf("unexpected concurrent seal status %d: %s", item.response.Status, item.response.Raw)
		}
	}
	if successes != 1 || conflicts != 1 {
		return fmt.Errorf("concurrent seal outcomes: %d success, %d conflict", successes, conflicts)
	}

	read, err := h.request(http.MethodGet, "/cases/"+caseID, nil)
	if err != nil {
		return err
	}
	if err := expectStatus(read, http.StatusOK); err != nil {
		return err
	}
	state, err := nestedString(read.Body, "case", "state")
	if err != nil {
		return err
	}
	if state != "sealed" {
		return fmt.Errorf("sealed case state = %q", state)
	}
	sealID, err := nestedString(read.Body, "seal", "id")
	if err != nil {
		return err
	}
	if sealID == "" {
		return errors.New("evidence seal is missing")
	}
	return nil
}

func newHarness() *harness {
	server := httptest.NewServer(bootstrap.NewHandler())
	return &harness{
		baseURL: server.URL,
		client:  server.Client(),
		server:  server,
	}
}

func (h *harness) close() {
	h.server.Close()
}

func (h *harness) request(method, path string, body any) (apiResponse, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return apiResponse{}, err
		}
		reader = bytes.NewReader(encoded)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, h.baseURL+path, reader)
	if err != nil {
		return apiResponse{}, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := h.client.Do(request)
	if err != nil {
		return apiResponse{}, err
	}
	defer response.Body.Close()
	rawBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return apiResponse{}, err
	}
	result := apiResponse{Status: response.StatusCode, Raw: string(rawBytes)}
	if len(bytes.TrimSpace(rawBytes)) > 0 {
		if err := json.Unmarshal(rawBytes, &result.Body); err != nil {
			return apiResponse{}, fmt.Errorf("decode %s %s response: %w", method, path, err)
		}
	}
	return result, nil
}

func (h *harness) createDevice(tag string) (string, error) {
	response, err := h.request(http.MethodPost, "/devices", map[string]any{
		"asset_tag": tag,
		"model":     "Bench Meter",
		"room":      "Room A",
		"steward":   "Lin",
	})
	if err != nil {
		return "", err
	}
	if err := expectStatus(response, http.StatusCreated); err != nil {
		return "", err
	}
	return nestedString(response.Body, "device", "id")
}

func defaultCommissionBody() map[string]any {
	return map[string]any{
		"responsible_party": "Lin",
		"install_location":  "Bench 1, Room A",
		"foundation_state":  "leveled and bolted down",
		"basis":             "site checklist #7",
		"confirmed_by":      "Sora",
	}
}

// commissionDevice submits a commissioning confirmation and returns the raw
// response so callers can assert on both success and failure outcomes.
func (h *harness) commissionDevice(deviceID string, body map[string]any) (apiResponse, error) {
	return h.request(http.MethodPost, "/devices/"+deviceID+"/confirmations", body)
}

func (h *harness) openCase(deviceID string) (apiResponse, error) {
	response, err := h.request(http.MethodPost, "/cases", map[string]any{
		"device_id": deviceID,
		"nominal":   25.0,
		"tolerance": 0.5,
		"steward":   "Qiao",
	})
	if err != nil {
		return apiResponse{}, err
	}
	if err := expectStatus(response, http.StatusCreated); err != nil {
		return apiResponse{}, err
	}
	return response, nil
}

func (h *harness) recordMeasurement(caseID string, observed float64, technician string) (apiResponse, error) {
	return h.request(http.MethodPost, "/cases/"+caseID+"/measurements", map[string]any{
		"observed":   observed,
		"technician": technician,
	})
}

func (h *harness) rejectedCase(tag string) (string, error) {
	deviceID, err := h.createDevice(tag)
	if err != nil {
		return "", err
	}
	if _, err := h.commissionDevice(deviceID, defaultCommissionBody()); err != nil {
		return "", err
	}
	opened, err := h.openCase(deviceID)
	if err != nil {
		return "", err
	}
	caseID, err := nestedString(opened.Body, "case", "id")
	if err != nil {
		return "", err
	}
	response, err := h.recordMeasurement(caseID, 26.5, "Rui")
	if err != nil {
		return "", err
	}
	if err := expectStatus(response, http.StatusCreated); err != nil {
		return "", err
	}
	return caseID, nil
}

func (h *harness) readyCase(tag string) (string, error) {
	deviceID, err := h.createDevice(tag)
	if err != nil {
		return "", err
	}
	if _, err := h.commissionDevice(deviceID, defaultCommissionBody()); err != nil {
		return "", err
	}
	opened, err := h.openCase(deviceID)
	if err != nil {
		return "", err
	}
	caseID, err := nestedString(opened.Body, "case", "id")
	if err != nil {
		return "", err
	}
	response, err := h.recordMeasurement(caseID, 25.2, "Rui")
	if err != nil {
		return "", err
	}
	if err := expectStatus(response, http.StatusCreated); err != nil {
		return "", err
	}
	return caseID, nil
}

func expectStatus(response apiResponse, expected int) error {
	if response.Status != expected {
		return fmt.Errorf("status = %d, expected %d: %s", response.Status, expected, response.Raw)
	}
	return nil
}

func expectErrorCode(response apiResponse, expected string) error {
	code, err := nestedString(response.Body, "error", "code")
	if err != nil {
		return err
	}
	if code != expected {
		return fmt.Errorf("error code = %q, expected %q", code, expected)
	}
	return nil
}

func nestedString(root map[string]any, path ...string) (string, error) {
	value, err := nestedValue(root, path...)
	if err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s is not a string", strings.Join(path, "."))
	}
	return text, nil
}

func nestedBool(root map[string]any, path ...any) (bool, error) {
	value, err := nestedAny(root, path...)
	if err != nil {
		return false, err
	}
	flag, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%v is not a boolean", path)
	}
	return flag, nil
}

func nestedSlice(root map[string]any, key string) ([]any, error) {
	value, ok := root[key]
	if !ok {
		return nil, fmt.Errorf("missing %s", key)
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an array", key)
	}
	return items, nil
}

func hasEvent(root map[string]any, kind string) bool {
	items, err := nestedSlice(root, "events")
	if err != nil {
		return false
	}
	for _, item := range items {
		event, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if event["kind"] == kind {
			return true
		}
	}
	return false
}

func nestedValue(root map[string]any, path ...string) (any, error) {
	parts := make([]any, 0, len(path))
	for _, part := range path {
		parts = append(parts, part)
	}
	return nestedAny(root, parts...)
}

func nestedAny(root map[string]any, path ...any) (any, error) {
	var current any = root
	for _, part := range path {
		switch typed := part.(type) {
		case string:
			object, ok := current.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s is not an object", typed)
			}
			value, ok := object[typed]
			if !ok {
				return nil, fmt.Errorf("missing %s", typed)
			}
			current = value
		case int:
			items, ok := current.([]any)
			if !ok {
				return nil, fmt.Errorf("index %d is not inside an array", typed)
			}
			if typed < 0 || typed >= len(items) {
				return nil, fmt.Errorf("index %d is out of range", typed)
			}
			current = items[typed]
		default:
			return nil, fmt.Errorf("unsupported path element %T", part)
		}
	}
	return current, nil
}
