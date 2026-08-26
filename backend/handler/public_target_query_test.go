package handler

import "testing"

func TestWildcardPatternCandidates(t *testing.T) {
	cases := []struct {
		domain string
		want   []string
	}{
		{domain: "example.com", want: nil},
		{domain: "a.example.com", want: []string{"*.example.com"}},
		{domain: "a.b.example.com", want: []string{"*.b.example.com", "*.example.com"}},
		{domain: "com", want: nil},
	}

	for _, tc := range cases {
		got := wildcardPatternCandidates(tc.domain)
		if len(got) != len(tc.want) {
			t.Fatalf("wildcardPatternCandidates(%q) = %v, want %v", tc.domain, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("wildcardPatternCandidates(%q)[%d] = %q, want %q", tc.domain, i, got[i], tc.want[i])
			}
		}
	}
}

func TestMatchPublicTarget(t *testing.T) {
	cases := []struct {
		name          string
		licenseType   string
		storedTarget  string
		isWildcard    bool
		target        string
		isIP          bool
		wantMatchType string
		wantMatched   bool
	}{
		{
			name:          "单域名精确命中",
			licenseType:   "domain",
			storedTarget:  "example.com",
			target:        "example.com",
			wantMatchType: "exact",
			wantMatched:   true,
		},
		{
			name:         "单域名不做后缀匹配",
			licenseType:  "domain",
			storedTarget: "example.com",
			target:       "a.example.com",
		},
		{
			name:          "泛域名覆盖一级子域",
			licenseType:   "wildcard",
			storedTarget:  "*.example.com",
			isWildcard:    true,
			target:        "a.example.com",
			wantMatchType: "wildcard",
			wantMatched:   true,
		},
		{
			name:          "泛域名覆盖多级子域",
			licenseType:   "wildcard",
			storedTarget:  "*.example.com",
			isWildcard:    true,
			target:        "a.b.example.com",
			wantMatchType: "wildcard",
			wantMatched:   true,
		},
		{
			name:         "泛域名不覆盖根域",
			licenseType:  "wildcard",
			storedTarget: "*.example.com",
			isWildcard:   true,
			target:       "example.com",
		},
		{
			name:         "泛域名标记缺失时不命中",
			licenseType:  "wildcard",
			storedTarget: "*.example.com",
			isWildcard:   false,
			target:       "a.example.com",
		},
		{
			name:          "IP 精确命中",
			licenseType:   "ip",
			storedTarget:  "1.2.3.4",
			target:        "1.2.3.4",
			isIP:          true,
			wantMatchType: "exact",
			wantMatched:   true,
		},
		{
			name:         "IP 不会命中泛域名授权",
			licenseType:  "wildcard",
			storedTarget: "*.example.com",
			isWildcard:   true,
			target:       "1.2.3.4",
			isIP:         true,
		},
		{
			name:          "密钥授权按已绑定站点命中",
			licenseType:   "key",
			storedTarget:  "example.com",
			target:        "example.com",
			wantMatchType: "exact",
			wantMatched:   true,
		},
		{
			name:         "空绑定目标不命中",
			licenseType:  "domain",
			storedTarget: "",
			target:       "example.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matchType, matched := matchPublicTarget(tc.licenseType, tc.storedTarget, tc.isWildcard, tc.target, tc.isIP)
			if matched != tc.wantMatched || matchType != tc.wantMatchType {
				t.Fatalf("matchPublicTarget() = (%q, %v), want (%q, %v)", matchType, matched, tc.wantMatchType, tc.wantMatched)
			}
		})
	}
}

// 反查候选集必须覆盖真实校验能命中的全部泛域名写法，
// 否则会出现「查询说未授权、实际校验通过」的不一致。
func TestWildcardCandidatesCoverVerifySemantics(t *testing.T) {
	targets := []string{"a.example.com", "a.b.example.com", "a.b.c.example.com"}
	patterns := []string{"*.example.com", "*.b.example.com", "*.c.example.com", "*.b.c.example.com"}

	for _, target := range targets {
		candidates := map[string]bool{}
		for _, candidate := range wildcardPatternCandidates(target) {
			candidates[candidate] = true
		}

		for _, pattern := range patterns {
			if !wildcardDomainMatch(pattern, target) {
				continue
			}
			if !candidates[pattern] {
				t.Fatalf("目标 %q 会被 %q 覆盖，但候选集未包含该写法", target, pattern)
			}
		}
	}
}