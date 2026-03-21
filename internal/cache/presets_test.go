package cache_test

import (
	"testing"
	"time"

	"github.com/sofq/jira-cli/internal/cache"
)

func TestMetadataTTL(t *testing.T) {
	const wantTTL = 10 * time.Minute

	t.Run("known metadata paths return 10 minutes", func(t *testing.T) {
		knownPaths := []string{
			"/rest/api/3/project",
			"/rest/api/3/status",
			"/rest/api/3/issuetype",
			"/rest/api/3/priority",
			"/rest/api/3/resolution",
			"/rest/api/3/issueLinkType",
		}
		for _, path := range knownPaths {
			t.Run(path, func(t *testing.T) {
				got := cache.MetadataTTL(path)
				if got != wantTTL {
					t.Errorf("MetadataTTL(%q) = %v, want %v", path, got, wantTTL)
				}
			})
		}
	})

	t.Run("unknown paths return 0", func(t *testing.T) {
		unknownPaths := []string{
			"/rest/api/3/issue/PROJ-1",
			"/rest/api/3/search",
			"/rest/api/3/myself",
			"/rest/api/3/field",
			"",
			"/rest/api/2/project",
		}
		for _, path := range unknownPaths {
			t.Run(path, func(t *testing.T) {
				got := cache.MetadataTTL(path)
				if got != 0 {
					t.Errorf("MetadataTTL(%q) = %v, want 0 (no auto-cache)", path, got)
				}
			})
		}
	})

	t.Run("sub-paths return 0 (exact match only)", func(t *testing.T) {
		subPaths := []string{
			"/rest/api/3/project/PROJ",
			"/rest/api/3/project/PROJ/statuses",
			"/rest/api/3/status/10001",
			"/rest/api/3/issuetype/10002",
			"/rest/api/3/priority/1",
			"/rest/api/3/resolution/done",
			"/rest/api/3/issueLinkType/10000",
		}
		for _, path := range subPaths {
			t.Run(path, func(t *testing.T) {
				got := cache.MetadataTTL(path)
				if got != 0 {
					t.Errorf("MetadataTTL(%q) = %v, want 0 (sub-paths not auto-cached)", path, got)
				}
			})
		}
	})
}
