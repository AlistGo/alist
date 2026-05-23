package conf

import (
	"net/url"
	"regexp"
	"sync/atomic"
)

var (
	BuiltAt    string
	GitAuthor  string
	GitCommit  string
	Version    string = "dev"
	WebVersion string
)

var (
	Conf *Config
	URL  *url.URL
)

var SlicesMap = make(map[string][]string)
var FilenameCharMap = make(map[string]string)
var PrivacyReg []*regexp.Regexp

var (
	// StoragesLoaded loaded success if empty
	StoragesLoaded atomic.Bool
)
var (
	RawIndexHtml string
	ManageHtml   string
	IndexHtml    string
)
