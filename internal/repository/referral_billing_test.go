package repository

import "testing"

func TestDiscountedMinorRoundsPerBranch(t *testing.T) {
	for _, tc := range []struct {
		full int64
		bps  int
		want int64
	}{
		{1500000, 5000, 750000}, {101, 5000, 51}, {101, 0, 101}, {101, 10000, 0},
	} {
		if got := discountedMinor(tc.full, tc.bps); got != tc.want {
			t.Fatalf("discountedMinor(%d,%d)=%d, want %d", tc.full, tc.bps, got, tc.want)
		}
	}
}

func TestReferralRewardStopsAfterTwelvePaidCharges(t *testing.T) {
	if got := referralRewardMinor(750000, 12, 1000, 12); got != 75000 {
		t.Fatalf("twelfth charge reward=%d", got)
	}
	if got := referralRewardMinor(1500000, 13, 1000, 12); got != 0 {
		t.Fatalf("thirteenth charge reward=%d", got)
	}
	if got := referralRewardMinor(0, 1, 1000, 12); got != 0 {
		t.Fatalf("free charge reward=%d", got)
	}
}
