/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package version

// Version is set at build-time.
var Version string

func GetVersion() string {
	if Version == "" {
		return "UNKNOWN"
	}
	return Version
}
