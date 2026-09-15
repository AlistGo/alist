package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alist-org/alist/v3/cmd/flags"
	"github.com/alist-org/alist/v3/internal/bootstrap/patch"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/pkg/utils"
)

var LastLaunchedVersion = ""

func safeCall(v string, i int, f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("patch %s[%d] panicked: %v", v, i, r)
		}
	}()
	if err := f(); err != nil {
		return fmt.Errorf("patch %s[%d]: %w", v, i, err)
	}
	return nil
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
	return patchNumA >= patchNumB
}

func InitUpgradePatch() error {
	release := strings.HasPrefix(conf.Version, "v")
	if release && LastLaunchedVersion == conf.Version {
		return nil
	}
	previous := LastLaunchedVersion
	if previous == "" {
		previous = "v0.0.0"
	}
	major, minor, patchNum, err := getVersion(previous)
	if err != nil {
		// Development builds can predate any migration. Replay idempotent
		// patches rather than marking an unknown database as already upgraded.
		utils.Log.Warnf("Cannot parse last launched version %q; checking all upgrade patches", previous)
		major, minor, patchNum = 0, 0, 0
	}
	for _, vp := range patch.UpgradePatches {
		ma, mi, pn, err := getVersion(vp.Version)
		if err != nil {
			return fmt.Errorf("invalid patch version %s: %w", vp.Version, err)
		}
		if !release || compareVersion(ma, mi, pn, major, minor, patchNum) {
			for i, p := range vp.Patches {
				if err := safeCall(vp.Version, i, p); err != nil {
					return err
				}
			}
		}
	}
	if release {
		return recordLaunchedVersion()
	}
	return nil
}

// Persist only the version field from the on-disk config. Runtime config may
// contain environment overrides and must not be written back here.
func recordLaunchedVersion() error {
	configPath := filepath.Join(flags.DataDir, "config.json")
	body, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("invalid config object in %s", configPath)
	}
	fields["last_launched_version"], err = json.Marshal(conf.Version)
	if err != nil {
		return err
	}
	body, err = json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	info, err := os.Stat(configPath)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(configPath), ".config-upgrade-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(info.Mode().Perm()); err == nil {
		_, err = tmp.Write(body)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tmp.Name(), configPath); err != nil {
		return err
	}
	conf.Conf.LastLaunchedVersion = conf.Version
	LastLaunchedVersion = conf.Version
	return nil
}
