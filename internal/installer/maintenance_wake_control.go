package installer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/windowsadapter"
)

var ErrMaintenanceWakeDisabled = errors.New("owned native wake is disabled")
var ErrMaintenanceWakeControlChanged = errors.New("native wake control revision changed")

// MaintenanceWakeControl is an installer-owned OS eligibility veto, not a job
// grant. A zero revision exists only in an attended missing-control review.
type MaintenanceWakeControl struct {
	SchemaVersion int    `json:"schema_version"`
	BindingSHA256 string `json:"binding_sha256"`
	Revision      uint64 `json:"revision"`
	Enabled       bool   `json:"enabled"`
}

func (b MaintenanceWakeBinding) windowsWakeControlPath() (string, string, error) {
	digest, err := b.Digest()
	if err != nil {
		return "", "", err
	}
	parent := filepath.Join(b.UserHome, ".maestro-install")
	if err := scheduler.ValidatePrivateDirectory(parent); err != nil {
		// A missing installation root is not a fresh native definition.
		return "", "", fmt.Errorf("existing private installer state required: %v", err)
	}
	directory := filepath.Join(parent, "native-wake")
	if err := scheduler.ValidatePrivateDirectory(directory); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	return filepath.Join(directory, digest+".json"), digest, nil
}
func decodeWindowsWakeControl(body []byte, digest string) (MaintenanceWakeControl, error) {
	var out MaintenanceWakeControl
	var fields map[string]json.RawMessage
	if len(body) > 4096 {
		return out, errors.New("bounded complete native wake control required")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return out, errors.New("native wake control object required")
	}
	fields = map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return out, err
		}
		key, ok := token.(string)
		if !ok {
			return out, errors.New("native wake control key required")
		}
		if _, duplicate := fields[key]; duplicate {
			return out, errors.New("duplicate native wake control field")
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return out, err
		}
		fields[key] = raw
	}
	last, err := decoder.Token()
	if err != nil || last != json.Delim('}') || decoder.Decode(new(any)) != io.EOF || len(fields) != 4 {
		return out, errors.New("complete native wake control required")
	}
	for _, key := range []string{"schema_version", "binding_sha256", "revision", "enabled"} {
		raw, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return out, errors.New("complete typed native wake control required")
		}
	}
	if releasecontract.DecodeStrictJSON(body, &out) != nil || out.SchemaVersion != 1 || out.BindingSHA256 != digest || out.Revision == 0 {
		return MaintenanceWakeControl{}, errors.New("native wake control identity or schema differs")
	}
	return out, nil
}
func (b MaintenanceWakeBinding) ReadWindowsWakeControl() (MaintenanceWakeControl, error) {
	path, digest, err := b.windowsWakeControlPath()
	if err != nil {
		return MaintenanceWakeControl{}, err
	}
	file, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return MaintenanceWakeControl{}, err
	}
	defer file.Close()
	if err := scheduler.ValidatePrivateFile(file, 0600); err != nil {
		return MaintenanceWakeControl{}, err
	}
	before, err := file.Stat()
	if err != nil {
		return MaintenanceWakeControl{}, err
	}
	body, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return MaintenanceWakeControl{}, err
	}
	after, err := file.Stat()
	if err != nil {
		return MaintenanceWakeControl{}, err
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		return MaintenanceWakeControl{}, errors.New("native wake control changed while reading")
	}
	return decodeWindowsWakeControl(body, digest)
}
func (b MaintenanceWakeBinding) RequireWindowsWakeControl(ctx context.Context) (MaintenanceWakeControl, error) {
	if err := ctx.Err(); err != nil {
		return MaintenanceWakeControl{}, err
	}
	control, err := b.ReadWindowsWakeControl()
	if err != nil {
		return control, err
	}
	if !control.Enabled {
		return control, ErrMaintenanceWakeDisabled
	}
	return control, ctx.Err()
}

// WriteWindowsWakeControl is called only within the original installation
// publication guard. Existing private-file locking/CAS/ACL/no-follow publication
// owns the narrow definition revision; it creates no scheduler/job authority.
func (b MaintenanceWakeBinding) WriteWindowsWakeControl(ctx context.Context, expected uint64, enabled bool) (MaintenanceWakeControl, error) {
	var result MaintenanceWakeControl
	if err := ctx.Err(); err != nil {
		return result, err
	}
	path, digest, err := b.windowsWakeControlPath()
	if err != nil {
		return result, err
	}
	current, err := b.ReadWindowsWakeControl()
	if errors.Is(err, os.ErrNotExist) {
		current = MaintenanceWakeControl{SchemaVersion: 1, BindingSHA256: digest}
	} else if err != nil {
		return result, err
	}
	if current.Revision != expected || expected == math.MaxUint64 {
		return result, ErrMaintenanceWakeControlChanged
	}
	if err := scheduler.EnsurePrivateDirectory(filepath.Dir(path)); err != nil {
		return result, err
	}
	if err := scheduler.ValidatePrivateDirectory(filepath.Dir(path)); err != nil {
		return result, err
	}
	err = scheduler.UpdatePrivateFileGuarded(path, 4096, func(publish func() error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return publish()
	}, func(body []byte) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		actual := MaintenanceWakeControl{SchemaVersion: 1, BindingSHA256: digest}
		if len(body) > 0 {
			var err error
			actual, err = decodeWindowsWakeControl(body, digest)
			if err != nil {
				return nil, err
			}
		}
		if actual.Revision != expected {
			return nil, ErrMaintenanceWakeControlChanged
		}
		result = MaintenanceWakeControl{SchemaVersion: 1, BindingSHA256: digest, Revision: expected + 1, Enabled: enabled}
		return json.Marshal(result)
	})
	if err != nil {
		return MaintenanceWakeControl{}, err
	}
	observed, err := b.ReadWindowsWakeControl()
	if err != nil || observed != result {
		return MaintenanceWakeControl{}, errors.New("native wake control publication unavailable")
	}
	return result, nil
}

// VerifyWindowsNativeWake is also used by the original worker publication veto.
// A late OS acknowledgement cannot open or bypass its independent private file.
func (b MaintenanceWakeBinding) VerifyWindowsNativeWake(ctx context.Context) error {
	before, err := b.RequireWindowsWakeControl(ctx)
	if err != nil {
		return err
	}
	spec, err := b.WindowsSpec()
	if err != nil {
		return err
	}
	status, err := windowsadapter.NativeWake(ctx, spec, "inspect")
	if err != nil {
		return err
	}
	if !status.Present || !status.Enabled {
		return ErrMaintenanceWakeDisabled
	}
	after, err := b.RequireWindowsWakeControl(ctx)
	if err != nil {
		return err
	}
	if before != after {
		return ErrMaintenanceWakeControlChanged
	}
	return nil
}
