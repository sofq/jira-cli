package cache

import "time"

// MetadataTTL returns a default cache TTL for metadata API paths.
// Returns 0 for non-metadata paths (no auto-caching).
// Only exact path matches are considered — sub-paths such as
// /rest/api/3/project/PROJ are not auto-cached.
func MetadataTTL(path string) time.Duration {
	// These endpoints return low-churn metadata safe to cache by default.
	metadataPaths := []string{
		"/rest/api/3/project",
		"/rest/api/3/status",
		"/rest/api/3/issuetype",
		"/rest/api/3/priority",
		"/rest/api/3/resolution",
		"/rest/api/3/issueLinkType",
	}
	for _, p := range metadataPaths {
		if path == p {
			return 10 * time.Minute
		}
	}
	return 0
}
