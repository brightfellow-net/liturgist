// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package sqlstore

import "golang.org/x/sys/windows"

func freeBytes(dir string) (int64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var avail uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, nil, nil); err != nil {
		return 0, err
	}
	return int64(avail), nil //nolint:gosec // free space fits in int64
}
