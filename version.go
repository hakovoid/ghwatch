package main

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"
)

// version peut être imposée à la compilation (-ldflags "-X main.version=v1.2.3") ;
// sinon elle vient du tag git, que go build inscrit dans le binaire.
var version = ""

// buildVersion renvoie la version du binaire : « v0.3.0 » sur un tag,
// « v0.3.1-0.20261025…-abcdef » entre deux tags, « dev » hors git.
func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

// versionInfo : version, commit et date de compilation, pour -version.
func versionInfo() string {
	s := "ghwatch " + buildVersion()
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return s
	}
	var rev, at string
	dirty := false
	for _, kv := range bi.Settings {
		switch kv.Key {
		case "vcs.revision":
			rev = kv.Value[:min(len(kv.Value), 7)]
		case "vcs.time":
			if t, err := time.Parse(time.RFC3339, kv.Value); err == nil {
				at = t.Local().Format("02/01/2006 15:04")
			}
		case "vcs.modified":
			dirty = kv.Value == "true"
		}
	}
	var extra []string
	if rev != "" {
		if dirty {
			rev += ", modifié localement"
		}
		extra = append(extra, "commit "+rev)
	}
	if at != "" {
		extra = append(extra, at)
	}
	extra = append(extra, bi.GoVersion)
	return fmt.Sprintf("%s (%s)", s, strings.Join(extra, " · "))
}
