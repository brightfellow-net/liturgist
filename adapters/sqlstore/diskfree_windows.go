// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package sqlstore

import "golang.org/x/sys/windows"

func diskUsage(dir string) (free, total int64, err error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, 0, err
	}
	var avail, all uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &all, nil); err != nil {
		return 0, 0, err
	}
	return int64(avail), int64(all), nil //nolint:gosec // sizes fit in int64
}
