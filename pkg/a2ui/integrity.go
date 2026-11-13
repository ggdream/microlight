package a2ui

import "fmt"

// IntegrityError reports an operation that targets an unknown surface.
type IntegrityError struct {
	SurfaceID string
}

func (e *IntegrityError) Error() string {
	return fmt.Sprintf("unknown surface %q", e.SurfaceID)
}

func (e *IntegrityError) RendererMessage() RendererMessage {
	return RendererMessage{Version: Version, Error: &RendererError{
		Code: "INTEGRITY_ERROR", SurfaceID: &e.SurfaceID, Message: e.Error(),
	}}
}

// ViewDiagnostic describes a component that the renderer could not mount.
type ViewDiagnostic struct {
	Code      string
	SurfaceID string
	Component string
}

func (e *ViewDiagnostic) Error() string {
	return fmt.Sprintf("%s: component %q", e.Code, e.Component)
}

func (e *ViewDiagnostic) RendererMessage() RendererMessage {
	return RendererMessage{Version: Version, Error: &RendererError{
		Code: e.Code, SurfaceID: &e.SurfaceID, Message: e.Error(),
	}}
}
