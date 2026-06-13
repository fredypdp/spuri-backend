package storage

import "testing"

func TestConfiguredQuotaTotalBytesDefaultsToFreeAccount(t *testing.T) {
	t.Setenv("MEGA_QUOTA_TOTAL_BYTES", "")
	t.Setenv("MEGA_QUOTA_TOTAL_GB", "")

	total, err := configuredQuotaTotalBytes()
	if err != nil {
		t.Fatalf("configuredQuotaTotalBytes() error = %v", err)
	}

	const want uint64 = 20 * 1024 * 1024 * 1024
	if total != want {
		t.Fatalf("configuredQuotaTotalBytes() = %d, want %d", total, want)
	}
}

func TestConfiguredQuotaTotalBytesUsesBytesOverride(t *testing.T) {
	t.Setenv("MEGA_QUOTA_TOTAL_BYTES", "12345")
	t.Setenv("MEGA_QUOTA_TOTAL_GB", "99")

	total, err := configuredQuotaTotalBytes()
	if err != nil {
		t.Fatalf("configuredQuotaTotalBytes() error = %v", err)
	}

	if total != 12345 {
		t.Fatalf("configuredQuotaTotalBytes() = %d, want 12345", total)
	}
}

func TestConfiguredQuotaTotalBytesUsesGBOverride(t *testing.T) {
	t.Setenv("MEGA_QUOTA_TOTAL_BYTES", "")
	t.Setenv("MEGA_QUOTA_TOTAL_GB", "50")

	total, err := configuredQuotaTotalBytes()
	if err != nil {
		t.Fatalf("configuredQuotaTotalBytes() error = %v", err)
	}

	const want uint64 = 50 * 1024 * 1024 * 1024
	if total != want {
		t.Fatalf("configuredQuotaTotalBytes() = %d, want %d", total, want)
	}
}
