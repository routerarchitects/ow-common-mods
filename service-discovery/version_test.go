package servicediscovery

import "testing"

func TestParseLeadingSemver_AcceptsVPrefix(t *testing.T) {
	tests := []string{
		"1.2.3",
		"v1.2.3",
		"V1.2.3",
		"v1.2.3-abc123",
	}

	for _, tc := range tests {
		got := parseLeadingSemver(tc)
		if !got.ok || got.major != 1 || got.minor != 2 || got.patch != 3 {
			t.Fatalf("expected %q to parse as 1.2.3, got %+v", tc, got)
		}
	}
}

func TestCompareVersionLoose_VPrefixSemverOrdering(t *testing.T) {
	if cmp := compareVersionLoose("v1.10.0", "v1.2.0"); cmp <= 0 {
		t.Fatalf("expected v1.10.0 > v1.2.0, got %d", cmp)
	}

	if cmp := compareVersionLoose("v2.0.0", "1.9.9"); cmp <= 0 {
		t.Fatalf("expected v2.0.0 > 1.9.9, got %d", cmp)
	}
}

func TestCompareVersionLoose_IgnoresSuffixAfterLeadingSemver(t *testing.T) {
	if cmp := compareVersionLoose("v1.2.3-deadbee", "v1.2.4-cafebad"); cmp >= 0 {
		t.Fatalf("expected v1.2.3-* < v1.2.4-*, got %d", cmp)
	}
}

