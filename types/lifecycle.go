// Package types provides shared type definitions.
package types

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"
)

// StartStopper is a long-running component that can be started and stopped.
// Start must be non-blocking. Launch background work in a goroutine and return once ready.
type StartStopper interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// CloseOnly adapts a resource that is opened during application construction to
// a lifecycle-managed resource. It has no startup work and closes when stopped.
type CloseOnly func() error

// Start implements StartStopper.
func (CloseOnly) Start(context.Context) error {
	return nil
}

// Stop implements StartStopper.
func (closer CloseOnly) Stop(context.Context) error {
	return closer()
}

// Manager manages the lifecycle of multiple services.
// Services are started in registration order and stopped in reverse order.
type Manager struct {
	services []serviceEntry
	logger   *slog.Logger
}

type serviceEntry struct {
	name    string
	service StartStopper
}

// NewManager creates a new lifecycle manager.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{logger: logger}
}

// Add registers a service with the given name.
// Services are started in the order they are added.
func (m *Manager) Add(name string, svc StartStopper) {
	m.services = append(m.services, serviceEntry{name: name, service: svc})
}

// StartAll starts all registered services in order.
// It returns on the first error encountered.
func (m *Manager) StartAll(ctx context.Context) error {
	for _, entry := range m.services {
		m.logger.InfoContext(ctx, "starting service", "service", entry.name)
		if err := entry.service.Start(ctx); err != nil {
			return fmt.Errorf("start %s: %w", entry.name, err)
		}
	}
	return nil
}

// StopAll stops all registered services in reverse order with the given timeout.
// It attempts to stop all services and returns a combined error of any failures.
func (m *Manager) StopAll(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var errs []error
	for _, entry := range slices.Backward(m.services) {
		m.logger.Info("stopping service", "service", entry.name)
		if err := entry.service.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("stop %s: %w", entry.name, err))
		}
	}

	if err := errors.Join(errs...); err != nil {
		m.logger.Error("errors during shutdown", "error", err)
	}
}
