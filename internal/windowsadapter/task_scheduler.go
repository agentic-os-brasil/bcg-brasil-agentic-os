// Package windowsadapter describes the per-user Task Scheduler boundary. The
// renderer is testable everywhere; native creation stays unavailable until a
// qualified Windows probe exists.
package windowsadapter

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

var ErrNativeUnavailable = errors.New("Windows Task Scheduler native creation is not qualified")

type Spec struct{ Name, Program, Arguments, WorkingDirectory, StartBoundary string }
type Status struct {
	State string `json:"state"`
	Name  string `json:"name"`
}

func Render(spec Spec) ([]byte, error) {
	for _, value := range []string{spec.Name, spec.Program, spec.Arguments, spec.WorkingDirectory, spec.StartBoundary} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n") {
			return nil, errors.New("Windows task fields must be bounded")
		}
	}
	if strings.Contains(spec.Program, "&") || strings.Contains(spec.Arguments, "&") || strings.Contains(spec.WorkingDirectory, "&") {
		return nil, errors.New("Windows task values must not contain XML-sensitive interpolation")
	}
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"><RegistrationInfo><Author>BCGOS</Author><Description>Bounded presence wake; execution requires the runtime worker.</Description></RegistrationInfo><Triggers><CalendarTrigger><StartBoundary>%s</StartBoundary><ScheduleByDay><DaysInterval>1</DaysInterval></ScheduleByDay><Enabled>true</Enabled></CalendarTrigger></Triggers><Principals><Principal id="Author"><LogonType>InteractiveToken</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals><Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><StartWhenAvailable>true</StartWhenAvailable><ExecutionTimeLimit>PT2M</ExecutionTimeLimit></Settings><Actions Context="Author"><Exec><Command>%s</Command><Arguments>%s</Arguments><WorkingDirectory>%s</WorkingDirectory></Exec></Actions></Task>`, xmlEscape(spec.StartBoundary), xmlEscape(spec.Program), xmlEscape(spec.Arguments), xmlEscape(spec.WorkingDirectory))
	decoder := xml.NewDecoder(strings.NewReader(body))
	for {
		_, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return []byte(body), nil
}

func Install(spec Spec, _ bool) (Status, error) {
	return Status{State: "unavailable_native_qualification_pending", Name: spec.Name}, ErrNativeUnavailable
}
func ReadStatus(name string) Status {
	return Status{State: "unavailable_native_qualification_pending", Name: name}
}
func Pause(name string) (Status, error) {
	return Status{State: "unavailable_native_qualification_pending", Name: name}, ErrNativeUnavailable
}
func Resume(name string) (Status, error) {
	return Status{State: "unavailable_native_qualification_pending", Name: name}, ErrNativeUnavailable
}
func Uninstall(name string) error { return ErrNativeUnavailable }

func xmlEscape(value string) string {
	var builder strings.Builder
	_ = xml.EscapeText(&builder, []byte(value))
	return builder.String()
}
