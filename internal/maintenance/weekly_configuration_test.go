package maintenance

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func weeklyConfigurationFixture(t *testing.T) WeeklyConfiguration {
	t.Helper()
	dir := t.TempDir()
	c := WeeklyConfiguration{{Path: filepath.Join(dir, "settings.json"), Role: "settings", SHA256: strings.Repeat("a", 64)}, {Path: filepath.Join(dir, "managed.json"), Role: "managed_policy", Absent: true}}
	sort.Slice(c, func(i, j int) bool { return c[i].Path < c[j].Path })
	return c
}
func TestWeeklyConfigurationDeclarationAndRoleDigests(t *testing.T) {
	c := weeklyConfigurationFixture(t)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	settings, managed := c.Digest("settings"), c.Digest("managed_policy")
	if !digestPattern.MatchString(settings) || !digestPattern.MatchString(managed) || settings == managed {
		t.Fatal("role/absence declaration not bound")
	}
	d := append(WeeklyConfiguration(nil), c...)
	for i := range d {
		if d[i].Role == "managed_policy" {
			d[i].Path += ".other"
		}
	}
	sort.Slice(d, func(i, j int) bool { return d[i].Path < d[j].Path })
	if d.Digest("settings") != settings || d.Digest("managed_policy") == managed {
		t.Fatal("role digests do not bind exactly their ordered declared paths")
	}
	for _, tc := range []struct {
		name   string
		change func(WeeklyConfiguration) WeeklyConfiguration
	}{
		{"empty", func(c WeeklyConfiguration) WeeklyConfiguration { return nil }},
		{"missing role", func(c WeeklyConfiguration) WeeklyConfiguration { return c[:1] }},
		{"unsorted", func(c WeeklyConfiguration) WeeklyConfiguration { c[0], c[1] = c[1], c[0]; return c }},
		{"duplicate path", func(c WeeklyConfiguration) WeeklyConfiguration { c[1].Path = c[0].Path; return c }},
		{"relative path", func(c WeeklyConfiguration) WeeklyConfiguration { c[0].Path = "relative.json"; return c }},
		{"unknown role", func(c WeeklyConfiguration) WeeklyConfiguration { c[0].Role = "profile"; return c }},
		{"absence with hash", func(c WeeklyConfiguration) WeeklyConfiguration {
			c[0].Absent = true
			c[0].SHA256 = strings.Repeat("a", 64)
			return c
		}},
		{"present without hash", func(c WeeklyConfiguration) WeeklyConfiguration { c[0].Absent = false; c[0].SHA256 = ""; return c }},
		{"too many", func(c WeeklyConfiguration) WeeklyConfiguration {
			for len(c) < 33 {
				c = append(c, c[0])
			}
			return c
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := tc.change(append(WeeklyConfiguration(nil), c...))
			if bad.Validate() == nil || bad.Digest("settings") != "" {
				t.Fatal("invalid configuration became a digest")
			}
		})
	}
	if c.Digest("unknown") != "" {
		t.Fatal("unknown role accepted")
	}
}
