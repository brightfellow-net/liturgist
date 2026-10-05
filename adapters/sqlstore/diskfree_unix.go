// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package sqlstore

import "golang.org/x/sys/unix"

func diskUsage(dir string) (free, total int64, err error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, 0, err
	}
	free = int64(st.Bavail) * int64(st.Bsize)  //nolint:gosec,unconvert // field types differ per platform
	total = int64(st.Blocks) * int64(st.Bsize) //nolint:gosec,unconvert // field types differ per platform
	return free, total, nil
}
