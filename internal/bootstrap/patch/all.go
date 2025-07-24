package patch

import (
	"github.com/alist-org/alist/v3/internal/bootstrap/patch/v3_24_0"
	"github.com/alist-org/alist/v3/internal/bootstrap/patch/v3_32_0"
	"github.com/alist-org/alist/v3/internal/bootstrap/patch/v3_41_0"
)

type VersionPatches struct {
	// Version means if the system is upgraded from Version or an earlier one
	// to the current version, all patches in Patches will be executed.
	Version string
	Patches []func()
}

var UpgradePatches = []VersionPatches{
	{
		Version: "v3.24.0",
		Patches: []func(){
			v3_24_0.HashPwdForOldVersion,
		},
	},
	{
		Version: "v3.32.0",
		Patches: []func(){
			v3_32_0.UpdateAuthnForOldVersion,
		},
	},
	{
		Version: "v3.41.0",
		Patches: []func(){
			v3_41_0.GrantAdminPermissions,
		},
	},
}
