// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/danielgtaylor/huma/v2"
)

// SystemDeps are what the system operations need. They are registered only
// for the community edition (14 §5, H-8).
type SystemDeps struct {
	System *app.System
	Log    *slog.Logger
}

// SystemStatusView is the church admin's system page (14 §5).
type SystemStatusView struct {
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Database struct {
		Driver        string `json:"driver" enum:"sqlite,postgres"`
		SizeBytes     int64  `json:"size_bytes"`
		SchemaVersion int64  `json:"schema_version"`
	} `json:"database"`
	Disk struct {
		FreeBytes  int64 `json:"free_bytes"`
		TotalBytes int64 `json:"total_bytes"`
		Low        bool  `json:"low" doc:"under 1 GiB or 5 % free, whichever is larger"`
	} `json:"disk"`
	Backup struct {
		Supported    bool       `json:"supported" doc:"false with PostgreSQL: use pg_dump"`
		Scheduled    bool       `json:"scheduled" doc:"automatic backups are on"`
		LastAt       *time.Time `json:"last_at" doc:"the newest automatic or manual backup; null when there is none"`
		LastKind     string     `json:"last_kind" enum:"auto,manual," doc:"empty when there is none"`
		LastCopiedAt *time.Time `json:"last_copied_at" doc:"the last download, or backup written outside the data folder"`
		Warning      string     `json:"warning" enum:"stale,not_copied," doc:"stale: no backup in 48 hours; not_copied: no copy taken away in 30 days"`
	} `json:"backup"`
	EmailConfigured bool `json:"email_configured"`
	HTTPS           struct {
		Mode                string `json:"mode" enum:"plain_http,behind_proxy"`
		PlainHTTPWarning    bool   `json:"plain_http_warning" doc:"reachable on the network over plain HTTP"`
		ProxyMissingWarning bool   `json:"proxy_missing_warning" doc:"BASE_URL says https but no trusted proxy is configured"`
	} `json:"https"`
	Update struct {
		Enabled   bool   `json:"enabled"`
		Latest    string `json:"latest" doc:"newest release tag seen; empty before the first check"`
		Available bool   `json:"available"`
	} `json:"update"`
}

func systemStatusView(f app.SystemFacts) SystemStatusView {
	var v SystemStatusView
	v.Version, v.Commit = f.Version, f.Commit
	v.Database.Driver, v.Database.SizeBytes, v.Database.SchemaVersion = f.Database.Driver, f.Database.SizeBytes, f.Database.SchemaVersion
	v.Disk.FreeBytes, v.Disk.TotalBytes, v.Disk.Low = f.Disk.FreeBytes, f.Disk.TotalBytes, f.Disk.Low
	v.Backup.Supported, v.Backup.Scheduled = f.Backup.Supported, f.Backup.Scheduled
	v.Backup.LastAt, v.Backup.LastKind, v.Backup.LastCopiedAt, v.Backup.Warning = f.Backup.LastAt, f.Backup.LastKind, f.Backup.LastCopiedAt, f.Backup.Warning
	v.EmailConfigured = f.EmailConfigured
	v.HTTPS.Mode, v.HTTPS.PlainHTTPWarning, v.HTTPS.ProxyMissingWarning = f.HTTPS.Mode, f.HTTPS.PlainHTTPWarning, f.HTTPS.ProxyMissingWarning
	v.Update.Enabled, v.Update.Latest, v.Update.Available = f.Update.Enabled, f.Update.Latest, f.Update.Available
	return v
}

// RegisterSystem registers the system page operations (14 §5).
func RegisterSystem(api huma.API, d SystemDeps) {
	fail := func(ctx context.Context, err error) error { return MapError(ctx, err, d.Log) }
	sess := func(ctx context.Context) *domain.Session { return RequestInfoFrom(ctx).Session }
	sop := func(id, path, summary string) huma.Operation {
		return tagged(op(id, http.MethodGet, path, TenancyChurch, http.StatusOK, summary), "system")
	}

	huma.Register(api, sop("getSystemStatus", "/system/status", "The system page: version, disk, backups, HTTPS (church.settings)"),
		func(ctx context.Context, _ *struct{}) (*struct{ Body SystemStatusView }, error) {
			f, err := d.System.Status(ctx, sess(ctx))
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &struct{ Body SystemStatusView }{Body: systemStatusView(f)}, nil
		})

	huma.Register(api, sop("downloadBackup", "/system/backup", "Download a backup archive (church.settings)"),
		func(ctx context.Context, _ *struct{}) (*huma.StreamResponse, error) {
			stream, err := d.System.OpenBackup(ctx, sess(ctx))
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &huma.StreamResponse{Body: func(hctx huma.Context) {
				defer func() { _ = stream.Close() }()
				hctx.SetHeader("Content-Type", "application/zip")
				hctx.SetHeader("Content-Disposition", `attachment; filename="`+stream.Filename()+`"`)
				hctx.SetHeader("Cache-Control", "no-store")
				hctx.SetStatus(http.StatusOK)
				_, _ = stream.WriteTo(hctx.BodyWriter())
			}}, nil
		})
}
