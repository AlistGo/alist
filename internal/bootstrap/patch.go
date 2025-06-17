package bootstrap

import (
	"fmt"

	"github.com/alist-org/alist/v3/internal/bootstrap/patch"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/pkg/utils"
	"strings"
)

var LastLaunchedVersion = ""

func safeCall(v string, i int, f func()) {
	defer func() {
		if r := recover(); r != nil {
			utils.Log.Errorf("Recovered from patch (version: %s, index: %d) panic: %v", v, i, r)
		}
	}()

	f()
}

func getVersion(v string) (major, minor, patchNum int, err error) {
	_, err = fmt.Sscanf(v, "v%d.%d.%d", &major, &minor, &patchNum)
	return major, minor, patchNum, err
}

func compareVersion(majorA, minorA, patchNumA, majorB, minorB, patchNumB int) bool {
	if majorA != majorB {
		return majorA > majorB
	}
	if minorA != minorB {
		return minorA > minorB
	}
	if patchNumA != patchNumB {
		return patchNumA > patchNumB
	}
	return true
}

func InitUpgradePatch() {
	if !strings.HasPrefix(conf.Version, "v") {
		for _, vp := range patch.UpgradePatches {
			for i, p := range vp.Patches {
				safeCall(vp.Version, i, p)
			}
		}
		return
	}
	if LastLaunchedVersion == conf.Version {
		return
	}
	if LastLaunchedVersion == "" {
		LastLaunchedVersion = "v0.0.0"
	}
	major, minor, patchNum, err := getVersion(LastLaunchedVersion)
	if err != nil {
		utils.Log.Warnf("Failed to parse last launched version %s: %v, skipping all patches and rewrite last launched version", LastLaunchedVersion, err)
		return
	}
	for _, vp := range patch.UpgradePatches {
		ma, mi, pn, err := getVersion(vp.Version)
		if err != nil {
			utils.Log.Errorf("Skip invalid version %s patches: %v", vp.Version, err)
			continue
		}
		if compareVersion(ma, mi, pn, major, minor, patchNum) {
			for i, p := range vp.Patches {
				safeCall(vp.Version, i, p)
			}
		}
	}
}
