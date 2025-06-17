package lanzou

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	log "github.com/sirupsen/logrus"
)

const DAY time.Duration = 84600000000000

// 解析时间
var timeSplitReg = regexp.MustCompile("([0-9.]*)\\s*([\u4e00-\u9fa5]+)")

// 如果解析失败,则返回当前时间
func MustParseTime(str string) time.Time {
	lastOpTime, err := time.ParseInLocation("2006-01-02 -07", str+" +08", time.Local)
	if err != nil {
		strs := timeSplitReg.FindStringSubmatch(str)
		lastOpTime = time.Now()
		if len(strs) == 3 {
			i, _ := strconv.ParseInt(strs[1], 10, 64)
			ti := time.Duration(-i)
			switch strs[2] {
			case "秒前":
				lastOpTime = lastOpTime.Add(time.Second * ti)
			case "分钟前":
				lastOpTime = lastOpTime.Add(time.Minute * ti)
			case "小时前":
				lastOpTime = lastOpTime.Add(time.Hour * ti)
			case "天前":
				lastOpTime = lastOpTime.Add(DAY * ti)
			case "昨天":
				lastOpTime = lastOpTime.Add(-DAY)
			case "前天":
				lastOpTime = lastOpTime.Add(-DAY * 2)
			}
		}
	}
	return lastOpTime
}

// 解析大小
var sizeSplitReg = regexp.MustCompile(`(?i)([0-9.]+)\s*([bkm]+)`)

// 解析失败返回0
func SizeStrToInt64(size string) int64 {
	strs := sizeSplitReg.FindStringSubmatch(size)
	if len(strs) < 3 {
		return 0
	}

	s, _ := strconv.ParseFloat(strs[1], 64)
	switch strings.ToUpper(strs[2]) {
	case "B":
		return int64(s)
	case "K":
		return int64(s * (1 << 10))
	case "M":
		return int64(s * (1 << 20))
	}
	return 0
}

// 移除注释
func RemoveNotes(html string) string {
	return regexp.MustCompile(`<!--.*?-->|[^:]//.*|/\*.*?\*/`).ReplaceAllStringFunc(html, func(b string) string {
		if b[1:3] == "//" {
			return b[:1]
		}
		return "\n"
	})
}

// 清理JS注释
func RemoveJSComment(data string) string {
	var result strings.Builder
	inComment := false
	inSingleLineComment := false

	for i := 0; i < len(data); i++ {
		v := data[i]

		if inSingleLineComment && (v == '\n' || v == '\r') {
			inSingleLineComment = false
			result.WriteByte(v)
			continue
		}
		if inComment && v == '*' && i+1 < len(data) && data[i+1] == '/' {
			inComment = false
			continue
		}
		if v == '/' && i+1 < len(data) {
			nextChar := data[i+1]
			if nextChar == '*' {
				inComment = true
				i++
				continue
			} else if nextChar == '/' {
				inSingleLineComment = true
				i++
				continue
			}
		}
		result.WriteByte(v)
	}

	return result.String()
}

var findAcwScV2Reg = regexp.MustCompile(`arg1='([0-9A-Z]+)'`)

// 在页面被过多访问或其他情况下，有时候会先返回一个加密的页面，其执行计算出一个acw_sc__v2后放入页面后再重新访问页面才能获得正常页面
// 若该页面进行了js加密，则进行解密，计算acw_sc__v2，并加入cookie
func CalcAcwScV2(html string) (string, error) {
	log.Debugln("acw_sc__v2", html)
	acwScV2s := findAcwScV2Reg.FindStringSubmatch(html)
	if len(acwScV2s) != 2 {
		return "", fmt.Errorf("无法匹配acw_sc__v2")
	}
	return HexXor(Unbox(acwScV2s[1]), "3000176000856006061501533003690027800375"), nil
}

func Unbox(hex string) string {
	var box = []int{6, 28, 34, 31, 33, 18, 30, 23, 9, 8, 19, 38, 17, 24, 0, 5, 32, 21, 10, 22, 25, 14, 15, 3, 16, 27, 13, 35, 2, 29, 11, 26, 4, 36, 1, 39, 37, 7, 20, 12}
	var newBox = make([]byte, len(hex))
	for i := 0; i < len(box); i++ {
		j := box[i]
		if len(newBox) > j {
			newBox[j] = hex[i]
		}
	}
	return string(newBox)
}

func HexXor(hex1, hex2 string) string {
	out := bytes.NewBuffer(make([]byte, len(hex1)))
	for i := 0; i < len(hex1) && i < len(hex2); i += 2 {
		v1, _ := strconv.ParseInt(hex1[i:i+2], 16, 64)
		v2, _ := strconv.ParseInt(hex2[i:i+2], 16, 64)
		out.WriteString(strconv.FormatInt(v1^v2, 16))
	}
	return out.String()
}

var findDataReg = regexp.MustCompile(`data[:\s]+({[^}]+})`)    // 查找json
var findKVReg = regexp.MustCompile(`'(.+?)':('?([^' },]*)'?)`) // 拆分kv

// 根据key查询js变量
func findJSVarFunc(key, data string) string {
	var values []string
	if key != "sasign" {
		values = regexp.MustCompile(`var ` + key + `\s*=\s*['"]?(.+?)['"]?;`).FindStringSubmatch(data)
	} else {
		matches := regexp.MustCompile(`var `+key+`\s*=\s*['"]?(.+?)['"]?;`).FindAllStringSubmatch(data, -1)
		if len(matches) == 3 {
			values = matches[1]
		} else {
			if len(matches) > 0 {
				values = matches[0]
			}
		}
	}
	if len(values) == 0 {
		return ""
	}
	return values[1]
}

var findFunction = regexp.MustCompile(`(?ims)^function[^{]+`)
var findFunctionAll = regexp.MustCompile(`(?is)function[^{]+`)

// 查找所有方法位置
func findJSFunctionIndex(data string, all bool) [][2]int {
	findFunction := findFunction
	if all {
		findFunction = findFunctionAll
	}

	indexs := findFunction.FindAllStringIndex(data, -1)
	fIndexs := make([][2]int, 0, len(indexs))

	for _, index := range indexs {
		if len(index) != 2 {
			continue
		}
		count, data := 0, data[index[1]:]
		for ii, v := range data {
			if v == ' ' && count == 0 {
				continue
			}
			if v == '{' {
				count++
			}

			if v == '}' {
				count--
			}
			if count == 0 {
				fIndexs = append(fIndexs, [2]int{index[0], index[1] + ii + 1})
				break
			}
		}
	}
	return fIndexs
}

// 删除JS全局方法
func removeJSGlobalFunction(html string) string {
	indexs := findJSFunctionIndex(html, false)
	block := make([]string, len(indexs))
	for i, next := len(indexs)-1, len(html); i >= 0; i-- {
		index := indexs[i]
		block[i] = html[index[1]:next]
		next = index[0]
	}
	return strings.Join(block, "")
}

// 根据名称获取方法
func getJSFunctionByName(html string, name string) (string, error) {
	indexs := findJSFunctionIndex(html, true)
	for _, index := range indexs {
		data := html[index[0]:index[1]]
		if regexp.MustCompile(`function\s+` + name + `[()\s]+{`).MatchString(data) {
			return data, nil
		}
	}
	return "", fmt.Errorf("not find %s function", name)
}

// 解析html中的JSON,选择最长的数据
func htmlJsonToMap2(html string) (map[string]string, error) {
	datas := findDataReg.FindAllStringSubmatch(html, -1)
	var sData string
	for _, data := range datas {
		if len(datas) > 0 && len(data[1]) > len(sData) {
			sData = data[1]
		}
	}
	if sData == "" {
		return nil, fmt.Errorf("not find data")
	}
	return jsonToMap(sData, html), nil
}

// 解析html中的JSON
func htmlJsonToMap(html string) (map[string]string, error) {
	datas := findDataReg.FindStringSubmatch(html)
	if len(datas) != 2 {
		return nil, fmt.Errorf("not find data")
	}
	return jsonToMap(datas[1], html), nil
}

func jsonToMap(data, html string) map[string]string {
	var param = make(map[string]string)
	kvs := findKVReg.FindAllStringSubmatch(data, -1)
	for _, kv := range kvs {
		k, v := kv[1], kv[3]
		if v == "" || strings.Contains(kv[2], "'") || IsNumber(kv[2]) {
			param[k] = v
		} else {
			param[k] = findJSVarFunc(v, html)
		}
	}
	return param
}

func IsNumber(str string) bool {
	for _, s := range str {
		if !unicode.IsDigit(s) {
			return false
		}
	}
	return true
}

var findFromReg = regexp.MustCompile(`data : '(.+?)'`) // 查找from字符串

// 解析html中的form
func htmlFormToMap(html string) (map[string]string, error) {
	forms := findFromReg.FindStringSubmatch(html)
	if len(forms) != 2 {
		return nil, fmt.Errorf("not find file sgin")
	}
	return formToMap(forms[1]), nil
}

func formToMap(from string) map[string]string {
	var param = make(map[string]string)
	for _, kv := range strings.Split(from, "&") {
		kv := strings.SplitN(kv, "=", 2)[:2]
		param[kv[0]] = kv[1]
	}
	return param
}

var regExpirationTime = regexp.MustCompile(`e=(\d+)`)

func GetExpirationTime(url string) (etime time.Duration) {
	exps := regExpirationTime.FindStringSubmatch(url)
	if len(exps) < 2 {
		return
	}
	timestamp, err := strconv.ParseInt(exps[1], 10, 64)
	if err != nil {
		return
	}
	etime = time.Duration(timestamp-time.Now().Unix()) * time.Second
	return
}

func CookieToString(cookies []*http.Cookie) string {
	if cookies == nil {
		return ""
	}
	cookieStrings := make([]string, len(cookies))
	for i, cookie := range cookies {
		cookieStrings[i] = cookie.Name + "=" + cookie.Value
	}
	return strings.Join(cookieStrings, ";")
}
