// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

// Package debug implements debug server for satellite and storage node.
package debug

import (
	"context"

	"cloud.google.com/go/profiler"
	"github.com/zeebo/errs"
	"go.uber.org/zap"
)

// InitProfilerError is error class for errors that occur during initialization
var InitProfilerError = errs.Class("initializing profiler")

// Profiler sets up continuous cpu/memory profiling for a storj peer
type Profiler struct {
	log         *zap.Logger
	PeerName    string
	PeerVersion string
}

// NewProfiler creates a new profiler for the current peer that will
// run continuous cpu/mem profiling
func NewProfiler(log *zap.Logger, peerName, version string) *Profiler {
	return &Profiler{log, peerName, version}
}

// Run starts the continuous profiler to collect cpu and mem info
func (p *Profiler) Run(ctx context.Context) error {
	if err := profiler.Start(profiler.Config{
		Service:        p.PeerName,
		ServiceVersion: p.PeerVersion,
		// TODO: remove debug logging when done testing
		DebugLogging: true,
	}); err != nil {
		return InitProfilerError.Wrap(err)
	}
	p.log.Debug("successful debug profiler init")
	return nil
}

// Close stops the profiler
func (p *Profiler) Close() error {
	return nil
}
