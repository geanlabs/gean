package forkchoice

import "testing"

func TestQuorumScore(t *testing.T) {
	tests := []struct {
		validators uint64
		want       int64
	}{
		{validators: 0, want: 0},
		{validators: 1, want: 1},
		{validators: 2, want: 2},
		{validators: 3, want: 2},
		{validators: 4, want: 3},
		{validators: 5, want: 4},
		{validators: 6, want: 4},
	}

	for _, test := range tests {
		if got := quorumScore(test.validators); got != test.want {
			t.Fatalf("quorumScore(%d)=%d, want %d", test.validators, got, test.want)
		}
	}
}

func TestQuorumScoreSaturates(t *testing.T) {
	if got := quorumScore(^uint64(0)); got != maxScore {
		t.Fatalf("quorumScore(max)=%d, want max score %d", got, maxScore)
	}
}
