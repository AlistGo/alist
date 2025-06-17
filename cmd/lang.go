/*
Package cmd
Copyright © 2022 Noah Hsu<i@nn.ci>
*/
package cmd

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	_ "github.com/alist-org/alist/v3/drivers"
	"github.com/alist-org/alist/v3/internal/bootstrap"
	"github.com/alist-org/alist/v3/internal/bootstrap/data"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type KV[V any] map[string]V

type Drivers KV[KV[interface{}]]

func firstUpper(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func convert(s string) string {
	ss := strings.Split(s, "_")
	ans := strings.Join(ss, " ")
	return firstUpper(ans)
}

func writeFile(name string, data interface{}) {
	f, err := os.Open(fmt.Sprintf("../alist-web/src/lang/en/%s.json", name))
	if err != nil {
		log.Errorf("failed to open %s.json: %+v", name, err)
		return
	}
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
		log.Errorf("failed to read %s.json: %+v", name, err)
		return
	}
	oldData := make(map[string]interface{})
	newData := make(map[string]interface{})
	err = utils.Json.Unmarshal(content, &oldData)
	if err != nil {
		log.Errorf("failed to unmarshal %s.json: %+v", name, err)
		return
	}
	content, err = utils.Json.Marshal(data)
	if err != nil {
		log.Errorf("failed to marshal json: %+v", err)
		return
	}
	err = utils.Json.Unmarshal(content, &newData)
	if err != nil {
		log.Errorf("failed to unmarshal json: %+v", err)
		return
	}
	if reflect.DeepEqual(oldData, newData) {
		log.Infof("%s.json no changed, skip", name)
	} else {
		log.Infof("%s.json changed, update file", name)
		//log.Infof("old: %+v\nnew:%+v", oldData, data)
		utils.WriteJsonToFile(fmt.Sprintf("lang/%s.json", name), newData, true)
	}
}

func generateDriversJson() {
	drivers := make(Drivers)
	drivers["drivers"] = make(KV[interface{}])
	drivers["config"] = make(KV[interface{}])
	driverInfoMap := op.GetDriverInfoMap()
	for k, v := range driverInfoMap {
		drivers["drivers"][k] = convert(k)
		items := make(KV[interface{}])
		config := map[string]string{}
		if v.Config.Alert != "" {
			alert := strings.SplitN(v.Config.Alert, "|", 2)
			if len(alert) > 1 {
				config["alert"] = alert[1]
			}
		}
		drivers["config"][k] = config
		for i := range v.Additional {
			item := v.Additional[i]
			items[item.Name] = convert(item.Name)
			if item.Help != "" {
				items[fmt.Sprintf("%s-tips", item.Name)] = item.Help
			}
			if item.Type == conf.TypeSelect && len(item.Options) > 0 {
				options := make(KV[string])
				_options := strings.Split(item.Options, ",")
				for _, o := range _options {
					options[o] = convert(o)
				}
				items[fmt.Sprintf("%ss", item.Name)] = options
			}
		}
		drivers[k] = items
	}
	writeFile("drivers", drivers)
}

func generateSettingsJson() {
	settings := data.InitialSettings()
	settingsLang := make(KV[any])
	for _, setting := range settings {
		settingsLang[setting.Key] = convert(setting.Key)
		if setting.Help != "" {
			settingsLang[fmt.Sprintf("%s-tips", setting.Key)] = setting.Help
		}
		if setting.Type == conf.TypeSelect && len(setting.Options) > 0 {
			options := make(KV[string])
			_options := strings.Split(setting.Options, ",")
			for _, o := range _options {
				options[o] = convert(o)
			}
			settingsLang[fmt.Sprintf("%ss", setting.Key)] = options
		}
	}
	writeFile("settings", settingsLang)
	//utils.WriteJsonToFile("lang/settings.json", settingsLang)
}

// LangCmd represents the lang command
var LangCmd = &cobra.Command{
	Use:   "lang",
	Short: "Generate language json file",
	Run: func(cmd *cobra.Command, args []string) {
		bootstrap.InitConfig()
		err := os.MkdirAll("lang", 0777)
		if err != nil {
			utils.Log.Fatalf("failed create folder: %s", err.Error())
		}
		generateDriversJson()
		generateSettingsJson()
	},
}

func init() {
	RootCmd.AddCommand(LangCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// langCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// langCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
