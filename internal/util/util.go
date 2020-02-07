// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package util

import (
	"strings"
)

// Or returns primary if non-empty and backup otherwise
func Or(primary, backup string) string {
	primary = strings.TrimSpace(primary)
	if primary == "" {
		return strings.TrimSpace(backup)
	}
	return primary
}
